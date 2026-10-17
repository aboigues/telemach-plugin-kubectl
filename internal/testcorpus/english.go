package testcorpus

import (
	"regexp"
	"strings"
)

// englishWords sont des mots-outils anglais sans homographe français : leur présence dans un
// texte rédigé par le plugin trahit une phrase non traduite (SC-005). Les termes Kubernetes
// (pod, namespace, get, list…) n'y figurent pas, car ils restent en anglais par principe.
var englishWords = map[string]bool{
	"the": true, "is": true, "are": true, "was": true, "were": true, "not": true,
	"cannot": true, "does": true, "doesn't": true, "don't": true, "can't": true, "have": true,
	"has": true, "with": true, "from": true, "should": true, "must": true, "please": true,
	"your": true, "you": true, "this": true, "that": true, "which": true, "when": true,
	"there": true, "been": true, "will": true, "would": true, "into": true, "only": true,
	"missing": true, "expected": true, "allowed": true, "because": true, "failed": true,
	"found": true, "unknown": true, "invalid": true, "error": true, "and": true, "of": true,
	"to": true, "for": true, "be": true, "it": true, "did": true, "specify": true,
}

var (
	// Zones citées telles quelles : « … », "…" et `…` (valeurs, messages, termes).
	quoted = regexp.MustCompile(`«[^»]*»|"[^"]*"|` + "`[^`]*`")
	ansi   = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)
	// Un mot isolé, majuscule initiale permise ; les noms à tiret, chemins et options
	// (--from-file) ne sont pas de la prose.
	token = regexp.MustCompile(`[^\s,.;:!?()«»]+`)
	prose = regexp.MustCompile(`^[A-Za-z][a-z]*('[a-z]+)?$`)
	// Ligne de commande à copier, éventuellement numérotée ou précédée d'un libellé.
	command = regexp.MustCompile(`^(\d+\.\s+)?([^:]*:\s+)?(\$ )?(kubectl|k|kind|echo|cat|printf|base64)(\s|$)`)
)

// verbatimHeaders ouvrent un bloc recopié de kubectl ou du conteneur, jusqu'à la ligne vide.
// kubeStates sont des phases et raisons Kubernetes qui s'écrivent comme des mots anglais.
var kubeStates = map[string]bool{"Error": true, "Failed": true, "Unknown": true}

var verbatimHeaders = []string{"── Message kubectl d'origine", "📜 Fin du journal"}

// EnglishWords renvoie les mots anglais trouvés dans le texte rédigé d'un rendu, en ignorant
// ce qui est recopié tel quel : message d'origine, journal, commandes, extraits YAML ou JSON,
// titres de concept (termes Kubernetes) et passages cités, même coupés par un retour à la ligne.
func EnglishWords(text string) []string {
	var kept []string
	inVerbatim := false
	for _, line := range strings.Split(ansi.ReplaceAllString(text, ""), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case inVerbatim:
			inVerbatim = trimmed != ""
			continue
		case startsVerbatim(trimmed):
			inVerbatim = true
			continue
		case strings.Contains(line, "│"), command.MatchString(trimmed):
			continue
		}
		if i := strings.Index(trimmed, "Concept — "); i >= 0 {
			trimmed = trimmed[:i]
		}
		kept = append(kept, trimmed)
	}
	var out []string
	for _, w := range token.FindAllString(quoted.ReplaceAllString(strings.Join(kept, "\n"), " "), -1) {
		if prose.MatchString(w) && !kubeStates[w] && englishWords[strings.ToLower(w)] {
			out = append(out, strings.ToLower(w))
		}
	}
	return out
}

func startsVerbatim(line string) bool {
	for _, h := range verbatimHeaders {
		if strings.HasPrefix(line, h) {
			return true
		}
	}
	return false
}
