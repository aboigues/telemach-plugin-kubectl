package main

import (
	"strings"
	"testing"
)

// FR-009a, FR-009b, quickstart V17 : la deuxième erreur identique est courte, « last »
// réaffiche l'explication complète.
func TestLastAfterRepeat(t *testing.T) {
	env := map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"}
	a, _, errb := newTestApp(t, env)
	f := writeStderr(t, `error: the server doesn't have a resource type "pdos"`+"\n")
	explain := []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--", "get", "pdos", "-n", "demo"}
	for i := 0; i < 2; i++ {
		if code := run(a, explain); code != 0 {
			t.Fatalf("explain %d : code %d", i, code)
		}
	}
	second := errb.String()[strings.LastIndex(errb.String(), "✖"):]
	if !strings.Contains(second, "Même erreur que précédemment") || strings.Contains(second, "📘 Concept") {
		t.Errorf("la seconde explication doit être courte :\n%s", second)
	}
	errb.Reset()
	if code := run(a, []string{"last", "--color=never"}); code != 0 {
		t.Fatalf("last : code %d", code)
	}
	got := errb.String()
	for _, want := range []string{"Explication complète de l'erreur enregistrée aujourd'hui à 12:00 : kubectl get pdos -n demo", "📘 Concept", "kubectl get pods -n demo", "Message kubectl d'origine"} {
		if !strings.Contains(got, want) {
			t.Errorf("last sans %q :\n%s", want, got)
		}
	}
	if strings.Contains(got, "intégration shell") {
		t.Errorf("aucun avertissement attendu quand l'intégration est à jour :\n%s", got)
	}
}

func TestLastWithoutDiagnosis(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "bash:9.9.9"})
	if code := run(a, []string{"last", "--color=never"}); code != 0 || !strings.Contains(errb.String(), "Aucune erreur récente") {
		t.Errorf("code %d :\n%s", code, errb)
	}
}

// D-LL-006, D-LL-013 : intégration absente ou chargée par une autre version → avertissement.
func TestLastShellStatus(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"SHELL": "/bin/bash"}, `eval "$(kubectl-telemach shell-init bash)"`},
		{map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:0.0.1"}, "chargée par la version 0.0.1"},
	} {
		a, _, errb := newTestApp(t, tc.env)
		if code := run(a, []string{"last", "--color=never"}); code != 0 || !strings.Contains(errb.String(), tc.want) {
			t.Errorf("code %d, attendu %q :\n%s", code, tc.want, errb)
		}
	}
}

// Avec l'alias k, « last » reprend la forme tapée par l'apprenant.
func TestLastInvokedAsK(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"})
	f := writeStderr(t, `error: unknown command "gt" for "kubectl"`+"\n")
	if code := run(a, []string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--invoked-as", "k", "--", "gt", "pods"}); code != 0 {
		t.Fatal(code)
	}
	errb.Reset()
	run(a, []string{"last", "--color=never"})
	if !strings.Contains(errb.String(), ": k gt pods") || !strings.Contains(errb.String(), "k get pods") {
		t.Errorf("forme « k » attendue :\n%s", errb)
	}
}
