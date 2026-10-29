package catalog

import "regexp"

// Category regroupe les erreurs par domaine Kubernetes.
type Category string

// Catégories autorisées (data-model.md § CatalogEntry).
var Categories = map[Category]bool{
	"cmdline": true, "connection": true, "auth": true, "notfound": true, "namespace": true,
	"manifest": true, "schema": true, "conflict": true, "workload": true, "runtime": true,
	// Feature 002 : pièges silencieux et nouvelles erreurs.
	"service": true, "rollout": true, "config": true, "ingress": true, "storage": true, "portforward": true,
	// Retours de formation (issues #10 et suivantes) : étape d'admission du serveur API.
	"admission": true,
	// Feature 004 : nœuds (drain, placement), métriques et autoscaling, Jobs et CronJobs.
	"node": true, "metrics": true, "batch": true,
}

// Valeurs autorisées du champ fallback.
const (
	FallbackApplication   = "application"    // repli quand l'erreur vient du programme du conteneur
	FallbackManifestIssue = "manifest-issue" // pédagogie d'un type de problème de manifeste
)

// Entry est une entrée du catalogue d'erreurs. Les textes sont en français et peuvent
// utiliser les valeurs capturées par les motifs, par exemple {{.type}}, ainsi que
// {{.kubectl}} (« kubectl » ou « k », selon ce qu'a tapé l'apprenant).
type Entry struct {
	ID          string           `yaml:"id"`
	Category    Category         `yaml:"category"`
	Applies     []string         `yaml:"applies"`
	Patterns    []string         `yaml:"patterns"`
	PodReasons  []string         `yaml:"pod_reasons"`  // états de pod couverts (diagnose)
	LogPatterns []string         `yaml:"log_patterns"` // motifs du journal d'un pod en échec (diagnose, feature 002)
	ExitCodes   []int            `yaml:"exit_codes"`   // codes de sortie de conteneur couverts
	IssueKinds  []string         `yaml:"issue_kinds"`  // types de ManifestIssue couverts (manifest-issue)
	Pitfalls    []string         `yaml:"pitfalls"`     // pièges silencieux couverts (feature 002, diagnose d'un objet)
	Fallback    string           `yaml:"fallback"`
	Concept     ConceptRef       `yaml:"concept"`
	Summary     string           `yaml:"summary"`
	Cause       string           `yaml:"cause"`
	Fixes       []Fix            `yaml:"fixes"`
	AntiPattern *AntiPatternNote `yaml:"anti_pattern"`
	Deepen      Deepening        `yaml:"deepen"`
	DocURL      string           `yaml:"doc_url"`
	Tests       []string         `yaml:"tests"`

	compiled    []*regexp.Regexp
	compiledLog []*regexp.Regexp
}

// Fix est une action corrective. Command est un gabarit facultatif ; Manifest est un extrait
// YAML facultatif ; Safe vaut false pour une solution rapide mais dangereuse, alors
// présentée comme anti-pattern.
type Fix struct {
	Description string `yaml:"description"`
	Command     string `yaml:"command"`
	Manifest    string `yaml:"manifest"`
	Safe        bool   `yaml:"safe"`
}

// ConceptRef est le concept Kubernetes enseigné : terme d'origine et définition en français.
type ConceptRef struct {
	Name       string `yaml:"name"`
	Definition string `yaml:"definition"`
}

// AntiPatternNote met en garde contre une solution rapide mais dangereuse.
type AntiPatternNote struct {
	Description string `yaml:"description"`
}

// Deepening est le contenu d'approfondissement (kubectl telemach deepen).
type Deepening struct {
	Explanation string `yaml:"explanation"`
	Example     string `yaml:"example"`
	Exercise    string `yaml:"exercise"`
}

// Compiled renvoie les motifs compilés de l'entrée.
func (e *Entry) Compiled() []*regexp.Regexp { return e.compiled }

// AppliesTo indique si l'entrée concerne la sous-commande verb (liste vide = toutes).
func (e *Entry) AppliesTo(verb string) bool {
	if len(e.Applies) == 0 {
		return true
	}
	for _, a := range e.Applies {
		if a == verb {
			return true
		}
	}
	return false
}
