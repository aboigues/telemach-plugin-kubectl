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

// Commande interactive : stderr non capté ; une version de kubectl mal formée est ignorée.
func TestExplainInteractiveAndVersion(t *testing.T) {
	a, _, errb := newTestApp(t, nil)
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "137", "--interactive", "--kubectl-version", "pas-une-version", "--", "exec", "-it", "web", "--", "sh"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	got := errb.String()
	if !strings.Contains(got, "interactive") || strings.Contains(got, "pas-une-version") {
		t.Errorf("mode interactif mal rendu ou version invalide affichée :\n%s", got)
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
	if code := run(a, []string{"version"}); code != 0 || !strings.Contains(out.String(), "TELEMACH PLUGIN KUBECTL 9.9.9") || !strings.Contains(out.String(), "1.35, 1.36, 1.37") {
		t.Errorf("version : code %d\n%s", code, out)
	}
}
