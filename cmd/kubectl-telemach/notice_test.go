package main

import (
	"os"
	"strings"
	"testing"
)

// Feature 004 (T021, FR-007) : l'avertissement PodSecurity d'une commande réussie est expliqué
// par une information, au plus une fois par namespace et par 24 h, sans lecture du cluster.
func TestNoticePodSecurityWarning(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL": "/chemin/inexistant/kubectl"})
	stderr, err := os.ReadFile("../../testdata/errors/apply-pod-security-warn/stderr.txt")
	if err != nil {
		t.Fatal(err)
	}
	f := writeStderr(t, string(stderr))
	args := []string{"notice", "--color=never", "--stderr-file", f, "--", "apply", "-f", "pod.yaml", "-n", "t3w"}
	if code := run(a, args); code != 0 {
		t.Fatalf("code %d :\n%s", code, errb)
	}
	got := strings.ReplaceAll(errb.String(), " ", " ")
	for _, want := range []string{"ℹ", "restricted", "telemach deepen"} {
		if !strings.Contains(got, want) {
			t.Errorf("information sans %q :\n%s", want, got)
		}
	}
	// deepen approfondit l'avertissement, enregistré comme dernier diagnostic.
	errb.Reset()
	run(a, []string{"deepen", "--color=never"})
	if !strings.Contains(errb.String(), "Pod Security Admission") {
		t.Errorf("deepen doit approfondir l'avertissement :\n%s", errb)
	}
	errb.Reset()
	run(a, args)
	if errb.Len() != 0 {
		t.Errorf("l'information ne s'affiche qu'une fois par namespace et par 24 h :\n%s", errb)
	}
	// Un autre namespace a droit à son information.
	errb.Reset()
	run(a, []string{"notice", "--color=never", "--stderr-file", f, "--", "apply", "-f", "pod.yaml", "-n", "autre"})
	if errb.Len() == 0 {
		t.Error("information attendue pour un autre namespace")
	}
	// Une sortie sans avertissement PodSecurity ne produit rien.
	errb.Reset()
	run(a, []string{"notice", "--color=never", "--stderr-file", writeStderr(t, "Warning: autre chose\n"), "--", "apply", "-f", "pod.yaml"})
	if errb.Len() != 0 {
		t.Errorf("rien n'est attendu :\n%s", errb)
	}
}
