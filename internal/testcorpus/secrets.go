package testcorpus

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// minSecretLen écarte les fragments trop courts pour être recherchés sans faux positif.
const minSecretLen = 6

// plainWord reconnaît un mot ordinaire (« endpoint ») : masqué dans son contexte, mais trop
// courant pour être cherché ailleurs.
var plainWord = regexp.MustCompile(`^[a-z]+$`)

// SecretValues renvoie les valeurs sensibles du corpus de masquage : un mot d'un fichier
// testdata/secrets/<cas>.txt absent de <cas>.expected.txt a été masqué, c'est donc un secret,
// sauf s'il reste en clair dans un autre cas (« apiVersion » masqué dans une annotation) ou
// s'il s'agit d'un mot ordinaire. S'y ajoute la valeur écrite en clair dans les manifestes et
// les pods de test (testdata/manifests/plaintext-secret, testdata/pods).
func SecretValues(testdata string) ([]string, error) {
	expected, err := filepath.Glob(filepath.Join(testdata, "secrets", "*.expected.txt"))
	if err != nil {
		return nil, err
	}
	keptAnywhere := map[string]bool{}
	perCase := map[string]map[string]bool{}
	for _, exp := range expected {
		masked, err := os.ReadFile(exp) //nolint:gosec // corpus de test
		if err != nil {
			return nil, err
		}
		kept := map[string]bool{}
		// Le masque « •••••• » peut être collé au mot suivant : on le retire avant découpage.
		for _, w := range words(strings.ReplaceAll(string(masked), "•", " ")) {
			kept[w] = true
			keptAnywhere[w] = true
		}
		perCase[exp] = kept
	}
	set := map[string]bool{"s3cret-fake": true}
	for _, exp := range expected {
		raw, err := os.ReadFile(strings.TrimSuffix(exp, ".expected.txt") + ".txt") //nolint:gosec // corpus de test
		if err != nil {
			return nil, err
		}
		for _, w := range words(string(raw)) {
			if len(w) >= minSecretLen && !perCase[exp][w] && !keptAnywhere[w] && !plainWord.MatchString(w) {
				set[w] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Strings(out)
	return out, nil
}

// words découpe un texte sur les séparateurs d'une ligne de commande, d'un YAML ou d'une URL.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(" \t\r\n=:\"',;()[]{}@", r)
	})
}

// Leaks renvoie les valeurs de secrets présentes en clair dans text.
func Leaks(text string, secrets []string) []string {
	var out []string
	for _, s := range secrets {
		if strings.Contains(text, s) {
			out = append(out, s)
		}
	}
	return out
}
