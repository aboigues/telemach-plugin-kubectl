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

// T047 : objets (Service, Ingress, PVC) et images à vérifier après une création réussie,
// déduits sans lire le cluster (research.md S9).
func TestObjectTargets(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	alone := write("alone.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: web\nspec:\n  selector:\n    app: web\n")
	together := write("together.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: shop\nspec:\n  selector:\n    app: shop\n---\n"+
		"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: shop\nspec:\n  selector:\n    matchLabels:\n      app: shop\n  template:\n    metadata:\n      labels:\n        app: shop\n        tier: front\n    spec:\n      containers:\n      - name: app\n        image: nginx\n---\n"+
		"apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: shop\n  namespace: autre\n---\n"+
		"apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: data\n")
	pod := write("pod.yaml", "apiVersion: v1\nkind: Service\nmetadata:\n  name: solo\nspec:\n  selector:\n    run: solo\n---\n"+
		"apiVersion: v1\nkind: Pod\nmetadata:\n  name: solo\n  labels:\n    run: solo\nspec:\n  initContainers:\n  - name: init\n    image: busybox:1.38\n  containers:\n  - name: app\n    image: registry.example:5000/app@sha256:abc\n")
	tests := []struct {
		line string
		want []ObjectTarget
	}{
		{"apply -f " + alone + " -n demo", []ObjectTarget{{Kind: "service", Name: "web", Namespace: "demo"}}},
		{"apply -f " + together + " -n demo", []ObjectTarget{
			{Kind: "service", Name: "shop", Namespace: "demo", CoveredBySameApply: true},
			{Kind: "ingress", Name: "shop", Namespace: "autre"},
			{Kind: "pvc", Name: "data", Namespace: "demo"},
		}},
		{"create -f " + pod, []ObjectTarget{{Kind: "service", Name: "solo", CoveredBySameApply: true}}},
		{"apply -f " + alone + " -f " + pod, []ObjectTarget{{Kind: "service", Name: "web"}, {Kind: "service", Name: "solo", CoveredBySameApply: true}}},
		{"create service clusterip web --tcp=80:8080 -n demo", []ObjectTarget{{Kind: "service", Name: "web", Namespace: "demo"}}},
		{"create svc nodeport web --tcp=80", []ObjectTarget{{Kind: "service", Name: "web"}}},
		{"expose deployment hello --name=hello-typo --port=80 -n demo", []ObjectTarget{{Kind: "service", Name: "hello-typo", Namespace: "demo"}}},
		{"expose deployment/hello --port=80", []ObjectTarget{{Kind: "service", Name: "hello"}}},
		{"expose deployment hello --port=80 --dry-run=client", nil},
		{"apply -f " + together + " --dry-run=server", nil},
		{"apply -f -", nil},
		{"apply -f https://example.org/x.yaml", nil},
		{"apply -k .", nil},
		{"create deployment web --image=nginx", nil},
	}
	for _, tt := range tests {
		got := ObjectTargets(command.Parse(strings.Fields(tt.line)))
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("ObjectTargets(%q) (-attendu +obtenu) :\n%s", tt.line, diff)
		}
	}

	images := []struct {
		line string
		want []string
	}{
		{"apply -f " + together, []string{"nginx"}},
		{"create -f " + pod, []string{"registry.example:5000/app@sha256:abc", "busybox:1.38"}},
		{"create deployment web --image=nginx:latest", []string{"nginx:latest"}},
		{"run web --image=busybox", []string{"busybox"}},
		{"apply -f " + together + " --dry-run=client", nil},
		{"apply -f " + alone, nil},
	}
	for _, tt := range images {
		if diff := cmp.Diff(tt.want, Images(command.Parse(strings.Fields(tt.line)))); diff != "" {
			t.Errorf("Images(%q) (-attendu +obtenu) :\n%s", tt.line, diff)
		}
	}
}
