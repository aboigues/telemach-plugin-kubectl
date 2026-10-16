package schema

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func parse(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func findings(t *testing.T, version, src string) []Finding {
	t.Helper()
	set, _, err := Select(version)
	if err != nil {
		t.Fatal(err)
	}
	root := parse(t, src)
	var api, kind string
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "apiVersion":
			api = root.Content[i+1].Value
		case "kind":
			kind = root.Content[i+1].Value
		}
	}
	f, ok := set.Validate(root, api, kind)
	if !ok {
		t.Fatalf("ressource %s/%s non couverte", api, kind)
	}
	return f
}

func TestSelect(t *testing.T) {
	tests := []struct {
		in       string
		want     string
		fallback bool
	}{
		{"v1.36.5", "1.36", false},
		{"v1.35.9", "1.35", false},
		{"1.37.1", "1.37", false},
		{"v1.32.3", "1.37", true},
		{"", "1.37", true},
		{"pas-une-version", "1.37", true},
	}
	for _, tt := range tests {
		s, fb, err := Select(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		if s.Version != tt.want || fb != tt.fallback {
			t.Errorf("Select(%q) = %s, %v ; attendu %s, %v", tt.in, s.Version, fb, tt.want, tt.fallback)
		}
	}
}

const deploy = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: nginxinc/nginx-unprivileged:1.30-alpine
        ports:
        - containerPort: 8080
`

func TestValidManifestAllVersions(t *testing.T) {
	for _, v := range Supported {
		if f := findings(t, v, deploy); len(f) != 0 {
			t.Errorf("règles %s : aucun problème attendu, obtenu %+v", v, f)
		}
	}
}

func TestFindings(t *testing.T) {
	tests := []struct {
		name, src              string
		kind, path, suggestion string
		line, col              int
	}{
		{"champ inconnu", replace(deploy, "        image:", "        imagePullPolicyy: Always\n        image:"), UnknownField, "spec.template.spec.containers[0].imagePullPolicyy", "imagePullPolicy", 17, 9},
		{"mauvais niveau", replace(deploy, "        ports:\n        - containerPort: 8080\n", "        containerPort: 8080\n"), WrongLevel, "spec.template.spec.containers[0].containerPort", "ports[].containerPort", 18, 9},
		{"entier entre guillemets", replace(deploy, "replicas: 2", `replicas: "2"`), WrongType, "spec.replicas", "2", 6, 13},
		{"objet au lieu de liste", replace(deploy, "      containers:\n      - name: web\n        image", "      containers:\n        name: web\n        image"), WrongType, "spec.template.spec.containers", "", 16, 9},
		{"selector obligatoire", replace(deploy, "  selector:\n    matchLabels:\n      app: web\n", ""), RequiredMissing, "spec.selector", "selector", 5, 1},
		{"containers obligatoire", "apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\nspec:\n  restartPolicy: Always\n", RequiredMissing, "spec.containers", "containers", 5, 1},
		{"targetPort int-or-string accepté, port en texte refusé", "apiVersion: v1\nkind: Service\nmetadata:\n  name: web\nspec:\n  ports:\n  - port: \"80\"\n    targetPort: http\n", WrongType, "spec.ports[0].port", "80", 7, 11},
		{"champ racine inconnu", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\ndat:\n  a: b\n", UnknownField, "dat", "data", 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := findings(t, "1.37", tt.src)
			if len(f) != 1 {
				t.Fatalf("un seul problème attendu, obtenu %+v", f)
			}
			got := f[0]
			if got.Kind != tt.kind || got.Path != tt.path || got.Suggestion != tt.suggestion || got.Node.Line != tt.line || got.Node.Column != tt.col {
				t.Errorf("obtenu %s %q (suggestion %q) en %d:%d ; attendu %s %q (%q) en %d:%d",
					got.Kind, got.Path, got.Suggestion, got.Node.Line, got.Node.Column, tt.kind, tt.path, tt.suggestion, tt.line, tt.col)
			}
		})
	}
}

func TestPreserveUnknownAndQuantities(t *testing.T) {
	src := replace(deploy, "        ports:", "        resources:\n          limits: {cpu: 500m, memory: 128Mi}\n          requests: {cpu: 1, memory: \"64Mi\"}\n        ports:")
	if f := findings(t, "1.37", src); len(f) != 0 {
		t.Errorf("les quantités (texte ou nombre) sont acceptées : %+v", f)
	}
}

func TestKnownKind(t *testing.T) {
	s, _, _ := Select("1.37")
	if api, ok := s.KnownKind("Deployment"); !ok || api != "apps/v1" {
		t.Errorf("KnownKind(Deployment) = %q, %v", api, ok)
	}
	if _, ok := s.KnownKind("Widget"); ok {
		t.Error("Widget n'est pas couvert")
	}
	if len(s.KindNames()) != 12 {
		t.Errorf("12 ressources couvertes attendues, %d trouvées", len(s.KindNames()))
	}
}

func replace(s, old, repl string) string {
	if !strings.Contains(s, old) {
		panic("remplacement impossible : " + old)
	}
	return strings.Replace(s, old, repl, 1)
}
