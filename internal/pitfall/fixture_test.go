package pitfall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/testcorpus"
)

const objectsCorpus = "../../testdata/objects"

// fixture est le faux lecteur du corpus d'objets (internal/testcorpus), vu comme un Reader.
type fixture = testcorpus.ObjectFixture

var _ Reader = (*fixture)(nil)

// expectation est le contenu de testdata/objects/<cas>/expected.yaml.
type expectation struct {
	Kind    Kind              `yaml:"kind"`
	Name    string            `yaml:"name"`
	Primary string            `yaml:"primary"`
	Links   map[string]string `yaml:"links"`
	Closest []string          `yaml:"closest"`
	Phase   string            `yaml:"phase"`
	// Failing est la raison du pod en difficulté lié à l'objet (feature 001), s'il y en a un.
	Failing string `yaml:"failing"`
	Also    *struct {
		Name    string `yaml:"name"`
		Primary string `yaml:"primary"`
	} `yaml:"also"`
}

// objectCase est un cas du corpus d'objets, variante de version comprise (<cas>@1.35 reprend
// les attentes du cas de base).
type objectCase struct {
	name string
	dir  string
	exp  expectation
	at   time.Time
}

func loadObjectCases(t *testing.T, kind Kind) []objectCase {
	t.Helper()
	dirs, _ := filepath.Glob(filepath.Join(objectsCorpus, "*"))
	var out []objectCase
	for _, dir := range dirs {
		base := filepath.Base(dir)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() || base == "scenarios" || base == "configmap-dates" {
			continue
		}
		expDir := dir
		if i := strings.Index(base, "@"); i >= 0 {
			expDir = filepath.Join(objectsCorpus, base[:i])
		}
		// expected-<type>.yaml quand un même relevé sert à plusieurs types d'objets.
		data, err := os.ReadFile(filepath.Join(expDir, "expected-"+string(kind)+".yaml")) //nolint:gosec // corpus du dépôt
		if err != nil {
			data, err = os.ReadFile(filepath.Join(expDir, "expected.yaml")) //nolint:gosec // corpus du dépôt
		}
		if err != nil {
			continue // cas d'une autre histoire, pas encore attendu
		}
		var exp expectation
		if err := yaml.Unmarshal(data, &exp); err != nil {
			t.Fatalf("%s/expected.yaml : %v", expDir, err)
		}
		if exp.Kind != kind {
			continue
		}
		at, _ := os.ReadFile(filepath.Join(dir, "captured-at.txt")) //nolint:gosec // corpus du dépôt
		when, _ := time.Parse(time.RFC3339, strings.TrimSpace(string(at)))
		out = append(out, objectCase{name: base, dir: dir, exp: exp, at: when})
	}
	if len(out) == 0 {
		t.Fatalf("aucun cas %s dans le corpus (D-LL-009)", kind)
	}
	return out
}

func testContext(t *testing.T, f *fixture, at time.Time) Context {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return Context{Reader: f, Scope: kuberead.Scope{Namespace: "demo"}, Catalog: cat, Now: at}
}

// linkStates résume les maillons d'un instantané.
func linkStates(s ObjectSnapshot) map[string]string {
	out := map[string]string{}
	for _, l := range s.Links {
		out[string(l.Step)] = string(l.State)
	}
	return out
}

func timeOf(t *testing.T, dir string) time.Time {
	t.Helper()
	at, _ := os.ReadFile(filepath.Join(dir, "captured-at.txt")) //nolint:gosec // corpus du dépôt
	when, err := time.Parse(time.RFC3339, strings.TrimSpace(string(at)))
	if err != nil {
		t.Fatal(err)
	}
	return when
}

func goDump(v any) string { return fmt.Sprintf("%#v", v) }

// variant copie un cas relevé dans un dossier temporaire en remplaçant certains fichiers : un
// contenu vide supprime le fichier (lecture « introuvable »). Il sert aux cas qu'un cluster
// kind ne produit pas (annotation dépréciée, plus de 10 backends).
func variant(t *testing.T, base string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir(filepath.Join(objectsCorpus, base))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(objectsCorpus, base, e.Name())) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if content == "" {
			_ = os.Remove(path)
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
