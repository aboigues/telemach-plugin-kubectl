package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v3"
)

func TestRedactCorpus(t *testing.T) {
	inputs, err := filepath.Glob("../../testdata/secrets/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, in := range inputs {
		if strings.HasSuffix(in, ".expected.txt") {
			continue
		}
		count++
		name := strings.TrimSuffix(filepath.Base(in), ".txt")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(in, ".txt") + ".expected.txt")
			if err != nil {
				t.Fatalf("fichier attendu manquant : %v", err)
			}
			if diff := cmp.Diff(string(want), Redact(string(raw))); diff != "" {
				t.Errorf("Redact (-attendu +obtenu) :\n%s", diff)
			}
		})
	}
	if count < 20 {
		t.Fatalf("le corpus doit contenir au moins 20 cas, %d trouvés", count)
	}
}

func TestRedactArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"--token séparé", []string{"get", "pods", "--token", "x"}, []string{"get", "pods", "--token", Mask}},
		{"--token=", []string{"get", "pods", "--token=x"}, []string{"get", "pods", "--token=" + Mask}},
		{"--password", []string{"get", "po", "--password", "x"}, []string{"get", "po", "--password", Mask}},
		{"--from-literal=", []string{"create", "secret", "generic", "s", "--from-literal=password=x"}, []string{"create", "secret", "generic", "s", "--from-literal=password=" + Mask}},
		{"--from-literal séparé", []string{"create", "secret", "generic", "s", "--from-literal", "password=x"}, []string{"create", "secret", "generic", "s", "--from-literal", "password=" + Mask}},
		{"--docker-password=", []string{"create", "secret", "docker-registry", "r", "--docker-password=x"}, []string{"create", "secret", "docker-registry", "r", "--docker-password=" + Mask}},
		{"chemin --client-key conservé", []string{"get", "pods", "--client-key", "/tmp/k.key"}, []string{"get", "pods", "--client-key", "/tmp/k.key"}},
		{"options ordinaires conservées", []string{"get", "pods", "-n", "demo", "--context", "kind-x"}, []string{"get", "pods", "-n", "demo", "--context", "kind-x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, RedactArgs(tt.in)); diff != "" {
				t.Errorf("RedactArgs (-attendu +obtenu) :\n%s", diff)
			}
		})
	}
}

func TestRedactNode(t *testing.T) {
	src := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: '{"x":"y"}'
spec:
  template:
    spec:
      containers:
      - name: web
        image: nginxinc/nginx-unprivileged:1.30-alpine
        env:
        - name: DB_PASSWORD
          value: s3cret-fake
        - name: LOG_LEVEL
          value: debug
        - name: API_TOKEN
          valueFrom:
            secretKeyRef:
              name: api
              key: token
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	got := RedactNode(&doc)
	want := []Position{{Line: 6, Column: 55}, {Line: 15, Column: 18}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("positions masquées (-attendu +obtenu) :\n%s", diff)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"s3cret-fake", `{"x":"y"}`} {
		if strings.Contains(string(out), leak) {
			t.Errorf("valeur sensible %q encore présente :\n%s", leak, out)
		}
	}
	for _, kept := range []string{"debug", "key: token", "name: api"} {
		if !strings.Contains(string(out), kept) {
			t.Errorf("valeur non sensible %q perdue :\n%s", kept, out)
		}
	}
}

func TestRedactNodeSecret(t *testing.T) {
	src := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: db\ndata:\n  password: czNjcmV0\nstringData:\n  user: admin\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	if got := RedactNode(&doc); len(got) != 2 {
		t.Fatalf("2 valeurs masquées attendues, %d obtenues : %v", len(got), got)
	}
	out, _ := yaml.Marshal(&doc)
	if strings.Contains(string(out), "czNjcmV0") || strings.Contains(string(out), "admin") {
		t.Errorf("données du Secret non masquées :\n%s", out)
	}
	if !strings.Contains(string(out), "password:") || !strings.Contains(string(out), "name: db") {
		t.Errorf("les noms des entrées et le nom du Secret doivent rester visibles :\n%s", out)
	}
}

func TestIsSensitiveName(t *testing.T) {
	for _, n := range []string{"DB_PASSWORD", "api-key", "client_secret", "TLS_CERT", "Auth"} {
		if !IsSensitiveName(n) {
			t.Errorf("%q devrait être sensible", n)
		}
	}
	for _, n := range []string{"LOG_LEVEL", "PORT", "TZ"} {
		if IsSensitiveName(n) {
			t.Errorf("%q ne devrait pas être sensible", n)
		}
	}
}

func FuzzRedact(f *testing.F) {
	for _, seed := range []string{"", "A_PASSWORD=x", "postgres://u:p@h", "--token=x", "kind: Secret\ndata:\n  a: b\n", "- name: TOKEN\n  value: v\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		once := Redact(in)
		if twice := Redact(once); twice != once {
			t.Fatalf("Redact n'est pas idempotent :\n1: %q\n2: %q", once, twice)
		}
	})
}

// Issue #11 : les champs du schéma Kubernetes dont le nom contient « key », « auth »… sans
// être secrets restent visibles, dans le nœud YAML comme dans le texte.
func TestStructuralFieldsKept(t *testing.T) {
	src := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              matchLabelKeys: [pod-template-hash]
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values: [api]
      tolerations:
      - key: dedie
        operator: Equal
        value: gpu
        effect: NoSchedule
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
`
	if got := Redact(src); got != src {
		t.Errorf("texte modifié :\n%s", got)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	if got := RedactNode(&doc); len(got) != 0 {
		t.Errorf("aucune valeur masquée attendue, obtenu %v", got)
	}
	// Un vrai nom sensible reste masqué.
	if got := Redact("  apiKey: abc123\n"); strings.Contains(got, "abc123") {
		t.Errorf("apiKey non masqué : %q", got)
	}
}
