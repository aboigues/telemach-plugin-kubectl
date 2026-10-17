package main

import (
	"strings"
	"testing"
)

// US3, quickstart V12 : deepen approfondit le dernier diagnostic ; --exercise ajoute un
// exercice dans un namespace que l'apprenant crée lui-même.
func TestDeepen(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"})
	f := writeStderr(t, `Error from server (NotFound): pods "web" not found`+"\n")
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--invoked-as", "k", "--", "get", "pod", "web"}); code != 0 {
		t.Fatal(code)
	}
	errb.Reset()
	if code := run(a, []string{"deepen", "--color=never"}); code != 0 {
		t.Fatal(code)
	}
	got := errb.String()
	for _, want := range []string{"📘 Concept — namespace", "▶ Exemple", "kubectl get pods -A", "k telemach deepen --exercise", "📖 Documentation : https://kubernetes.io/docs/"} {
		if !strings.Contains(got, want) {
			t.Errorf("deepen sans %q :\n%s", want, got)
		}
	}
	errb.Reset()
	run(a, []string{"deepen", "--color=never", "--exercise"})
	if ex := errb.String(); !strings.Contains(ex, "🧪 Exercice") || !strings.Contains(ex, "kubectl create namespace") {
		t.Errorf("exercice attendu :\n%s", ex)
	}
}

func TestDeepenWithoutDiagnosis(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"})
	if code := run(a, []string{"deepen", "--color=never"}); code != 0 || !strings.Contains(errb.String(), "Aucun diagnostic récent") {
		t.Errorf("code %d :\n%s", code, errb)
	}
}

// Après check, deepen approfondit le premier problème trouvé.
func TestDeepenAfterCheck(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"})
	run(a, []string{"check", "--color=never", "-f", corpus + "selector-mismatch/deploy.yaml"})
	errb.Reset()
	run(a, []string{"deepen", "--color=never"})
	if !strings.Contains(errb.String(), "label selector") {
		t.Errorf("deepen après check :\n%s", errb)
	}
}
