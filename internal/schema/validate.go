package schema

import (
	"sort"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/textdist"
)

// Types de problèmes détectés par le schéma (sous-ensemble de ManifestIssue.Kind).
const (
	UnknownField    = "unknown-field"
	WrongLevel      = "wrong-level"
	WrongType       = "wrong-type"
	RequiredMissing = "required-missing"
	InvalidEnum     = "invalid-enum"
)

// Finding est un écart entre un manifeste et les règles de Kubernetes. Node est le nœud à
// montrer : la clé d'un champ inconnu, la valeur d'un type incorrect, la clé du parent d'un
// champ obligatoire manquant.
type Finding struct {
	Kind       string
	Path       string // chemin du champ : spec.template.spec.containers[0].image
	Node       *yaml.Node
	Suggestion string // champ proche, niveau correct ou valeur attendue
	Expected   string // type attendu, en français
}

// Validate parcourt le document et le schéma de sa ressource en parallèle. root est le
// mapping racine du document. Une ressource non couverte n'est pas validée (ok = false).
func (s *SchemaSet) Validate(root *yaml.Node, apiVersion, kind string) (findings []Finding, ok bool) {
	sch, found := s.Lookup(apiVersion, kind)
	if !found {
		return nil, false
	}
	v := validator{set: s}
	v.walk(root, sch, "", nil, nil)
	return v.findings, true
}

type validator struct {
	set      *SchemaSet
	findings []Finding
}

func (v *validator) add(f Finding) { v.findings = append(v.findings, f) }

// walk valide node avec sch. keyNode est la clé qui porte node (nil à la racine) ; parent
// est le schéma de l'objet qui contient node (pour reconnaître un champ placé un niveau trop
// bas).
func (v *validator) walk(node *yaml.Node, sch *Schema, path string, keyNode *yaml.Node, parent *Schema) {
	sch = v.set.resolve(sch)
	if sch == nil || node == nil || isNull(node) || sch.PreserveUnknown {
		return
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	if sch.IntOrString || sch.Format == "int-or-string" {
		if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!str") {
			v.add(Finding{Kind: WrongType, Path: path, Node: node, Expected: "un nombre entier ou un texte"})
		}
		return
	}
	switch {
	case sch.Type == "object" || len(sch.Properties) > 0:
		v.object(node, sch, path, keyNode, parent)
	case sch.Type == "array":
		if node.Kind != yaml.SequenceNode {
			v.add(Finding{Kind: WrongType, Path: path, Node: node, Expected: "une liste (éléments précédés de « - »)"})
			return
		}
		for i, item := range node.Content {
			v.walk(item, sch.Items, path+"["+strconv.Itoa(i)+"]", nil, nil)
		}
	case sch.Type == "string":
		v.scalar(node, path, sch, "un texte", "!!str", "!!binary", "!!timestamp")
	case sch.Type == "integer":
		v.scalar(node, path, sch, "un nombre entier (sans guillemets)", "!!int")
	case sch.Type == "number":
		v.scalar(node, path, sch, "un nombre (sans guillemets)", "!!int", "!!float")
	case sch.Type == "boolean":
		v.scalar(node, path, sch, "true ou false (sans guillemets)", "!!bool")
	}
}

// scalar vérifie la forme YAML d'une valeur simple et, le cas échéant, sa liste de valeurs.
func (v *validator) scalar(node *yaml.Node, path string, sch *Schema, expected string, tags ...string) {
	if node.Kind != yaml.ScalarNode {
		v.add(Finding{Kind: WrongType, Path: path, Node: node, Expected: expected})
		return
	}
	okTag := false
	for _, t := range tags {
		if node.Tag == t {
			okTag = true
		}
	}
	if !okTag {
		f := Finding{Kind: WrongType, Path: path, Node: node, Expected: expected}
		if node.Tag == "!!str" && (tags[0] == "!!int" || tags[0] == "!!bool") {
			f.Suggestion = node.Value // retirer les guillemets
		}
		v.add(f)
		return
	}
	if len(sch.Enum) > 0 {
		var allowed []string
		for _, e := range sch.Enum {
			if s, ok := e.(string); ok {
				allowed = append(allowed, s)
				if s == node.Value {
					return
				}
			}
		}
		if len(allowed) > 0 {
			sugg, _ := textdist.Closest(node.Value, allowed, 3)
			v.add(Finding{Kind: InvalidEnum, Path: path, Node: node, Suggestion: sugg})
		}
	}
}

// object vérifie un mapping : champs connus, champs inconnus (avec suggestion), champs
// obligatoires.
func (v *validator) object(node *yaml.Node, sch *Schema, path string, keyNode *yaml.Node, parent *Schema) {
	if node.Kind != yaml.MappingNode {
		v.add(Finding{Kind: WrongType, Path: path, Node: node, Expected: "un objet (des paires « clé: valeur »)"})
		return
	}
	present := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, val := node.Content[i], node.Content[i+1]
		present[k.Value] = true
		child := join(path, k.Value)
		if prop, ok := sch.Properties[k.Value]; ok {
			v.walk(val, prop, child, k, sch)
			continue
		}
		if sch.AdditionalProperties != nil {
			v.walk(val, sch.AdditionalProperties, child, k, sch)
			continue
		}
		if len(sch.Properties) == 0 {
			continue // objet libre
		}
		v.add(v.unknown(k, child, sch, parent))
	}
	for _, req := range sch.Required {
		if present[req] {
			continue
		}
		at := keyNode
		if at == nil {
			at = node
		}
		v.add(Finding{Kind: RequiredMissing, Path: join(path, req), Node: at, Suggestion: req})
	}
}

// unknown qualifie un champ inconnu : faute de frappe (champ proche au même niveau), ou
// champ placé au mauvais niveau (il existe un niveau en dessous ou au-dessus).
func (v *validator) unknown(k *yaml.Node, path string, sch, parent *Schema) Finding {
	names := make([]string, 0, len(sch.Properties))
	for name := range sch.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	maxDist := 2
	if len(k.Value) > 8 {
		maxDist = len(k.Value) / 4
	}
	best, bestDist := "", maxDist+1
	for _, name := range names {
		if d := textdist.TypoDistance(k.Value, name); d < bestDist {
			best, bestDist = name, d
		}
	}
	if best != "" {
		return Finding{Kind: UnknownField, Path: path, Node: k, Suggestion: best}
	}
	// Un niveau en dessous : « containerPort » directement sous un conteneur, au lieu de
	// « ports[].containerPort ».
	for _, name := range names {
		child := v.set.resolve(sch.Properties[name])
		suffix := "."
		if child != nil && child.Type == "array" {
			child = v.set.resolve(child.Items)
			suffix = "[]."
		}
		if child != nil {
			if _, ok := child.Properties[k.Value]; ok {
				return Finding{Kind: WrongLevel, Path: path, Node: k, Suggestion: name + suffix + k.Value}
			}
		}
	}
	// Un niveau au-dessus.
	if p := v.set.resolve(parent); p != nil {
		if _, ok := p.Properties[k.Value]; ok {
			return Finding{Kind: WrongLevel, Path: path, Node: k, Suggestion: "../" + k.Value}
		}
	}
	return Finding{Kind: UnknownField, Path: path, Node: k}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}
