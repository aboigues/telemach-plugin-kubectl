package diagnose

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
)

const testCatalog = `
- id: cmdline.unknown-resource-type
  category: cmdline
  applies: [get, describe]
  patterns: ['the server doesn''t have a resource type "(?P<type>[^"]+)"']
  concept: {name: resource type, definition: Chaque objet Kubernetes a un type.}
  summary: Le type de ressource « {{.type}} » n'existe pas.
  cause: Le type tapé ne correspond à aucun type connu.
  fixes: [{description: Lister les types connus., command: '{{.kubectl}} api-resources', safe: true}]
  deepen: {explanation: Le serveur publie ses types., example: kubectl api-resources, exercise: Crée un namespace avec kubectl create namespace exo.}
  doc_url: https://kubernetes.io/docs/reference/kubectl/
  tests: [x]
- id: runtime.application-error
  category: runtime
  applies: [exec]
  fallback: application
  concept: {name: container process, definition: Le programme du conteneur a échoué.}
  summary: La commande lancée dans le conteneur a échoué.
  cause: Le programme a renvoyé le code {{.exit_code}}.
  fixes: [{description: Lire le journal du pod., command: '{{.kubectl}} logs {{.name}}', safe: true}]
  deepen: {explanation: Le code vient du programme., example: kubectl logs web, exercise: Crée un namespace avec kubectl create namespace exo.}
  doc_url: https://kubernetes.io/docs/tasks/debug/
  tests: [y]
`

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(fstest.MapFS{"x.yaml": {Data: []byte(testCatalog)}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDiagnoseRecognized(t *testing.T) {
	cat := loadCatalog(t)
	in := Input{
		Command:   command.Parse(strings.Fields("get pdos -n demo")),
		ExitCode:  1,
		Stderr:    `error: the server doesn't have a resource type "pdos"`,
		InvokedAs: "k",
	}
	d := Diagnose(in, cat)
	if d.Confidence != Recognized || d.Entry == nil || d.Entry.ID != "cmdline.unknown-resource-type" {
		t.Fatalf("diagnostic inattendu : %+v", d)
	}
	if d.Captures["type"] != "pdos" || d.Captures["kubectl"] != "k" || d.Captures["namespace"] != "demo" || d.Captures["scope"] != "-n demo" {
		t.Errorf("captures : %v", d.Captures)
	}
	if d.Origin == OriginUnknown {
		t.Errorf("une erreur reconnue doit avoir une origine : %s", d.Origin)
	}
}

func TestDiagnoseUnknownInventsNothing(t *testing.T) {
	d := Diagnose(Input{Command: command.Parse([]string{"get", "pods"}), ExitCode: 1, Stderr: "error: something totally new"}, loadCatalog(t))
	if d.Confidence != Unknown || d.Entry != nil {
		t.Errorf("aucune cause ne doit être inventée : %+v", d)
	}
}

func TestDiagnoseAppliesFilter(t *testing.T) {
	in := Input{Command: command.Parse([]string{"apply", "-f", "x.yaml"}), ExitCode: 1, Stderr: `error: the server doesn't have a resource type "x"`}
	if d := Diagnose(in, loadCatalog(t)); d.Entry != nil {
		t.Errorf("l'entrée ne s'applique pas à apply : %+v", d.Entry.ID)
	}
}

func TestFingerprintStable(t *testing.T) {
	c := command.Parse([]string{"logs", "web-7d4b9c8f6d-abcde"})
	a := Fingerprint(c, "E1016 10:00:00.123456   1234 x.go:1] error: pod web-7d4b9c8f6d-abcde failed at 2026-10-16T10:00:00Z")
	b := Fingerprint(c, "E1016 10:05:00.999999   1234 x.go:1] error:  pod web-5f6b7c8d9e-zyxwv failed at 2026-10-16T10:05:00Z")
	if a != b {
		t.Error("l'empreinte doit ignorer horodatages, préfixes et suffixes de pods")
	}
	if a == Fingerprint(c, "error: autre chose") {
		t.Error("deux erreurs différentes doivent avoir des empreintes différentes")
	}
}

// Issue #10 : le serveur API a répondu, c'est le webhook d'admission qu'il n'a pas pu
// joindre. Quelle que soit la cause réseau recopiée en fin de message (refus, certificat,
// délai), l'erreur ne doit pas être attribuée au serveur API ni au kubeconfig.
func TestWebhookIsNotTheAPIServer(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	const prefix = `Error from server (InternalError): error when creating "ing.yaml": Internal error occurred: failed calling webhook "validate.nginx.ingress.kubernetes.io": failed to call webhook: Post "https://ingress-nginx-controller-admission.ingress-nginx.svc:443/networking/v1/ingresses?timeout=10s": `
	for _, cause := range []string{
		"dial tcp 10.96.1.2:443: connect: connection refused",
		"tls: failed to verify certificate: x509: certificate signed by unknown authority",
		"context deadline exceeded",
	} {
		in := Input{Command: command.Parse(strings.Fields("apply -f ing.yaml")), ExitCode: 1, Stderr: prefix + cause}
		d := Diagnose(in, cat)
		if d.Entry == nil || d.Entry.ID != "admission.webhook-unreachable" {
			t.Errorf("%s : entrée %v au lieu de admission.webhook-unreachable", cause, d.Entry)
			continue
		}
		if d.Captures["service"] != "ingress-nginx-controller-admission" || d.Captures["webhook_ns"] != "ingress-nginx" || d.Origin != OriginCluster {
			t.Errorf("%s : captures %v, origine %s", cause, d.Captures, d.Origin)
		}
	}
}

// Feature 004 : « wait » expiré, même message pour une condition et pour --for=delete ; seule
// l'option --for choisit la leçon (when_flag), et le diagnostic de l'objet est proposé quand
// diagnose prend son type en charge.
func TestWaitTimeoutByFlag(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ args, stderr, entry, target string }{
		{"wait --for=condition=Ready pod/web -n t13", "error: timed out waiting for the condition on pods/web", "wait.timeout", "pod/web"},
		{"wait --for=condition=Available deployment/api", "error: timed out waiting for the condition on deployments/api", "wait.timeout", "deployment/api"},
		{"wait --for=condition=Ready configmap/c", "error: timed out waiting for the condition on configmaps/c", "wait.timeout", ""},
		{"wait --for=delete configmap/reste", "error: timed out waiting for the condition on configmaps/reste", "wait.delete-timeout", ""},
	} {
		d := Diagnose(Input{Command: command.Parse(strings.Fields(tc.args)), ExitCode: 1, Stderr: tc.stderr}, cat)
		if d.Entry == nil || d.Entry.ID != tc.entry || d.Captures["diag_target"] != tc.target {
			t.Errorf("%s : entrée %v, cible %q ; attendu %s, %q", tc.args, d.Entry, d.Captures["diag_target"], tc.entry, tc.target)
		}
	}
}

// Feature 004 : la liste des pods d'un drain refusé s'arrête avant « , continuing command… ».
func TestDrainPodsCapture(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, entry, pods string }{
		{"drain-unmanaged-pods", "drain.unmanaged-pods", "drain-unmanaged-pods/solo"},
		{"drain-daemonset-pods", "drain.daemonset-pods", "kube-system/kindnet-tj8tw, kube-system/kube-proxy-8wbxr"},
	} {
		c := loadCase(t, tc.file)
		d := diagnoseCase(c, cat)
		if d.Entry == nil || d.Entry.ID != tc.entry || d.Captures["pods"] != tc.pods || d.Captures["node"] != "telemach-cluster-worker" {
			t.Errorf("%s : entrée %v, pods %q, nœud %q", tc.file, d.Entry, d.Captures["pods"], d.Captures["node"])
		}
	}
}

// loadCase renvoie un cas du corpus testdata/errors/ par son nom.
func loadCase(t *testing.T, name string) corpusCase {
	t.Helper()
	for _, c := range loadCorpus(t) {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cas %q absent du corpus", name)
	return corpusCase{}
}
