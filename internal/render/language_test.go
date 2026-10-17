package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/testcorpus"
)

// goldenFiles renvoie tous les rendus de référence (US1 à US4), en couleur et sans couleur.
func goldenFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "golden")
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // fichiers du dépôt
		rel, _ := filepath.Rel(root, path)
		out[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, us := range []string{"us1", "us2", "us3", "us4"} {
		found := false
		for name := range out {
			found = found || strings.HasPrefix(name, us+string(filepath.Separator))
		}
		if !found {
			t.Errorf("aucun rendu de référence pour %s (D-LL-009)", us)
		}
	}
	return out
}

// SC-005 : tout le texte rédigé par le plugin est en français ; seuls restent en anglais les
// termes Kubernetes et ce qui est recopié tel quel (message d'origine, journal, commandes,
// extraits de manifestes).
func TestGoldenFrench(t *testing.T) {
	for name, text := range goldenFiles(t) {
		if words := testcorpus.EnglishWords(text); len(words) > 0 {
			t.Errorf("%s : texte anglais hors termes Kubernetes : %q", name, words)
		}
	}
}

// SC-006 : aucune valeur sensible du corpus n'apparaît en clair dans un rendu de référence.
func TestGoldenNoSecret(t *testing.T) {
	secrets, err := testcorpus.SecretValues(filepath.Join("..", "..", "testdata"))
	if err != nil || len(secrets) == 0 {
		t.Fatalf("corpus de secrets illisible ou vide (D-LL-009) : %v", err)
	}
	for name, text := range goldenFiles(t) {
		if leaks := testcorpus.Leaks(text, secrets); len(leaks) > 0 {
			t.Errorf("%s : secret en clair : %q", name, leaks)
		}
	}
}
