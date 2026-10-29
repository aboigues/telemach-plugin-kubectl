package catalog

import (
	"os"
	"path/filepath"
	"regexp"
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
		{"origine inconnue", "applies: [get]", "applies: [get]\n  origin: application", "origin inconnue"},
		{"when_flag invalide", "applies: [get]", "applies: [get]\n  when_flag: for", "when_flag invalide"},
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
	case len(e.Pitfalls) > 0:
		if _, err := os.Stat(filepath.Join(root, "objects", name)); err != nil {
			t.Errorf("%s : cas %q absent de testdata/objects/", e.ID, name)
		}
	case e.Fallback == FallbackManifestIssue:
		if _, err := os.Stat(filepath.Join(root, "manifests", name)); err != nil {
			t.Errorf("%s : cas %q absent de testdata/manifests/", e.ID, name)
		}
	case len(e.PodReasons) > 0 || len(e.LogPatterns) > 0:
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

// TestExercisesAreSafe (T068) : chaque exercice se fait dans un namespace que l'apprenant crée
// lui-même, jamais dans default ni kube-system.
func TestExercisesAreSafe(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Entries {
		ex := e.Deepen.Exercise
		if !strings.Contains(ex, "kubectl create namespace") {
			t.Errorf("%s : l'exercice doit faire créer un namespace d'exercice (« kubectl create namespace »)", e.ID)
		}
		for _, forbidden := range []string{"-n default", "-n kube-system", "--namespace=default", "--namespace kube-system", "delete namespace default", "delete namespace kube-system"} {
			if strings.Contains(ex, forbidden) {
				t.Errorf("%s : l'exercice ne doit pas viser %q", e.ID, forbidden)
			}
		}
	}
}

// pitfallEntry est une leçon de piège silencieux (feature 002) : reconnue par son identifiant
// de piège, sans motif.
const pitfallEntry = `
- id: service.no-matching-pods
  category: service
  pitfalls: [service.no-matching-pods]
  concept:
    name: label selector
    definition: Un Service envoie le trafic aux pods dont les labels satisfont son selector.
  summary: Le Service « {{.service}} » ne désigne aucun pod.
  cause: Aucun pod du namespace ne porte les labels du selector.
  fixes:
    - description: Comparer les labels des pods au selector du Service.
      command: '{{.kubectl}} get pods --show-labels'
      safe: true
  deepen:
    explanation: Un Service ne connaît pas ses pods par leur nom, seulement par leurs labels.
    example: kubectl get pods --show-labels
    exercise: Crée un namespace d'exercice avec kubectl create namespace exo, puis un Service sans pod.
  doc_url: https://kubernetes.io/docs/concepts/services-networking/service/
  tests: [selector-typo]
`

// Feature 002 (T018) : une entrée de piège est valide sans motif ; un piège n'est couvert
// qu'une fois ; aucune commande ne propose l'API Endpoints dépréciée (FR-032).
func TestPitfallEntries(t *testing.T) {
	c, err := Load(fstest.MapFS{"x.yaml": {Data: []byte(pitfallEntry)}})
	if err != nil {
		t.Fatalf("entrée de piège refusée : %v", err)
	}
	if e, ok := c.ByPitfall("service.no-matching-pods"); !ok || e.ID != "service.no-matching-pods" {
		t.Error("ByPitfall doit retrouver l'entrée")
	}
	if _, ok := c.ByPitfall("service.inconnu"); ok {
		t.Error("piège inconnu : aucune entrée attendue")
	}
	dup := pitfallEntry + strings.Replace(pitfallEntry, "id: service.no-matching-pods", "id: service.other", 1)
	if err := load(t, dup); err == nil || !strings.Contains(err.Error(), "déjà couvert") {
		t.Errorf("piège couvert deux fois : erreur attendue, obtenu %v", err)
	}
	endpoints := strings.Replace(pitfallEntry, "get pods --show-labels'", "get endpoints'", 1)
	if err := load(t, endpoints); err == nil || !strings.Contains(err.Error(), "EndpointSlice") {
		t.Errorf("get endpoints : erreur attendue (FR-032), obtenu %v", err)
	}
}

// FR-028 : toute entrée dont une correction ou un texte mentionne une suppression forcée
// (--force --grace-period=0) ou la suppression des finalizers porte un anti_pattern qui le
// présente comme tel.
func TestForcedDeletionIsAntiPattern(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	forced := regexp.MustCompile(`--grace-period[= ]0|--force\b|finalizers`)
	forcedDeletion := regexp.MustCompile(`--grace-period[= ]0|finalizers|delete [^\n]*--force\b`)
	for _, e := range c.Entries {
		texts := []string{e.Summary, e.Cause, e.Deepen.Explanation, e.Deepen.Example, e.Deepen.Exercise}
		for _, f := range e.Fixes {
			texts = append(texts, f.Description, f.Command, f.Manifest)
		}
		for _, text := range texts {
			if forced.MatchString(text) && e.AntiPattern == nil {
				t.Errorf("%s : suppression forcée ou finalizers mentionnés sans anti_pattern : %q", e.ID, text)
			}
		}
		// La démarche : chercher d'abord ce qui bloque (finalizers, nœud, volume). Elle vaut
		// pour la suppression forcée d'un objet (delete --force, --grace-period=0, finalizers) ;
		// « drain --force » (feature 004) ne touche pas aux finalizers.
		if e.AntiPattern != nil && forcedDeletion.MatchString(e.AntiPattern.Description) &&
			(!strings.Contains(e.AntiPattern.Description, "finalizers") || !strings.Contains(e.AntiPattern.Description, "nœud")) {
			t.Errorf("%s : l'anti_pattern doit proposer d'identifier ce qui bloque (FR-028)", e.ID)
		}
	}
}
