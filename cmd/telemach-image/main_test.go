package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// execCall mémorise l'appel au binaire cible.
type execCall struct {
	path string
	argv []string
	env  []string
}

// testApp construit une application isolée : sorties en mémoire, exécution simulée.
func testApp(t *testing.T) (*app, *bytes.Buffer, *bytes.Buffer, *execCall) {
	t.Helper()
	var out, errb bytes.Buffer
	call := &execCall{}
	return &app{
		stdout:  &out,
		stderr:  &errb,
		getenv:  func(string) string { return "" },
		environ: func() []string { return []string{"TERM=xterm", "HOME=/", "XDG_STATE_HOME=/etat"} },
		exec: func(path string, argv, env []string) error {
			call.path, call.argv, call.env = path, argv, env
			return nil
		},
		version:  "1.2.3",
		root:     t.TempDir(),
		dest:     t.TempDir(),
		manifest: t.TempDir(),
	}, &out, &errb, call
}

// FR-019 : l'aide donne les deux usages avec les commandes canoniques de
// contracts/image-cli.md, à la version de l'image.
func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			a, out, _, call := testApp(t)
			if code := run(a, args); code != 0 {
				t.Fatalf("code = %d", code)
			}
			if call.path != "" {
				t.Errorf("le plugin ne doit pas être exécuté pour l'aide")
			}
			for _, want := range []string{
				`docker run --rm -it -v "$PWD":/manifestes:ro telemachlearning/telemach-kubectl:1.2.3 check`,
				"mkdir -p ~/.local/bin",
				`docker run --rm --user "$(id -u):$(id -g)" -v ~/.local/bin:/sortie telemachlearning/telemach-kubectl:1.2.3 install`,
				`docker run --rm --user "$(id -u):$(id -g)" -e HOST_OS=darwin -v ~/.local/bin:/sortie telemachlearning/telemach-kubectl:1.2.3 install`,
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("commande absente de l'aide : %s\n%s", want, out.String())
				}
			}
		})
	}
}

// Une version de développement ne doit pas produire de commande avec un tag invalide.
func TestHelpDevVersion(t *testing.T) {
	a, out, _, _ := testApp(t)
	a.version = "dev"
	run(a, nil)
	if !strings.Contains(out.String(), "telemachlearning/telemach-kubectl:X.Y.Z") {
		t.Errorf("tag générique attendu pour une version de développement :\n%s", out.String())
	}
}

// Toute autre commande est transmise telle quelle au plugin linux embarqué (FR-016).
func TestPassThrough(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"explain", "--exit-code", "1", "--", "get", "pods"}} {
		t.Run(args[0], func(t *testing.T) {
			a, _, _, call := testApp(t)
			if code := run(a, args); code != 0 {
				t.Fatalf("code = %d", code)
			}
			if call.path != linuxPlugin(a.root) {
				t.Errorf("chemin = %q, attendu %q", call.path, linuxPlugin(a.root))
			}
			want := append([]string{"kubectl-telemach"}, args...)
			if strings.Join(call.argv, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("argv = %q, attendu %q", call.argv, want)
			}
			// Sans HOME ni XDG_STATE_HOME, le plugin n'écrit aucun état (contracts/image-cli.md).
			if strings.Join(call.env, " ") != "TERM=xterm" {
				t.Errorf("environnement du plugin = %q, attendu TERM seulement", call.env)
			}
		})
	}
}

func TestPassThroughFailure(t *testing.T) {
	a, _, errb, _ := testApp(t)
	a.exec = func(string, []string, []string) error { return errors.New("introuvable") }
	if code := run(a, []string{"version"}); code != 2 {
		t.Errorf("code = %d, attendu 2", code)
	}
	if !strings.Contains(errb.String(), "introuvable") {
		t.Errorf("cause attendue dans le message :\n%s", errb.String())
	}
}

// Un argument inconnu de « install » est une erreur d'utilisation (code 2), sans rien
// exécuter.
func TestInstallUnknownArgument(t *testing.T) {
	a, _, _, call := testApp(t)
	if code := run(a, []string{"install", "--inconnu"}); code != 2 {
		t.Errorf("code = %d, attendu 2", code)
	}
	if call.path != "" {
		t.Errorf("le plugin ne doit pas être exécuté")
	}
}

// T040 (research R6) : « check » sans -f vérifie le dossier monté, sous-dossiers compris ;
// avec -f, les arguments sont transmis tels quels ; un dossier sans manifeste est expliqué par
// l'image elle-même, sans lancer le plugin.
func TestCheck(t *testing.T) {
	withManifest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(withManifest, "sous-dossier"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withManifest, "sous-dossier", "web.yml"), []byte("kind: Pod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	jsonOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(jsonOnly, "web.json"), []byte(`{"kind":"Pod"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "LISEZMOI.txt"), []byte("rien\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		dir      string
		args     []string
		wantArgv []string // nil : plugin non exécuté
		wantCode int
	}{
		{"sans -f : dossier monté", withManifest, []string{"check"}, []string{"check", "-f", "MANIFESTES", "-R"}, 0},
		{"sans -f, avec options", withManifest, []string{"check", "--color", "never"}, []string{"check", "--color", "never", "-f", "MANIFESTES", "-R"}, 0},
		{"-f explicite", empty, []string{"check", "-f", "x.yaml"}, []string{"check", "-f", "x.yaml"}, 0},
		{"--filename= explicite", empty, []string{"check", "--filename=x.yaml"}, []string{"check", "--filename=x.yaml"}, 0},
		{"-f collé", empty, []string{"check", "-fx.yaml"}, []string{"check", "-fx.yaml"}, 0},
		{"-f groupé avec -R", empty, []string{"check", "-Rf", "x"}, []string{"check", "-Rf", "x"}, 0},
		{"-R seul n'est pas -f", withManifest, []string{"check", "-R"}, []string{"check", "-R", "-f", "MANIFESTES", "-R"}, 0},
		{"aide sans montage", empty, []string{"check", "--help"}, []string{"check", "--help"}, 0},
		{"aide courte sans montage", empty, []string{"check", "-h"}, []string{"check", "-h"}, 0},
		{"manifeste .json seul", jsonOnly, []string{"check"}, []string{"check", "-f", "MANIFESTES", "-R"}, 0},
		{"dossier sans manifeste", empty, []string{"check"}, nil, 2},
		{"dossier absent", filepath.Join(empty, "absent"), []string{"check"}, nil, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _, errb, call := testApp(t)
			a.manifest = c.dir
			if code := run(a, c.args); code != c.wantCode {
				t.Errorf("code = %d, attendu %d\n%s", code, c.wantCode, errb.String())
			}
			if c.wantArgv == nil {
				if call.path != "" {
					t.Errorf("le plugin ne doit pas être exécuté : %q", call.argv)
				}
				for _, want := range []string{"Aucun manifeste", "montage", `docker run --rm -it -v "$PWD":/manifestes:ro telemachlearning/telemach-kubectl:1.2.3 check`} {
					if !strings.Contains(errb.String(), want) {
						t.Errorf("message de l'image sans %q :\n%s", want, errb.String())
					}
				}
				return
			}
			want := append([]string{"kubectl-telemach"}, c.wantArgv...)
			for i, w := range want {
				if w == "MANIFESTES" {
					want[i] = c.dir
				}
			}
			if strings.Join(call.argv, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("argv = %q, attendu %q", call.argv, want)
			}
		})
	}
}
