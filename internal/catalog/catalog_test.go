package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// validEntry est une entrée minimale conforme, utilisée comme base des cas d'erreur.
const validEntry = `
- id: cmdline.unknown-resource-type
  category: cmdline
  applies: [get]
  patterns: ['the server doesn''t have a resource type "(?P<type>[^"]+)"']
  concept:
    name: resource type
    definition: Chaque objet Kubernetes a un type, par exemple pod ou deployment.
  summary: Le type de ressource « {{.type}} » n'existe pas.
  cause: Le type tapé ne correspond à aucun type publié par le cluster.
  fixes:
    - description: Lister les types de ressources connus du cluster.
      command: '{{.kubectl}} api-resources'
      safe: true
  deepen:
    explanation: Le serveur API publie la liste de ses types de ressources.
    example: kubectl api-resources --namespaced=true
    exercise: Crée un namespace d'exercice avec kubectl create namespace exo, puis liste ses pods.
  doc_url: https://kubernetes.io/docs/reference/kubectl/
  tests: [get-unknown-type]
`

func load(t *testing.T, content string) error {
	t.Helper()
	_, err := Load(fstest.MapFS{"x.yaml": {Data: []byte(content)}})
	return err
}

func TestLoadValidEntry(t *testing.T) {
	if err := load(t, validEntry); err != nil {
		t.Fatalf("entrée valide refusée : %v", err)
	}
}

func TestValidationRules(t *testing.T) {
	tests := []struct {
		name, from, to, wantErr string
	}{
		{"identifiant invalide", "id: cmdline.unknown-resource-type", "id: Cmd_Type", "identifiant invalide"},
		{"catégorie inconnue", "category: cmdline", "category: réseau", "catégorie inconnue"},
		{"motif invalide", `patterns: ['the server doesn''t have a resource type "(?P<type>[^"]+)"']`, "patterns: ['a(b']", "motif invalide"},
		{"résumé trop long", "summary: Le type de ressource « {{.type}} » n'existe pas.", "summary: " + strings.Repeat("é", 161), "résumé trop long"},
		{"cause manquante", "cause: Le type tapé ne correspond à aucun type publié par le cluster.", "cause: ''", "cause obligatoire"},
		{"doc hors kubernetes.io", "https://kubernetes.io/docs/reference/kubectl/", "https://example.com/", "doc_url"},
		{"tests manquants", "tests: [get-unknown-type]", "tests: []", "cas de test"},
		{"texte en anglais", "cause: Le type tapé ne correspond à aucun type publié par le cluster.", "cause: The type is unknown to the cluster.", "français"},
		{"champ inconnu", "doc_url:", "unknown_field: x\n  doc_url:", "unknown_field"},
		{"correction non sûre sans anti-pattern", "safe: true", "safe: false", "anti_pattern"},
		{"fallback inconnu", "applies: [get]", "applies: [get]\n  fallback: autre", "fallback inconnu"},
		{"manifest-issue sans issue_kinds", "applies: [get]", "applies: [get]\n  fallback: manifest-issue", "issue_kinds"},
		{"aucun motif", `patterns: ['the server doesn''t have a resource type "(?P<type>[^"]+)"']`, "patterns: []", "au moins un motif"},
		{"approfondissement incomplet", "exercise: Crée", "exercice: Crée", "exercice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := strings.Replace(validEntry, tt.from, tt.to, 1)
			if content == validEntry {
				t.Fatalf("remplacement sans effet : %q", tt.from)
			}
			err := load(t, content)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("erreur contenant %q attendue, obtenu %v", tt.wantErr, err)
			}
		})
	}
}

func TestDuplicateID(t *testing.T) {
	if err := load(t, validEntry+validEntry); err == nil || !strings.Contains(err.Error(), "double") {
		t.Errorf("identifiant en double non détecté : %v", err)
	}
}

func TestRender(t *testing.T) {
	got := Render("{{.kubectl}} get {{.type}} {{.absent}}", map[string]string{"kubectl": "k", "type": "pods"})
	if got != "k get pods …" {
		t.Errorf("Render = %q", got)
	}
}

// TestEmbeddedCatalog vérifie le catalogue livré : règles du modèle de données, langue
// (SC-005) et présence de chaque cas de test cité dans le corpus testdata/.
func TestEmbeddedCatalog(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatalf("catalogue embarqué invalide :\n%v", err)
	}
	for _, e := range c.Entries {
		for _, name := range e.Tests {
			checkTestCase(t, e, name)
		}
	}
}

// checkTestCase vérifie qu'un cas cité existe et, pour une erreur de commande, qu'au moins
// un motif de l'entrée reconnaît son message.
func checkTestCase(t *testing.T, e *Entry, name string) {
	t.Helper()
	root := "../../testdata"
	switch {
	case e.Fallback == FallbackManifestIssue:
		if _, err := os.Stat(filepath.Join(root, "manifests", name)); err != nil {
			t.Errorf("%s : cas %q absent de testdata/manifests/", e.ID, name)
		}
	case len(e.PodReasons) > 0:
		if _, err := os.Stat(filepath.Join(root, "pods", name)); err != nil {
			t.Errorf("%s : cas %q absent de testdata/pods/", e.ID, name)
		}
	default:
		data, err := os.ReadFile(filepath.Join(root, "errors", name, "stderr.txt"))
		if err != nil && e.Category == "manifest" {
			data, err = os.ReadFile(filepath.Join(root, "manifests", name, "stderr.txt"))
		}
		if err != nil {
			t.Errorf("%s : cas %q absent de testdata/errors/ (et testdata/manifests/) : %v", e.ID, name, err)
			return
		}
		if len(e.Patterns) == 0 {
			return
		}
		for _, re := range e.Compiled() {
			if re.Match(data) {
				return
			}
		}
		t.Errorf("%s : aucun motif ne reconnaît testdata/errors/%s/stderr.txt", e.ID, name)
	}
}
