package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, name, content string) *File {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, issues := Read(p)
	if len(issues) != 0 {
		t.Fatalf("lecture : %+v", issues)
	}
	return f
}

const twoDocs = `apiVersion: v1
kind: ConfigMap
metadata:
  name: c
data:
  a: b
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
      - name: web
        image: nginxinc/nginx-unprivileged:1.30-alpine
        imagePullPolicyy: Always
`

func TestDocumentsAndPaths(t *testing.T) {
	f := writeManifest(t, "all.yaml", twoDocs)
	if len(f.Documents) != 2 {
		t.Fatalf("2 documents attendus, %d trouvés", len(f.Documents))
	}
	d := f.Documents[1]
	if d.Index != 2 || d.StartLine != 8 || d.Kind != "Deployment" || d.Name != "web" {
		t.Errorf("document 2 : %+v", d)
	}
	n := Paths(d.Root)["spec.template.spec.containers[0].image"]
	if n == nil || n.Line != 21 || n.Column != 16 {
		t.Errorf("chemin → nœud : %+v", n)
	}
}

func TestLocateMessages(t *testing.T) {
	f := writeManifest(t, "all.yaml", twoDocs)
	tests := []struct {
		name, stderr string
		line, col    int
	}{
		{"champ inconnu (chemin avec indices)", `Error from server (BadRequest): error when creating "all.yaml": Deployment in version "v1" cannot be handled as a Deployment: strict decoding error: unknown field "spec.template.spec.containers[0].imagePullPolicyy"`, 22, 9},
		{"chemin sans indices (json)", `Error from server (BadRequest): error when creating "all.yaml": Deployment in version "v1" cannot be handled as a Deployment: json: cannot unmarshal string into Go struct field Container.spec.template.spec.containers.image of type int`, 21, 16},
		{"is invalid", "The Deployment \"web\" is invalid: spec.template.metadata.labels: Invalid value: {\"app\":\"api\"}: `selector` does not match template `labels`", 17, 7},
		{"no matches for kind", `error: resource mapping not found for name: "web" namespace: "" from "all.yaml": no matches for kind "Deployment" in version "apps/v1beta9"`, 9, 7},
		{"champ obligatoire absent → parent", `The Deployment "web" is invalid: spec.template.spec.securityContext.runAsUser: Required value`, 18, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locs := Locate(tt.stderr, []*File{f})
			if len(locs) == 0 {
				t.Fatal("aucune position trouvée")
			}
			if locs[0].Line != tt.line || locs[0].Column != tt.col || locs[0].Document != 2 {
				t.Errorf("position %d:%d (document %d), attendu %d:%d (document 2)", locs[0].Line, locs[0].Column, locs[0].Document, tt.line, tt.col)
			}
		})
	}
}

// La ligne d'une erreur YAML est donnée par kubectl relativement au document : elle est
// ramenée au fichier.
func TestLocateYAMLLineRelativeToDocument(t *testing.T) {
	content := strings.Replace(twoDocs, "        imagePullPolicyy: Always\n", "\timagePullPolicy: Always\n", 1)
	f := writeManifest(t, "all.yaml", content)
	locs := Locate(`error: error parsing all.yaml: error converting YAML to JSON: yaml: line 15: found character that cannot start any token`, []*File{f})
	if len(locs) != 1 || locs[0].Line != 22 {
		t.Fatalf("ligne 15 du document 2 = ligne 22 du fichier, obtenu %+v", locs)
	}
}

func TestLocateUnknownMessage(t *testing.T) {
	f := writeManifest(t, "all.yaml", twoDocs)
	if locs := Locate("error: something totally new", []*File{f}); len(locs) != 0 {
		t.Errorf("aucune position attendue : %+v", locs)
	}
}

func FuzzLocate(fz *testing.F) {
	for _, seed := range []string{
		`error when creating "all.yaml": Deployment in version "v1" cannot be handled as a Deployment: strict decoding error: unknown field "spec.x[0].y"`,
		`The Pod "web" is invalid: spec.containers[99].name: Required value`,
		`error parsing all.yaml: error converting YAML to JSON: yaml: line 999999: x`,
		`resource mapping not found for name: "" namespace: "" from "": no matches for kind "" in version ""`,
	} {
		fz.Add(seed)
	}
	dir := fz.TempDir()
	p := filepath.Join(dir, "all.yaml")
	if err := os.WriteFile(p, []byte(twoDocs), 0o600); err != nil {
		fz.Fatal(err)
	}
	f, _ := Read(p)
	fz.Fuzz(func(t *testing.T, msg string) {
		for _, l := range Locate(msg, []*File{f}) {
			if l.Line < 0 || l.Line > len(f.Lines)+1 {
				t.Fatalf("ligne hors du fichier : %+v", l)
			}
		}
	})
}
