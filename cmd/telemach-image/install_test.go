package main

import (
	"bytes"
	"debug/buildinfo"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
)

var update = flag.Bool("update", false, "réécrit les fichiers golden")

// installApp prépare une image simulée : binaires linux et darwin sous root, dossier de
// destination « monté », processus non-root propriétaire du dossier.
func installApp(t *testing.T) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	a, out, errb, _ := testApp(t)
	a.version = "0.2.0"
	a.goarch = "arm64"
	a.getuid = func() int { return 1000 }
	a.owner = func(string) (int, error) { return 1000, nil }
	a.mountinfo = func() ([]byte, error) {
		return []byte("36 35 98:0 /home/u/.local/bin " + a.dest + " rw,relatime - ext4 /dev/sdc rw\n"), nil
	}
	a.buildInfo = func(path string) (*buildinfo.BuildInfo, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// Les faux binaires contiennent « module version ».
		fields := strings.Fields(string(data))
		if len(fields) < 2 || !strings.Contains(fields[0], ".") {
			return nil, errors.New("not a Go binary")
		}
		return &buildinfo.BuildInfo{Main: debug.Module{Path: fields[0], Version: fields[1]}}, nil
	}
	for _, osName := range []string{"linux", "darwin"} {
		writeFile(t, filepath.Join(a.root, osName, pluginName), modulePath+" v0.2.0 "+osName, 0o755)
	}
	return a, out, errb
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// golden compare got au fichier testdata/golden/<name>.txt (réécrit avec -update).
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s : %v (relancer avec -update)", path, err)
	}
	if got != string(want) {
		t.Errorf("sortie différente de %s :\n--- obtenu ---\n%s\n--- attendu ---\n%s", path, got, want)
	}
}

// destEntries liste le contenu du dossier de destination.
func destEntries(t *testing.T, a *app) []string {
	t.Helper()
	entries, err := os.ReadDir(a.dest)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func installed(t *testing.T, a *app) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.dest, pluginName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallNew(t *testing.T) {
	a, out, _ := installApp(t)
	if code := run(a, []string{"install"}); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if got := installed(t, a); !strings.HasSuffix(got, "linux") {
		t.Errorf("binaire linux attendu par défaut, contenu %q", got)
	}
	fi, err := os.Stat(filepath.Join(a.dest, pluginName))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("droits = %v, attendu 0755", fi.Mode().Perm())
	}
	if names := destEntries(t, a); len(names) != 1 {
		t.Errorf("seul kubectl-telemach doit rester dans la destination : %v", names)
	}
	for _, want := range []string{"PATH", "kubectl plugin list", "masqué", "kubectl-telemach shell-init"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("message de succès sans %q :\n%s", want, out.String())
		}
	}
	golden(t, "install_new_linux", out.String())
}

func TestInstallDarwin(t *testing.T) {
	for _, v := range []string{"darwin", "DARWIN"} {
		a, out, _ := installApp(t)
		a.getenv = func(k string) string {
			if k == "HOST_OS" {
				return v
			}
			return ""
		}
		if code := run(a, []string{"install"}); code != 0 {
			t.Fatalf("HOST_OS=%s : code = %d", v, code)
		}
		if got := installed(t, a); !strings.HasSuffix(got, "darwin") {
			t.Errorf("HOST_OS=%s : binaire darwin attendu, contenu %q", v, got)
		}
		if v == "darwin" {
			golden(t, "install_new_darwin", out.String())
		}
	}
}

func TestInstallUpdateAndSameVersion(t *testing.T) {
	a, out, _ := installApp(t)
	writeFile(t, filepath.Join(a.dest, pluginName), modulePath+" v0.1.0 old", 0o755)
	if code := run(a, []string{"install"}); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out.String(), "Version remplacée : 0.1.0") {
		t.Errorf("ancienne version attendue :\n%s", out.String())
	}
	golden(t, "install_update", out.String())

	out.Reset()
	if code := run(a, []string{"install"}); code != 0 {
		t.Fatalf("réinstallation : code = %d", code)
	}
	if !strings.Contains(out.String(), "déjà à jour") {
		t.Errorf("« déjà à jour » attendu :\n%s", out.String())
	}
}

// refusal vérifie un refus : code attendu, rien d'écrit, message golden sur stderr.
func refusal(t *testing.T, a *app, errb *bytes.Buffer, args []string, wantCode int, goldenName string) {
	t.Helper()
	before := destEntries(t, a)
	var content string
	if len(before) > 0 {
		if data, err := os.ReadFile(filepath.Join(a.dest, pluginName)); err == nil {
			content = string(data)
		}
	}
	if code := run(a, args); code != wantCode {
		t.Errorf("code = %d, attendu %d\n%s", code, wantCode, errb.String())
	}
	after := destEntries(t, a)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("la destination a changé : %v → %v", before, after)
	}
	if content != "" {
		if data, _ := os.ReadFile(filepath.Join(a.dest, pluginName)); string(data) != content {
			t.Errorf("le fichier existant a été modifié")
		}
	}
	golden(t, goldenName, errb.String())
}

func TestInstallRefusals(t *testing.T) {
	t.Run("windows", func(t *testing.T) {
		a, _, errb := installApp(t)
		a.getenv = func(k string) string {
			if k == "HOST_OS" {
				return "windows"
			}
			return ""
		}
		refusal(t, a, errb, []string{"install"}, 2, "refuse_windows")
	})
	t.Run("système inconnu", func(t *testing.T) {
		a, _, errb := installApp(t)
		a.getenv = func(k string) string {
			if k == "HOST_OS" {
				return "freebsd"
			}
			return ""
		}
		refusal(t, a, errb, []string{"install"}, 2, "refuse_unknown_os")
	})
	t.Run("non monté", func(t *testing.T) {
		a, _, errb := installApp(t)
		a.mountinfo = func() ([]byte, error) { return []byte("1 0 0:1 / / rw - overlay overlay rw\n"), nil }
		refusal(t, a, errb, []string{"install"}, 1, "refuse_not_mounted")
	})
	t.Run("dossier de root", func(t *testing.T) {
		a, _, errb := installApp(t)
		a.owner = func(string) (int, error) { return 0, nil }
		refusal(t, a, errb, []string{"install"}, 1, "refuse_root_owned")
	})
	t.Run("non accessible en écriture", func(t *testing.T) {
		a, _, errb := installApp(t)
		if err := os.Chmod(a.dest, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(a.dest, 0o755) })
		if os.Getuid() == 0 {
			t.Skip("root ignore les droits du dossier")
		}
		refusal(t, a, errb, []string{"install"}, 1, "refuse_not_writable")
	})
	t.Run("monté en lecture seule", func(t *testing.T) {
		// Un montage :ro ne se simule pas sans privilèges : le classement de l'erreur et le
		// message sont vérifiés séparément.
		err := &fs.PathError{Op: "open", Path: "/sortie/x", Err: syscall.EROFS}
		if kind := copyRefusal(err); kind != refuseReadOnly {
			t.Fatalf("EROFS classé %d, attendu refuseReadOnly", kind)
		}
		if kind := copyRefusal(&fs.PathError{Op: "open", Path: "/sortie/x", Err: syscall.EACCES}); kind != refuseNotWritable {
			t.Fatalf("EACCES classé %d, attendu refuseNotWritable", kind)
		}
		golden(t, "refuse_read_only", refusalText(render.NewTheme(false, true), refuseReadOnly, "0.2.0", "linux", ""))
	})
	t.Run("fichier étranger", func(t *testing.T) {
		a, _, errb := installApp(t)
		writeFile(t, filepath.Join(a.dest, pluginName), "#!/bin/sh\necho autre", 0o755)
		refusal(t, a, errb, []string{"install"}, 1, "refuse_foreign")
	})
	t.Run("autre module Go", func(t *testing.T) {
		a, _, errb := installApp(t)
		writeFile(t, filepath.Join(a.dest, pluginName), "example.com/autre v1.0.0", 0o755)
		refusal(t, a, errb, []string{"install"}, 1, "refuse_foreign")
	})
	// Un lien symbolique n'est jamais remplacé (research R7).
	t.Run("lien symbolique", func(t *testing.T) {
		a, _, errb := installApp(t)
		target := filepath.Join(t.TempDir(), "ailleurs")
		writeFile(t, target, modulePath+" v0.1.0", 0o755)
		if err := os.Symlink(target, filepath.Join(a.dest, pluginName)); err != nil {
			t.Fatal(err)
		}
		refusal(t, a, errb, []string{"install"}, 1, "refuse_symlink")
		if !strings.Contains(errb.String(), "kubectl plugin list") {
			t.Errorf("le refus doit proposer de repérer les installations :\n%s", errb.String())
		}
	})
	t.Run("dossier homonyme", func(t *testing.T) {
		a, _, errb := installApp(t)
		if err := os.Mkdir(filepath.Join(a.dest, pluginName), 0o755); err != nil {
			t.Fatal(err)
		}
		refusal(t, a, errb, []string{"install"}, 1, "refuse_not_regular")
	})
	t.Run("argument inconnu", func(t *testing.T) {
		a, _, errb := installApp(t)
		refusal(t, a, errb, []string{"install", "--force"}, 2, "install_usage")
	})
}

func TestInstallHelp(t *testing.T) {
	a, out, _ := installApp(t)
	if code := run(a, []string{"install", "--help"}); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if names := destEntries(t, a); len(names) != 0 {
		t.Errorf("l'aide ne doit rien écrire : %v", names)
	}
	golden(t, "install_help", out.String())
}

// Un échec pendant la copie ne laisse ni fichier partiel ni fichier temporaire.
func TestInstallAtomic(t *testing.T) {
	a, _, errb := installApp(t)
	src := filepath.Join(a.root, "linux", pluginName)
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(src, 0o755); err != nil { // illisible comme fichier : la copie échoue
		t.Fatal(err)
	}
	if code := run(a, []string{"install"}); code != 1 {
		t.Errorf("code = %d, attendu 1\n%s", code, errb.String())
	}
	if names := destEntries(t, a); len(names) != 0 {
		t.Errorf("aucun fichier ne doit rester : %v", names)
	}
}
