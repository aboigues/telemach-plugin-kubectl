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
