package manifests

import (
	"fmt"

	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
	"github.com/aboigues/telemach-plugin-kubectl/internal/schema"
)

// SchemaFinding est un problème trouvé par les règles Kubernetes embarquées.
type SchemaFinding struct {
	Kind    string // type de problème (unknown-field, plaintext-secret…)
	Message string
}

// SchemaCheck valide un manifeste du dépôt avec les règles Kubernetes embarquées, pour
// chacune des versions couvertes (contrôle manifest-lint, T058). Les défauts volontaires d'un
// scénario sont filtrés par l'appelant, d'après leur type.
func SchemaCheck(path string) []SchemaFinding {
	var out []SchemaFinding
	for _, v := range schema.Supported {
		r := manifest.Analyze([]string{path}, false, "v"+v+".0")
		for _, is := range r.Issues {
			out = append(out, SchemaFinding{Kind: is.Kind, Message: fmt.Sprintf("règles %s : %s:%d:%d %s %s %s", v, is.File, is.Line, is.Column, is.Kind, is.FieldPath, is.Suggestion)})
		}
	}
	return out
}
