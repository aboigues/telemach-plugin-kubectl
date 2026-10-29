package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fromStdin analyse un contenu passé par l'entrée standard (check -f -).
func fromStdin(content []byte) Result {
	return AnalyzeInput([]string{"-"}, false, "v1.37.2", bytes.NewReader(content))
}

// SC-009 (feature 004, US5) : pour un même contenu, l'entrée standard donne exactement les
// mêmes problèmes que le fichier, désignés par <stdin>. Seul helm-values dépend du nom du
// fichier (values.yaml, Chart.yaml) : il est exclu.
func TestStdinSameAsFile(t *testing.T) {
	for _, c := range loadManifestCases(t) {
		if c.exp.Kind == KindHelmValues || c.exp.Kind == KindFileNotFound {
			continue
		}
		path := filepath.Join(c.dir, c.exp.File)
		data, err := os.ReadFile(path) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatalf("%s : %v", c.name, err)
		}
		fromFile := AnalyzeInput([]string{path}, false, "v1.37.2", nil)
		got := fromStdin(data)
		if len(got.Issues) != len(fromFile.Issues) {
			t.Errorf("%s : %d problèmes par l'entrée standard, %d par le fichier", c.name, len(got.Issues), len(fromFile.Issues))
			continue
		}
		for i, is := range got.Issues {
			want := fromFile.Issues[i]
			if is.File != StdinPath || is.Kind != want.Kind || is.Line != want.Line || is.Column != want.Column || is.FieldPath != want.FieldPath {
				t.Errorf("%s : %s %s:%d:%d %s, attendu %s %s:%d:%d %s", c.name,
					is.Kind, is.File, is.Line, is.Column, is.FieldPath, want.Kind, StdinPath, want.Line, want.Column, want.FieldPath)
			}
		}
	}
}

// Un rendu de « helm template » rattache chaque problème au template d'origine (# Source:).
func TestStdinHelmSource(t *testing.T) {
	data, err := os.ReadFile("../../testdata/manifests/helm-rendered/rendu.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r := fromStdin(data)
	if len(r.Issues) != 1 || r.Issues[0].Source != "telemarche/templates/api.yaml" || r.Issues[0].Line != 36 {
		t.Errorf("problème rattaché à telemarche/templates/api.yaml ligne 36 attendu : %+v", r.Issues)
	}
	if r.Documents != 2 {
		t.Errorf("2 documents attendus, %d", r.Documents)
	}
}

// Entrée vide, trop grosse ou mal encodée : problème bloquant, comme pour un fichier.
func TestStdinLimits(t *testing.T) {
	big := append([]byte("data: "), bytes.Repeat([]byte("a"), MaxFileSize)...)
	for name, tc := range map[string]struct {
		content []byte
		kind    string
	}{
		"vide":      {nil, KindNoManifestFile},
		"blancs":    {[]byte("\n# commentaire\n\n"), KindNoManifestFile},
		"trop gros": {big, KindFileUnreadable},
		"pas UTF-8": {[]byte("kind: Pod\nname: caf\xe9\n"), KindNotUTF8},
	} {
		r := fromStdin(tc.content)
		if len(r.Issues) != 1 || r.Issues[0].Kind != tc.kind || !r.Issues[0].Blocking || r.Issues[0].File != StdinPath {
			t.Errorf("%s : un problème bloquant %s sur <stdin> attendu : %+v", name, tc.kind, r.Issues)
			continue
		}
		if tc.kind != KindNotUTF8 && !strings.Contains(r.Issues[0].Detail, "entrée standard") {
			t.Errorf("%s : le détail doit parler de l'entrée standard : %q", name, r.Issues[0].Detail)
		}
	}
}
