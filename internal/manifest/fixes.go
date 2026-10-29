package manifest

import (
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

// fixedLines propose la version corrigée de la ligne fautive (FR-016), quand une correction
// sûre existe. Une ligne ajoutée a le numéro 0.
func fixedLines(f *File, is Issue) []Line {
	if is.Line < 1 || is.Line > len(f.Lines) {
		return nil
	}
	text := f.Lines[is.Line-1]
	indent := text[:len(text)-len(strings.TrimLeft(text, " \t"))]
	col := is.Column - 1
	if col < 0 || col > len(text) {
		col = len(indent)
	}
	one := func(t string) []Line { return []Line{{Number: is.Line, Text: redact.Redact(t)}} }
	switch is.Kind {
	case KindPodSecurity:
		if is.Suggestion != "" {
			return one(is.Suggestion)
		}
		out := []Line{{Number: is.Line, Text: redact.Redact(text)}}
		for _, l := range is.Insert {
			out = append(out, Line{Number: 0, Text: l})
		}
		return out
	case KindTabIndent:
		return one(strings.Repeat(" ", guessIndent(f, is.Line)) + strings.TrimLeft(text, " \t"))
	case KindYAMLSyntax:
		switch is.Suggestion {
		case ":":
			return one(strings.TrimRight(text, " ") + ":")
		case "---":
			return []Line{{Number: 0, Text: "---"}, {Number: is.Line, Text: text}}
		}
	case KindUnknownField:
		if is.Suggestion != "" {
			return one(replaceAt(text, col, keyAt(text, col), is.Suggestion))
		}
	case KindWrongLevel:
		if parent, field, ok := strings.Cut(is.Suggestion, "[]."); ok {
			value := strings.TrimSpace(text[min(len(text), col+len(field)+1):])
			return []Line{
				{Number: 0, Text: indent + parent + ":"},
				{Number: is.Line, Text: indent + "- " + field + ": " + value},
			}
		}
	case KindWrongType:
		if is.Suggestion != "" && !strings.ContainsAny(is.Suggestion, "\n:") {
			return one(text[:col] + is.Suggestion)
		}
	case KindInvalidEnum, KindRemovedAPIVersion, KindUnknownKind, KindInvalidName, KindInvalidCron:
		if is.Suggestion != "" {
			return one(text[:col] + is.Suggestion)
		}
	case KindMissingAPIVersion:
		if is.Suggestion != "" {
			return []Line{{Number: 0, Text: "apiVersion: " + is.Suggestion}, {Number: is.Line, Text: text}}
		}
	case KindSelectorMismatch:
		if len(is.RelatedLines) == 1 && !strings.Contains(is.Suggestion, ",") {
			rl := is.RelatedLines[0]
			if rl >= 1 && rl <= len(f.Lines) {
				t := f.Lines[rl-1]
				ind := t[:len(t)-len(strings.TrimLeft(t, " "))]
				return []Line{{Number: rl, Text: ind + is.Suggestion}}
			}
		}
	case KindPlaintextSecret:
		if strings.Contains(is.FieldPath, ".env[") {
			return []Line{
				{Number: is.Line, Text: text[:col-len("value: ")] + "valueFrom:"},
				{Number: 0, Text: indent + "  secretKeyRef:"},
				{Number: 0, Text: indent + "    name: NOM-DU-SECRET"},
				{Number: 0, Text: indent + "    key: NOM-DE-LA-CLE"},
			}
		}
	}
	return nil
}

// keyAt renvoie la clé YAML (ou JSON, entre guillemets) qui commence à la colonne col.
func keyAt(text string, col int) string {
	rest := text[col:]
	if strings.HasPrefix(rest, `"`) {
		if end := strings.Index(rest[1:], `"`); end >= 0 {
			return rest[1 : end+1]
		}
	}
	if i := strings.Index(rest, ":"); i >= 0 {
		return rest[:i]
	}
	return rest
}

// replaceAt remplace old par repl à partir de la colonne col (guillemets JSON conservés).
func replaceAt(text string, col int, old, repl string) string {
	if strings.HasPrefix(text[col:], `"`+old+`"`) {
		return text[:col+1] + repl + text[col+1+len(old):]
	}
	if strings.HasPrefix(text[col:], old) {
		return text[:col] + repl + text[col+len(old):]
	}
	return text
}

// guessIndent propose l'indentation en espaces d'une ligne indentée par tabulation, d'après
// ses voisines : un élément de liste se place deux colonnes avant les champs qui le suivent ;
// sinon, la ligne s'aligne sous la clé parente (+2) ou sur la ligne précédente.
func guessIndent(f *File, line int) int {
	width := func(n int) (int, bool) {
		t := f.Lines[n-1]
		trimmed := strings.TrimLeft(t, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "\t") {
			return 0, false
		}
		return len(t) - len(trimmed), true
	}
	current := strings.TrimLeft(f.Lines[line-1], " \t")
	if strings.HasPrefix(current, "- ") {
		for n := line + 1; n <= len(f.Lines); n++ {
			if w, ok := width(n); ok {
				if w >= 2 && !strings.HasPrefix(strings.TrimLeft(f.Lines[n-1], " "), "- ") {
					return w - 2
				}
				break
			}
		}
	}
	for n := line - 1; n >= 1; n-- {
		if w, ok := width(n); ok {
			if strings.HasSuffix(strings.TrimSpace(f.Lines[n-1]), ":") {
				return w + 2
			}
			return w
		}
	}
	return 0
}
