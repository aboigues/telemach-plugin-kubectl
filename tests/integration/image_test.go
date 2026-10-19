//go:build integration

package integration

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// imageVersion est la version injectée dans l'image de test.
const imageVersion = "0.0.0-test"

// testImage construit, une fois par test, l'image du Dockerfile à partir d'un contexte au
// format GoReleaser dockers_v2 (binaires linux sous linux/<arch>/, dossier de compilation
// darwin sous dist/), avec les options de compilation de .goreleaser.yaml.
func testImage(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker non installé")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("démon Docker indisponible")
	}
	ctx := t.TempDir()
	arch := runtime.GOARCH
	builds := []struct{ goos, pkg, out string }{
		{"linux", "kubectl-telemach", filepath.Join(ctx, "linux", arch, "kubectl-telemach")},
		{"linux", "telemach-image", filepath.Join(ctx, "linux", arch, "telemach-image")},
		{"darwin", "kubectl-telemach", filepath.Join(ctx, "dist", "kubectl-telemach_darwin_"+arch+"_test", "kubectl-telemach")},
	}
	for _, b := range builds {
		cmd := exec.Command("go", "build", "-trimpath", //nolint:gosec // compilation de test
			"-ldflags", "-s -w -buildid= -X main.version="+imageVersion,
			"-o", b.out, "../../cmd/"+b.pkg)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+b.goos, "GOARCH="+arch)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("compilation de %s (%s) : %v\n%s", b.pkg, b.goos, err, out)
		}
	}
	dockerfile, err := filepath.Abs("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	tag := "telemach-kubectl-test:" + imageVersion
	build := exec.Command("docker", "buildx", "build", "--load", "--platform", "linux/"+arch, //nolint:gosec // test
		"-t", tag, "-f", dockerfile, ctx)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("construction de l'image : %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", "-f", tag).Run() })
	return tag
}

// hostShell exécute script dans bash avec HOME isolé, comme un apprenant dans son terminal.
func hostShell(t *testing.T, home, script string, env ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("bash", "-c", script) //nolint:gosec // script de test
	cmd.Env = append(os.Environ(), append([]string{"HOME=" + home, "NO_COLOR=1"}, env...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		t.Fatalf("bash : %v", err)
	}
	return out.String(), errb.String(), code
}

// imageInstallCmd reproduit la commande d'installation du README (Linux et WSL2).
func imageInstallCmd(image, extra string) string {
	return `docker run --rm --user "$(id -u):$(id -g)" ` + extra + `-v ~/.local/bin:/sortie ` + image + ` install`
}

// T033 : l'installation par l'image, avec les commandes exactes du README (LL-004).
func TestImageInstall(t *testing.T) {
	image := testImage(t)

	t.Run("procédure du README", func(t *testing.T) {
		home := t.TempDir()
		start := time.Now()
		out, errb, code := hostShell(t, home, "mkdir -p ~/.local/bin\n"+imageInstallCmd(image, ""))
		elapsed := time.Since(start)
		if code != 0 {
			t.Fatalf("code = %d\nstdout:\n%s\nstderr:\n%s", code, out, errb)
		}
		// SC-003 (D-LL-012) : moins d'une minute une fois l'image présente.
		if elapsed > time.Minute {
			t.Errorf("installation en %v, plus d'une minute (SC-003)", elapsed)
		}
		t.Logf("installation par l'image en %v", elapsed)
		if !strings.Contains(out, "TELEMACH "+imageVersion+" installé") {
			t.Errorf("message de succès attendu :\n%s", out)
		}
		plugin := filepath.Join(home, ".local", "bin", "kubectl-telemach")
		fi, err := os.Stat(plugin)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("droits = %v, attendu 0755", fi.Mode().Perm())
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
			t.Errorf("propriétaire = %d, attendu l'utilisateur %d", st.Uid, os.Getuid())
		}
		version, err := exec.Command(plugin, "version").Output() //nolint:gosec // binaire installé par le test
		if err != nil {
			t.Fatal(err)
		}
		if first, _, _ := strings.Cut(string(version), "\n"); first != "TELEMACH PLUGIN KUBECTL "+imageVersion {
			t.Errorf("version = %q", first)
		}
		entries, _ := os.ReadDir(filepath.Dir(plugin))
		if len(entries) != 1 {
			t.Errorf("seul kubectl-telemach doit être présent : %v", entries)
		}

		// Réinstallation : déjà à jour.
		out, _, code = hostShell(t, home, imageInstallCmd(image, ""))
		if code != 0 || !strings.Contains(out, "déjà à jour") {
			t.Errorf("réinstallation : code %d\n%s", code, out)
		}
	})

	t.Run("macOS : binaire darwin", func(t *testing.T) {
		home := t.TempDir()
		_, errb, code := hostShell(t, home, "mkdir -p ~/.local/bin\n"+imageInstallCmd(image, "-e HOST_OS=darwin "))
		if code != 0 {
			t.Fatalf("code = %d\n%s", code, errb)
		}
		data, err := os.ReadFile(filepath.Join(home, ".local", "bin", "kubectl-telemach"))
		if err != nil {
			t.Fatal(err)
		}
		// En-tête Mach-O 64 bits (0xfeedfacf, petit-boutiste).
		if !bytes.HasPrefix(data, []byte{0xcf, 0xfa, 0xed, 0xfe}) {
			t.Errorf("binaire macOS (Mach-O) attendu")
		}
	})

	refusals := []struct {
		name, script, want string
		code               int
	}{
		{"sans montage", `docker run --rm --user "$(id -u):$(id -g)" ` + image + ` install`, "n'est pas monté", 1},
		{"sans --user", "mkdir -p ~/.local/bin\ndocker run --rm -v ~/.local/bin:/sortie " + image + " install", "--user", 1},
		// ~/.local existe souvent ; seul bin manque, et Docker le crée avec le compte root.
		{"dossier créé par Docker", "mkdir -p ~/.local\n" + imageInstallCmd(image, ""), "appartient à root", 1},
		{"Windows", "mkdir -p ~/.local/bin\n" + imageInstallCmd(image, "-e HOST_OS=windows "), "WSL2", 2},
		{"fichier étranger", "mkdir -p ~/.local/bin\nprintf '#!/bin/sh\\n' > ~/.local/bin/kubectl-telemach\n" + imageInstallCmd(image, ""), "n'est pas un binaire TELEMACH", 1},
		{"lien symbolique", "mkdir -p ~/.local/bin ~/ailleurs\nprintf '#!/bin/sh\\n' > ~/ailleurs/k\nln -s ~/ailleurs/k ~/.local/bin/kubectl-telemach\n" + imageInstallCmd(image, ""), "lien symbolique", 1},
	}
	for _, r := range refusals {
		t.Run(r.name, func(t *testing.T) {
			home := t.TempDir()
			_, errb, code := hostShell(t, home, r.script)
			if code != r.code {
				t.Errorf("code = %d, attendu %d\n%s", code, r.code, errb)
			}
			if !strings.Contains(errb, r.want) {
				t.Errorf("message attendu (%q) :\n%s", r.want, errb)
			}
			if r.name == "fichier étranger" {
				data, _ := os.ReadFile(filepath.Join(home, ".local", "bin", "kubectl-telemach"))
				if string(data) != "#!/bin/sh\n" {
					t.Errorf("le fichier étranger a été modifié")
				}
			}
			if r.name == "dossier créé par Docker" {
				entries, _ := os.ReadDir(filepath.Join(home, ".local", "bin"))
				if len(entries) != 0 {
					t.Errorf("rien ne doit être écrit : %v", entries)
				}
			}
		})
	}
}

// T041 : l'essai sans installation donne la même vérification que le plugin installé
// (FR-016), sur un dossier monté en lecture seule, sans rien y écrire ; un dossier sans
// manifeste est expliqué par l'image.
func TestImageCheck(t *testing.T) {
	image := testImage(t)
	plugin := filepath.Join(t.TempDir(), "kubectl-telemach")
	build := exec.Command("go", "build", "-o", plugin, "../../cmd/kubectl-telemach") //nolint:gosec // compilation de test
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compilation : %v\n%s", err, out)
	}
	// Dossiers ne contenant que des manifestes (LL-004 : jamais de fichier d'attentes).
	valid, err := filepath.Abs("../../testdata/manifests/valid")
	if err != nil {
		t.Fatal(err)
	}
	faulty := t.TempDir()
	data, err := os.ReadFile("../../testdata/manifests/unknown-field/deploy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(faulty, "deploy.yaml"), data, 0o644); err != nil { //nolint:gosec // lisible par le conteneur
		t.Fatal(err)
	}

	run := func(cmd *exec.Cmd) (string, int) {
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		cmd.Env = append(os.Environ(), "NO_COLOR=1", "TELEMACH_KUBECTL_VERSION=")
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return out.String(), 0
	}
	for name, dir := range map[string]string{"valide": valid, "champ inconnu": faulty} {
		t.Run(name, func(t *testing.T) {
			before, _ := os.ReadDir(dir)
			host, hostCode := run(exec.Command(plugin, "check", "--color=never", "-f", dir, "-R"))                                   //nolint:gosec // test
			img, imgCode := run(exec.Command("docker", "run", "--rm", "-v", dir+":/manifestes:ro", image, "check", "--color=never")) //nolint:gosec // test
			if imgCode != hostCode {
				t.Errorf("code %d dans l'image, %d sur l'hôte", imgCode, hostCode)
			}
			if got, want := img, strings.ReplaceAll(host, dir, "/manifestes"); got != want {
				t.Errorf("sortie différente :\n--- image ---\n%s\n--- hôte ---\n%s", got, want)
			}
			if after, _ := os.ReadDir(dir); len(after) != len(before) {
				t.Errorf("le dossier monté a changé : %d → %d fichiers", len(before), len(after))
			}
		})
	}
	t.Run("dossier sans manifeste", func(t *testing.T) {
		out, code := run(exec.Command("docker", "run", "--rm", "-v", t.TempDir()+":/manifestes:ro", image, "check")) //nolint:gosec // test
		if code != 2 || !strings.Contains(out, "Aucun manifeste") {
			t.Errorf("code %d, attendu 2 avec l'explication du montage :\n%s", code, out)
		}
	})
}
