package testcorpus

import (
	"slices"
	"testing"
)

func TestEnglishWords(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"phrase non traduite", "✖ The pod is not ready.", []string{"the", "is", "not"}},
		{"prose française", "✖ Le pod « web » n'existe pas dans le namespace demo.", nil},
		{"message d'origine recopié", "── Message kubectl d'origine ──\nError: the server doesn't have a resource type\n\n➜ suite", nil},
		{"fin de bloc recopié", "── Message kubectl d'origine ──\nerror\n\nthe end", []string{"the"}},
		{"journal recopié", "📜 Fin du journal (conteneur app)\n   failed to start\n", nil},
		{"citation coupée", "Message : « failed to create\n   task: failed » puis rien.", nil},
		{"commande à copier", "   1. Lister :  kubectl get pods --from-file=x\n", nil},
		{"extrait YAML", "   ➤14 │ the: value", nil},
		{"titre de concept", "📘 Concept — Secret and ConfigMap references", nil},
		{"noms et états", "pod missing-secret · état Error · phase Failed", nil},
		{"couleurs ANSI", "\x1b[1mthe\x1b[0m", []string{"the"}},
	}
	for _, tt := range tests {
		if got := EnglishWords(tt.text); !slices.Equal(got, tt.want) {
			t.Errorf("%s : %q, attendu %q", tt.name, got, tt.want)
		}
	}
}

func TestSecretValues(t *testing.T) {
	got, err := SecretValues("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fake-token-123", "s3cret-fake", "ZmFrZQ"} {
		if !slices.Contains(got, want) {
			t.Errorf("secret %q absent de %q", want, got)
		}
	}
	// Mots masqués dans leur contexte mais courants : jamais recherchés.
	for _, word := range []string{"apiVersion", "Secret", "endpoint", "demo"} {
		if slices.Contains(got, word) {
			t.Errorf("%q n'est pas une valeur sensible", word)
		}
	}
	// Un dossier sans corpus ne donne que la valeur des manifestes et pods de test.
	if v, err := SecretValues(t.TempDir()); err != nil || !slices.Equal(v, []string{"s3cret-fake"}) {
		t.Errorf("corpus vide : %q, %v", v, err)
	}
}

func TestLeaks(t *testing.T) {
	if got := Leaks("token=fake-token-123 ok", []string{"fake-token-123", "s3cret-fake"}); !slices.Equal(got, []string{"fake-token-123"}) {
		t.Errorf("Leaks : %q", got)
	}
}
