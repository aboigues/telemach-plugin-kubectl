package manifest

import "testing"

// LL-020 : le JSON autorise les tabulations ; un manifeste JSON valide indenté par
// tabulations (comme l'écrivent beaucoup d'éditeurs) n'est pas une erreur de syntaxe YAML.
// En YAML, une indentation par tabulation reste signalée.
func TestTabIndent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		tabs    int
	}{
		{"JSON indenté par tabulations", "{\n\t\"apiVersion\": \"v1\",\n\t\"kind\": \"ConfigMap\",\n\t\"metadata\": {\n\t\t\"name\": \"web\"\n\t},\n\t\"data\": {\n\t\t\"a\": \"b\"\n\t}\n}\n", 0},
		{"YAML indenté par tabulation", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n\tname: web\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := fromStdin([]byte(c.content))
			tabs := 0
			for _, is := range got.Issues {
				if is.Kind == KindTabIndent {
					tabs++
				} else if is.Blocking {
					t.Errorf("problème inattendu : %+v", is)
				}
			}
			if tabs != c.tabs {
				t.Errorf("%d tabulation(s) signalée(s), %d attendue(s) : %+v", tabs, c.tabs, got.Issues)
			}
		})
	}
}
