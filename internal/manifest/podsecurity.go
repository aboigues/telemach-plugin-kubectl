package manifest

import (
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Version corrigée après un refus PodSecurity (feature 004, FR-003, research.md F3). Ces
// règles ne sont appliquées que par explain, d'après la liste des violations renvoyée par le
// serveur : check ne connaît pas le niveau imposé au namespace et ne les applique jamais.

// securityFix est un champ du securityContext d'un conteneur, cité par le serveur.
type securityFix struct {
	keyword  string // mot du message du serveur qui désigne le champ
	field    string // clé dans securityContext
	flow     string // valeur en notation YAML en ligne
	block    []string
	podLevel bool // le champ peut aussi être posé au niveau du pod
	present  func(sc *yaml.Node) bool
}

var securityFixes = []securityFix{
	{
		keyword: "allowPrivilegeEscalation", field: "allowPrivilegeEscalation", flow: "false",
		present: func(sc *yaml.Node) bool { return scalar(sc, "allowPrivilegeEscalation") == "false" },
	},
	{
		keyword: "capabilities", field: "capabilities", flow: `{drop: ["ALL"]}`, block: []string{`drop: ["ALL"]`},
		present: func(sc *yaml.Node) bool {
			for _, c := range seq(get(get(sc, "capabilities"), "drop")) {
				if c.Value == "ALL" {
					return true
				}
			}
			return false
		},
	},
	{
		keyword: "runAsNonRoot", field: "runAsNonRoot", flow: "true", podLevel: true,
		present: func(sc *yaml.Node) bool { return scalar(sc, "runAsNonRoot") == "true" },
	},
	{
		keyword: "seccompProfile", field: "seccompProfile", flow: "{type: RuntimeDefault}", block: []string{"type: RuntimeDefault"}, podLevel: true,
		present: func(sc *yaml.Node) bool {
			t := scalar(get(sc, "seccompProfile"), "type")
			return t == "RuntimeDefault" || t == "Localhost"
		},
	},
}

// citedContainers retrouve, dans une violation, les conteneurs nommés :
// « container "web" », « containers "a", "b" », « pod or container "web" ».
var citedContainers = regexp.MustCompile(`containers? ((?:"[^"]+"(?:, )?)+)`)

// podTemplates associe chaque type de charge de travail au chemin de son modèle de pod.
var podTemplates = map[string][]string{
	"Pod": {}, "Deployment": {"spec", "template"}, "ReplicaSet": {"spec", "template"},
	"StatefulSet": {"spec", "template"}, "DaemonSet": {"spec", "template"}, "Job": {"spec", "template"},
	"CronJob": {"spec", "jobTemplate", "spec", "template"},
}

// AddPodSecurity ajoute un problème pod-security par conteneur fautif des manifestes relus,
// pour les champs cités dans violations (texte du serveur après « violates PodSecurity … : »).
func (r *Result) AddPodSecurity(violations string) {
	cited := map[string]bool{}
	for _, m := range citedContainers.FindAllStringSubmatch(violations, -1) {
		for _, name := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
			cited[name[1]] = true
		}
	}
	var wanted []securityFix
	for _, f := range securityFixes {
		if strings.Contains(violations, f.keyword) {
			wanted = append(wanted, f)
		}
	}
	if len(wanted) == 0 {
		return
	}
	for _, f := range r.Files {
		for _, d := range f.Documents {
			path, ok := podTemplates[d.Kind]
			if !ok || d.Root == nil {
				continue
			}
			tpl := d.Root
			prefix := ""
			for _, k := range path {
				tpl = get(tpl, k)
				prefix += k + "."
			}
			spec := get(tpl, "spec")
			podSC := get(spec, "securityContext")
			for i, c := range seq(get(spec, "containers")) {
				name := scalar(c, "name")
				if len(cited) > 0 && !cited[name] {
					continue
				}
				sc := get(c, "securityContext")
				var missing []securityFix
				for _, fx := range wanted {
					if fx.present(sc) || (fx.podLevel && fx.present(podSC)) {
						continue
					}
					missing = append(missing, fx)
				}
				if len(missing) == 0 {
					continue
				}
				r.Issues = append(r.Issues, podSecurityIssue(f, d, c, i, prefix, missing))
			}
		}
	}
	r.Issues = finish(r.Issues, r.Files)
}

// podSecurityIssue désigne le securityContext du conteneur (ou, s'il n'en a pas, son nom) et
// prépare la version corrigée : la ligne réécrite pour un securityContext en ligne ({…}), ou
// les lignes à ajouter sous la clé.
func podSecurityIssue(f *File, d *Document, c *yaml.Node, index int, prefix string, missing []securityFix) Issue {
	name := scalar(c, "name")
	is := Issue{
		Kind: KindPodSecurity, File: f.Path, Document: d.Index, Resource: d.Resource(),
		FieldPath: prefix + "spec.containers[" + itoa(index) + "].securityContext",
		Detail:    "conteneur « " + name + " »",
	}
	key := getKey(c, "securityContext")
	sc := get(c, "securityContext")
	switch {
	case key != nil && sc != nil && sc.Style&yaml.FlowStyle != 0 && key.Line == sc.Line:
		is.Line, is.Column = key.Line, key.Column
		var pairs []string
		for i := 0; i+1 < len(sc.Content); i += 2 {
			pairs = append(pairs, sc.Content[i].Value+": "+flowValue(sc.Content[i+1]))
		}
		for _, m := range missing {
			pairs = append(pairs, m.field+": "+m.flow)
		}
		text := f.Lines[key.Line-1]
		is.Suggestion = text[:key.Column-1] + "securityContext: {" + strings.Join(pairs, ", ") + "}"
	case key != nil:
		is.Line, is.Column = key.Line, key.Column
		indent := strings.Repeat(" ", key.Column-1+2)
		if sc != nil && len(sc.Content) > 0 {
			indent = strings.Repeat(" ", sc.Content[0].Column-1)
		}
		is.Insert = blockLines(indent, missing)
	default:
		nameKey := getKey(c, "name")
		is.Line, is.Column = nameKey.Line, nameKey.Column
		fieldIndent := strings.Repeat(" ", nameKey.Column-1)
		is.Insert = append([]string{fieldIndent + "securityContext:"}, blockLines(fieldIndent+"  ", missing)...)
	}
	return is
}

// blockLines écrit les champs manquants en notation par blocs.
func blockLines(indent string, missing []securityFix) []string {
	var out []string
	for _, m := range missing {
		if len(m.block) == 0 {
			out = append(out, indent+m.field+": "+m.flow)
			continue
		}
		out = append(out, indent+m.field+":")
		for _, b := range m.block {
			out = append(out, indent+"  "+b)
		}
	}
	return out
}

// flowValue réécrit une valeur scalaire ou une petite collection en notation en ligne.
func flowValue(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		var pairs []string
		for i := 0; i+1 < len(n.Content); i += 2 {
			pairs = append(pairs, n.Content[i].Value+": "+flowValue(n.Content[i+1]))
		}
		return "{" + strings.Join(pairs, ", ") + "}"
	case yaml.SequenceNode:
		var items []string
		for _, c := range n.Content {
			items = append(items, flowValue(c))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	if n.Style == yaml.DoubleQuotedStyle {
		return `"` + n.Value + `"`
	}
	return n.Value
}

// seq renvoie les éléments d'une séquence YAML.
func seq(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}
