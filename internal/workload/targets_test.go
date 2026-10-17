package workload_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	. "github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// T072 : cibles de la vérification différée, déduites sans lire le cluster.
func TestTargets(t *testing.T) {
	dir := t.TempDir()
	multi := filepath.Join(dir, "all.yaml")
	content := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: solo\n  namespace: autre\nspec:\n  containers: []\n---\n" +
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  selector:\n    matchLabels:\n      tier: front\n      app: web\n---\n" +
		"apiVersion: v1\nkind: Service\nmetadata:\n  name: web\n"
	if err := os.WriteFile(multi, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		line string
		want []Target
	}{
		{"apply -f " + multi + " -n demo", []Target{{Pod: "solo", Namespace: "autre"}, {Selector: "app=web,tier=front", Namespace: "demo"}}},
		{"create -f " + multi, []Target{{Pod: "solo", Namespace: "autre"}, {Selector: "app=web,tier=front"}}},
		{"create deployment web --image=x -n demo", []Target{{Selector: "app=web", Namespace: "demo"}}},
		{"run web --image=x", []Target{{Pod: "web"}}},
		{"apply -f " + multi + " --dry-run=client", nil},
		{"apply -f -", nil},
		{"apply -f https://example.org/x.yaml", nil},
		{"apply -k .", nil},
		{"get pods", nil},
		{"create configmap c --from-file=x", nil},
	}
	for _, tt := range tests {
		got := Targets(command.Parse(strings.Fields(tt.line)))
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("Targets(%q) (-attendu +obtenu) :\n%s", tt.line, diff)
		}
	}
}
