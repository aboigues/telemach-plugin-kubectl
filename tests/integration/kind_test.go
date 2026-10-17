//go:build integration

package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

// cluster est le cluster kind jetable partagé par les tests du paquet, créé par TestMain à
// partir de TELEMACH_KIND_IMAGE (image épinglée par digest).
var cluster *kindcluster.Cluster

func TestMain(m *testing.M) {
	image := os.Getenv("TELEMACH_KIND_IMAGE")
	if image == "" {
		fmt.Println("TELEMACH_KIND_IMAGE non défini : tests d'intégration ignorés (aucun cluster de l'utilisateur n'est jamais utilisé).")
		os.Exit(0)
	}
	c, err := kindcluster.Create(image)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cluster = c
	code := m.Run()
	if err := c.Delete(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// namespace crée un namespace propre au test, supprimé à la fin.
func namespace(t *testing.T, base string) string {
	t.Helper()
	ns, err := cluster.CreateNamespace(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cluster.Must("delete", "namespace", ns, "--wait=false") })
	return ns
}

// TestClusterIsolated vérifie que le cluster de test n'utilise ni le kubeconfig ni le
// contexte de l'utilisateur.
func TestClusterIsolated(t *testing.T) {
	if home := os.Getenv("HOME"); home != "" && cluster.Kubeconfig == home+"/.kube/config" {
		t.Fatal("le cluster de test ne doit jamais écrire dans ~/.kube/config")
	}
	r := cluster.Kubectl("config", "current-context")
	if r.ExitCode != 0 {
		t.Fatalf("config current-context : %s", r.Stderr)
	}
	ns := namespace(t, "isolation")
	if r := cluster.Kubectl("get", "namespace", ns); r.ExitCode != 0 {
		t.Errorf("namespace de test introuvable : %s", r.Stderr)
	}
}
