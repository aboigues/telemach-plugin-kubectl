package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// newTestApp construit une application isolée : état dans un dossier temporaire, variables
// d'environnement fournies, sorties capturées.
func newTestApp(t *testing.T, env map[string]string) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	a := &app{
		stdout: &out, stderr: &errb,
		getenv:     func(k string) string { return env[k] },
		now:        func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) },
		getwd:      func() (string, error) { return t.TempDir(), nil },
		executable: func() string { return "/usr/local/bin/kubectl-telemach" },
		store:      state.Store{Dir: filepath.Join(t.TempDir(), "state")},
		version:    "9.9.9",
	}
	return a, &out, &errb
}

func writeStderr(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "stderr")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Quickstart V6 : erreur inventée → bloc « Erreur non reconnue », aucune cause (FR-008).
func TestExplainUnknown(t *testing.T) {
	a, _, errb := newTestApp(t, nil)
	f := writeStderr(t, "error: something totally new\n")
	code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--kubectl-version", "v1.37.1", "--", "get", "pods"})
	if code != 0 {
		t.Fatalf("code %d, sortie :\n%s", code, errb)
	}
	got := errb.String()
	for _, want := range []string{"✖ Erreur non reconnue", "préfère ne pas deviner", "kubectl get — code de sortie 1 — kubectl v1.37.1", "https://kubernetes.io/docs/", "Message kubectl d'origine", "error: something totally new"} {
		if !strings.Contains(got, want) {
			t.Errorf("sortie sans %q :\n%s", want, got)
		}
	}
	if strings.Contains(got, LabelCauseForTest) {
		t.Errorf("aucune cause ne doit être avancée :\n%s", got)
	}
	if _, err := os.Stat(f); err != nil {
		t.Error("explain ne doit jamais supprimer le fichier stderr")
	}
}

// LabelCauseForTest est le libellé du bloc « cause », absent d'une erreur non reconnue.
const LabelCauseForTest = "🔎 Cause probable"

// FR-007a, SC-007 : couleur quand elle est demandée, aucune séquence sinon, mêmes symboles.
func TestExplainColor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		env   map[string]string
		color bool
	}{
		{"always", []string{"--color=always"}, nil, true},
		{"never", []string{"--color=never"}, nil, false},
		{"NO_COLOR", nil, map[string]string{"NO_COLOR": "1"}, false},
		{"CLICOLOR_FORCE", nil, map[string]string{"CLICOLOR_FORCE": "1"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, errb := newTestApp(t, tc.env)
			f := writeStderr(t, `error: the server doesn't have a resource type "pdos"`+"\n")
			args := append(append([]string{"explain"}, tc.args...), "--exit-code", "1", "--stderr-file", f, "--", "get", "pdos")
			if code := run(a, args); code != 0 {
				t.Fatalf("code %d", code)
			}
			got := errb.String()
			if has := strings.Contains(got, "\x1b["); has != tc.color {
				t.Errorf("séquences de couleur présentes = %v, attendu %v", has, tc.color)
			}
			if !strings.Contains(got, "✖") || !strings.Contains(got, "Message kubectl d'origine") {
				t.Errorf("symboles et libellés obligatoires, avec ou sans couleur :\n%s", got)
			}
		})
	}
}

func TestExplainUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"explain", "--exit-code", "0", "--", "get", "pods"},
		{"explain", "--exit-code", "300", "--", "get", "pods"},
		{"explain", "--exit-code", "1", "--invoked-as", "kc", "--", "get", "pods"},
		{"explain", "--exit-code", "1"},
	} {
		a, _, _ := newTestApp(t, nil)
		if code := run(a, args); code != 2 {
			t.Errorf("%v : code %d, attendu 2", args, code)
		}
	}
}

// Les secrets de la ligne de commande et du message ne sont ni affichés ni écrits (SC-006).
func TestExplainRedacts(t *testing.T) {
	a, _, errb := newTestApp(t, nil)
	f := writeStderr(t, "error: You must be logged in to the server (Unauthorized) token=fake-token-123\n")
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--", "get", "pods", "--token", "fake-token-456"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	data, _ := os.ReadFile(filepath.Join(a.store.Dir, "last.json"))
	for _, leak := range []string{"fake-token-123", "fake-token-456"} {
		if strings.Contains(errb.String(), leak) || strings.Contains(string(data), leak) {
			t.Errorf("secret %q affiché ou écrit :\n%s\n%s", leak, errb, data)
		}
	}
}

// Commande interactive (stderr non capté) : le code 137 vient du programme du conteneur
// (FR-004) ; une version de kubectl mal formée est ignorée.
func TestExplainInteractiveAndVersion(t *testing.T) {
	a, _, errb := newTestApp(t, nil)
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "137", "--interactive", "--kubectl-version", "pas-une-version", "--", "exec", "-it", "web", "--", "sh"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	got := errb.String()
	if !strings.Contains(got, "programme lancé dans le conteneur") || !strings.Contains(got, "pas de Kubernetes") || strings.Contains(got, "pas-une-version") {
		t.Errorf("l'échec d'un exec interactif doit être attribué au programme du conteneur, sans version invalide :\n%s", got)
	}
}

func TestShellInitAndVersion(t *testing.T) {
	a, out, _ := newTestApp(t, nil)
	if code := run(a, []string{"shell-init", "zsh"}); code != 0 || !strings.Contains(out.String(), "/usr/local/bin/kubectl-telemach") || !strings.Contains(out.String(), "zsh:9.9.9") {
		t.Errorf("shell-init zsh : code %d\n%s", code, out)
	}
	a, _, errb := newTestApp(t, nil)
	if code := run(a, []string{"shell-init", "fish"}); code != 2 || !strings.Contains(errb.String(), "non pris en charge") {
		t.Errorf("shell-init fish : code %d, %s", code, errb)
	}
	a, out, _ = newTestApp(t, nil)
	if code := run(a, []string{"version"}); code != 0 || !strings.Contains(out.String(), "TELEMACH PLUGIN KUBECTL 9.9.9") || !strings.Contains(out.String(), "1.35, 1.36, 1.37") || !strings.Contains(out.String(), "Licence Apache-2.0 — https://www.telemach-learning.fr") {
		t.Errorf("version : code %d\n%s", code, out)
	}
}

// T064, quickstart V8 : après un « kubectl apply -f » réel en échec, le champ inconnu est
// localisé dans le 3e document, à la ligne du fichier (et non du document).
func TestExplainManifestLocated(t *testing.T) {
	dir := "../../testdata/manifests/unknown-field-multidoc"
	stderr, err := os.ReadFile(filepath.Join(dir, "stderr.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd) //nolint:errcheck
	a, _, errb := newTestApp(t, nil)
	f := writeStderr(t, string(stderr))
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--kubectl-version", "v1.36.5", "--", "apply", "-f", "all.yaml", "-n", "demo"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	got := errb.String()
	for _, want := range []string{"Le champ « spec.replica » n'existe pas", "kubectl explain deployment.spec", "Règles Kubernetes 1.36", "all.yaml:24:3 (document 3, Deployment/web)", "replicas: 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("sortie sans %q :\n%s", want, got)
		}
	}
}

// -f - : la ligne ne peut pas être localisée, et TELEMACH le dit.
func TestExplainManifestFromStdin(t *testing.T) {
	a, _, errb := newTestApp(t, nil)
	f := writeStderr(t, `Error from server (BadRequest): error when creating "STDIN": Deployment in version "v1" cannot be handled as a Deployment: strict decoding error: unknown field "spec.replica"`+"\n")
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--", "apply", "-f", "-"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(errb.String(), "ne peut pas être localisée") {
		t.Errorf("mention attendue :\n%s", errb)
	}
}

// Research.md S10 : une commande interrompue par Ctrl-C (code 130) n'est expliquée que si son
// stderr capté contient une erreur reconnue (transfert refusé d'un port-forward) ; une
// commande interactive dont stderr a été capté n'est pas présentée comme « non captée ».
func TestExplainInterrupted(t *testing.T) {
	refused, err := os.ReadFile("../../testdata/errors/portforward-connection-refused/stderr.txt")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"port-forward", "deploy/web", "18083:9999", "-n", "demo"}
	for _, tc := range []struct {
		name, stderr string
		want         string
	}{
		{"erreur de transfert reconnue", string(refused), "Rien n'écoute sur le port 9999 dans le pod"},
		{"interruption seule", "Forwarding from 127.0.0.1:18083 -> 9999\n", ""},
		{"stderr vide", "", ""},
	} {
		a, _, errb := newTestApp(t, nil)
		f := writeStderr(t, tc.stderr)
		if code := run(a, append([]string{"explain", "--color=never", "--exit-code", "130", "--stderr-file", f, "--"}, args...)); code != 0 {
			t.Fatalf("%s : code %d", tc.name, code)
		}
		got := errb.String()
		if tc.want == "" && got != "" {
			t.Errorf("%s : rien ne doit être affiché après Ctrl-C :\n%s", tc.name, got)
		}
		if tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s : sortie sans %q :\n%s", tc.name, tc.want, got)
		}
	}

	a, _, errb := newTestApp(t, nil)
	f := writeStderr(t, "error: something totally new\n")
	run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--", "exec", "-it", "web", "--", "sh"})
	if got := errb.String(); strings.Contains(got, "n'a pas été capté") || !strings.Contains(got, "something totally new") {
		t.Errorf("exec -it au stderr capté :\n%s", got)
	}
}
