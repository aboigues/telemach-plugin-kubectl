//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Feature 004 (T059) : découverte des pannes d'une formation et rapport, sans cluster.
func TestTrainingDiscoveryAndReport(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"tp04-jobs/pannes/import.yaml", "tp06-hpa/pannes/export.yaml", "tp06-hpa/solution/hpa.yaml", "apps/web.yaml"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("kind: Job\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pannes, err := discoverPannes(root)
	if err != nil || len(pannes) != 2 || filepath.Base(pannes[0].TP) != "tp04-jobs" || filepath.Base(pannes[1].File) != "export.yaml" {
		t.Fatalf("pannes : %+v, %v", pannes, err)
	}
	for out, want := range map[string]string{
		"✖ Erreur non reconnue\n":                             "unknown",
		"✖ « hpa/web » n'est pas pris en charge par diagnose": "unsupported",
		"✖ Le Job « import » a abandonné après 3 tentatives":  "reconnue",
	} {
		if got := classifyOutput(out); got != want {
			t.Errorf("%q : %s, attendu %s", out, got, want)
		}
	}
	report := renderReport([]TrainingFinding{
		{TP: "tp04-jobs", Panne: "import.yaml", Command: "kubectl telemach diagnose -n panne-01", Message: "a | b", Result: "reconnue"},
		{TP: "tp06-hpa", Panne: "export.yaml", Command: "kubectl apply -f export.yaml", Message: "error: x", Result: "unknown"},
	})
	for _, want := range []string{"| TP | Panne |", `a \| b`, "2 ligne(s), 1 non reconnue(s)"} {
		if !strings.Contains(report, want) {
			t.Errorf("rapport sans %q :\n%s", want, report)
		}
	}
}
