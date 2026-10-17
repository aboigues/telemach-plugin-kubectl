package kuberead

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeKubectl journalise chacun de ses arguments sur une ligne (séparateur ␞ entre deux appels)
// puis répond selon FAKE_MODE.
const fakeKubectl = `#!/bin/sh
for a in "$@"; do printf '%s\n' "$a" >> "$CALL_LOG"; done
printf '␞\n' >> "$CALL_LOG"
case "$FAKE_MODE" in
forbidden) echo 'Error from server (Forbidden): events is forbidden: User "u" cannot list resource "events"' >&2; exit 1 ;;
refused) echo 'The connection to the server localhost:8080 was refused - did you specify the right host or port?' >&2; exit 1 ;;
esac
case "$*" in
*logs*) printf 'ligne 1\nDB_PASSWORD=s3cret-fake\n' ;;
*) printf '{"items":[],"metadata":{"name":"x"},"spec":{},"status":{}}' ;;
esac
`

func setup(t *testing.T, mode string) (*Reader, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fakewrap")
	if err := os.WriteFile(bin, []byte(fakeKubectl), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.log")
	t.Setenv("CALL_LOG", log)
	t.Setenv("FAKE_MODE", mode)
	// TELEMACH_KUBECTL peut contenir plusieurs mots (« microk8s kubectl ») : ils sont découpés
	// sans shell.
	r := New(bin+" kubectl", Scope{Context: "kind-x", Namespace: "demo", Kubeconfig: "/tmp/kc"})
	return r, log
}

func calls(t *testing.T, log string) [][]string {
	t.Helper()
	data, err := os.ReadFile(log) //nolint:gosec // fichier de test
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, block := range strings.Split(strings.TrimSuffix(string(data), "␞\n"), "␞\n") {
		out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n"))
	}
	return out
}

var scope = []string{"--context", "kind-x", "-n", "demo", "--kubeconfig", "/tmp/kc", "--request-timeout=10s"}

func withScope(args ...string) []string {
	return append(append([]string{"kubectl"}, args...), scope...)
}

// FR-023, SC-008 : chaque lecture produit exactement la commande prévue par research.md R8.
func TestAllowlist(t *testing.T) {
	r, log := setup(t, "")
	_, _ = r.ListPods()
	_, _ = r.GetPod("web")
	_, _ = r.GetDeployment("web")
	_, _ = r.PodsBySelector("app=web")
	_, _ = r.PodEvents("web")
	_, _ = r.Logs("web", "app", false)
	_, _ = r.Logs("web", "app", true)
	want := [][]string{
		withScope("get", "pods", "-o", "json"),
		withScope("get", "pod", "web", "-o", "json"),
		withScope("get", "deployment", "web", "-o", "json"),
		withScope("get", "pods", "-l", "app=web", "-o", "json"),
		withScope("get", "events", "--field-selector", "involvedObject.kind=Pod,involvedObject.name=web", "-o", "json"),
		withScope("logs", "web", "-c", "app", "--tail=20"),
		withScope("logs", "web", "-c", "app", "--tail=20", "--previous"),
	}
	got := calls(t, log)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("lectures (-attendu +obtenu) :\n%s", diff)
	}
	for _, c := range got {
		if c[1] != "get" && c[1] != "logs" {
			t.Errorf("verbe interdit : %v", c)
		}
		for _, a := range c {
			if a == "secret" || a == "secrets" || a == "-A" || a == "--all-namespaces" {
				t.Errorf("argument interdit %q : %v", a, c)
			}
		}
	}
}

// Les seules fonctions publiques du lecteur sont celles de la liste fermée : en ajouter une
// impose de mettre à jour ce test (et research.md R8).
func TestNoOtherReads(t *testing.T) {
	var methods []string
	rt := reflect.TypeOf(&Reader{})
	for i := 0; i < rt.NumMethod(); i++ {
		methods = append(methods, rt.Method(i).Name)
	}
	sort.Strings(methods)
	want := []string{"GetDeployment", "GetPod", "ListPods", "Logs", "PodEvents", "PodsBySelector"}
	if diff := cmp.Diff(want, methods); diff != "" {
		t.Errorf("méthodes publiques (-attendu +obtenu) :\n%s", diff)
	}
}

// Un nom hostile reste un seul argument : aucune interprétation par un shell.
func TestNoInjection(t *testing.T) {
	r, log := setup(t, "")
	name := "web; touch /tmp/pwned $(id)"
	_, _ = r.GetPod(name)
	got := calls(t, log)[0]
	if got[3] != name {
		t.Errorf("le nom doit rester un argument unique : %q", got)
	}
}

func TestDeniedAndUnreachable(t *testing.T) {
	r, _ := setup(t, "forbidden")
	_, err := r.PodEvents("web")
	var denied *ErrDenied
	if !errors.As(err, &denied) || !strings.Contains(denied.What, "événements") {
		t.Errorf("ErrDenied attendue : %v", err)
	}
	r, _ = setup(t, "refused")
	_, err = r.ListPods()
	var ke *ErrKubectl
	if !errors.As(err, &ke) || !strings.Contains(ke.Stderr, "localhost:8080 was refused") {
		t.Errorf("ErrKubectl attendue : %v", err)
	}
}

func TestLogsRedacted(t *testing.T) {
	r, _ := setup(t, "")
	lines, err := r.Logs("web", "app", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(lines, "\n"), "s3cret-fake") {
		t.Errorf("journal non masqué : %q", lines)
	}
}

// Les valeurs des variables d'environnement et les annotations ne sont jamais décodées.
func TestEnvValuesNeverDecoded(t *testing.T) {
	raw := `{"metadata":{"name":"p","annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{\"password\":\"s3cret-fake\"}"}},
	"spec":{"containers":[{"name":"c","env":[{"name":"DB_PASSWORD","value":"s3cret-fake"},{"name":"T","valueFrom":{"secretKeyRef":{"name":"db","key":"t"}}}]}]}}`
	var p Pod
	if err := jsonUnmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if dump := strings.ToLower(goString(p)); strings.Contains(dump, "s3cret") {
		t.Errorf("valeur sensible gardée en mémoire : %s", dump)
	}
	if p.Spec.Containers[0].Env[1].ValueFrom.SecretKeyRef.Name != "db" {
		t.Error("la référence au Secret doit être conservée")
	}
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func goString(v any) string { return fmt.Sprintf("%#v", v) }
