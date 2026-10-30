package manifest

import (
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Erreurs de syntaxe YAML (FR-014) : tabulations, YAML mal formé, deux-points manquant,
// clés en double (souvent un séparateur « --- » oublié). Elles sont bloquantes : le reste
// du document ne peut pas être analysé.

var yamlLine = regexp.MustCompile(`line (\d+): (.+)`)

// SyntaxIssues renvoie les problèmes de syntaxe d'un fichier.
func SyntaxIssues(f *File) []Issue {
	var issues []Issue
	for _, d := range f.Documents {
		if d.SyntaxErr == nil {
			// Le parseur a accepté le document : une tabulation n'y gêne pas (JSON, qui
			// les autorise, ou style flow).
			if d.Root != nil {
				issues = append(issues, duplicateKeys(f, d, d.Root)...)
			}
			continue
		}
		tabs := tabIssues(f, d)
		issues = append(issues, tabs...)
		if len(tabs) == 0 {
			issues = append(issues, parseErrorIssue(f, d))
		}
	}
	return issues
}

// tabIssues signale chaque ligne indentée avec une tabulation d'un document que le parseur a
// refusé.
func tabIssues(f *File, d *Document) []Issue {
	var out []Issue
	for n := d.StartLine; n <= d.EndLine && n <= len(f.Lines); n++ {
		line := f.Lines[n-1]
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if i := strings.IndexByte(indent, '\t'); i >= 0 {
			out = append(out, Issue{
				Kind: KindTabIndent, File: f.Path, Document: d.Index, Line: n, Column: i + 1,
				Resource: d.Resource(), Blocking: true,
			})
		}
	}
	return out
}

// parseErrorIssue rend une erreur du parseur YAML, en ramenant sa ligne au fichier. Pour
// « mapping values are not allowed », la cause est souvent un deux-points oublié à la ligne
// précédente : c'est elle qui est désignée.
func parseErrorIssue(f *File, d *Document) Issue {
	is := Issue{Kind: KindYAMLSyntax, File: f.Path, Document: d.Index, Resource: d.Resource(), Blocking: true}
	msg := d.SyntaxErr.Error()
	is.Detail = strings.TrimPrefix(msg, "yaml: ")
	m := yamlLine.FindStringSubmatch(msg)
	if m == nil {
		is.Line, is.Column = d.StartLine, 1
		return is
	}
	rel, _ := strconv.Atoi(m[1])
	is.Line = d.StartLine + rel - 1
	is.Detail = m[2]
	is.Column = firstColumn(f, is.Line)
	if strings.Contains(m[2], "mapping values are not allowed") {
		if prev := previousContentLine(f, is.Line, d.StartLine); prev > 0 {
			text := strings.TrimSpace(f.Lines[prev-1])
			if !strings.Contains(text, ":") && !strings.HasPrefix(text, "-") && !strings.HasPrefix(text, "#") {
				is.Line, is.Column = prev, firstColumn(f, prev)
				is.Suggestion = ":"
				is.Detail = "deux-points manquant après « " + text + " »"
			}
		}
	}
	return is
}

// duplicateKeys signale une clé répétée dans un même objet. À la racine d'un document, une
// clé « apiVersion » ou « kind » répétée trahit un séparateur « --- » oublié.
func duplicateKeys(f *File, d *Document, n *yaml.Node) []Issue {
	var out []Issue
	switch n.Kind {
	case yaml.MappingNode:
		seen := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if first, dup := seen[k.Value]; dup {
				is := Issue{
					Kind: KindYAMLSyntax, File: f.Path, Document: d.Index, Line: k.Line, Column: k.Column,
					Resource: d.Resource(), FieldPath: k.Value, Blocking: true,
					Detail: "la clé « " + k.Value + " » apparaît deux fois (déjà ligne " + strconv.Itoa(first) + ")",
				}
				if n == d.Root && (k.Value == "apiVersion" || k.Value == "kind") {
					is.Suggestion = "---"
					is.Detail = "deux documents se suivent sans séparateur « --- » : la clé « " + k.Value +
						" » apparaît deux fois (déjà ligne " + strconv.Itoa(first) + ")"
				}
				out = append(out, is)
				continue
			}
			seen[k.Value] = k.Line
			out = append(out, duplicateKeys(f, d, n.Content[i+1])...)
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			out = append(out, duplicateKeys(f, d, c)...)
		}
	}
	return out
}

func firstColumn(f *File, line int) int {
	if line < 1 || line > len(f.Lines) {
		return 0
	}
	l := f.Lines[line-1]
	return len(l) - len(strings.TrimLeft(l, " \t")) + 1
}

func previousContentLine(f *File, line, start int) int {
	for n := line - 1; n >= start; n-- {
		if t := strings.TrimSpace(f.Lines[n-1]); t != "" && !strings.HasPrefix(t, "#") {
			return n
		}
	}
	return 0
}
