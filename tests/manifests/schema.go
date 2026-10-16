package manifests

import (
	"fmt"

	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
	"github.com/aboigues/telemach-plugin-kubectl/internal/schema"
)

// SchemaCheck valide un manifeste du dépôt avec les règles Kubernetes embarquées, pour
// chacune des versions couvertes (contrôle manifest-lint, T058). Elle renvoie un message par
// problème trouvé ; les défauts volontaires d'un scénario sont filtrés par l'appelant.
func SchemaCheck(path string) []string {
	var out []string
	for _, v := range schema.Supported {
		r := manifest.Analyze([]string{path}, false, "v"+v+".0")
		for _, is := range r.Issues {
			out = append(out, fmt.Sprintf("règles %s : %s:%d:%d %s %s %s", v, is.File, is.Line, is.Column, is.Kind, is.FieldPath, is.Suggestion))
		}
	}
	return out
}
