package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
)

// ManifestOptions règle le rendu des problèmes de manifestes.
type ManifestOptions struct {
	Options
	Kubectl     string // « kubectl » ou « k »
	ShowRules   bool   // première ligne « Règles Kubernetes 1.37 » (check)
	ShowSuccess bool   // message explicite quand aucun problème n'est trouvé (check)
}

// RenderManifestIssues affiche les problèmes localisés (FR-013 à FR-016) : position, champ,
// explication courte, extrait coloré, version corrigée, puis les concepts en jeu.
func RenderManifestIssues(w io.Writer, r manifest.Result, cat *catalog.Catalog, o ManifestOptions) error {
	t := o.Theme
	width := max(o.Width, 40)
	kubectl := o.Kubectl
	if kubectl == "" {
		kubectl = "kubectl"
	}
	var b strings.Builder
	if o.ShowRules && r.RulesVersion != "" {
		line := LabelRules + " " + r.RulesVersion
		if r.RulesFallback {
			line += " (version de kubectl inconnue ou hors des versions " + "1.35 à 1.37 : règles les plus récentes)"
		}
		b.WriteString(t.Dim(line) + "\n")
	}
	problems := r.Problems()
	if problems == 0 && o.ShowSuccess {
		b.WriteString(t.Success(SymbolSuccess+" Aucun problème détecté dans "+fileList(r)+" ("+plural(r.Documents, "document")+")") + "\n")
	}
	if len(r.Issues) == 0 {
		_, err := io.WriteString(w, b.String())
		return err
	}
	var concepts []*catalog.Entry
	seen := map[string]bool{}
	for i, is := range r.Issues {
		if is.Info() {
			// Information (issue #12) : ni numéro de problème, ni extrait.
			if i > 0 || problems == 0 {
				b.WriteString("\n")
			}
			b.WriteString(t.Hint(SymbolInfo+" "+LabelInfo+" — "+position(is)) + "\n")
			if entry := entryFor(cat, is.Kind); entry != nil {
				captures := issueCaptures(is, kubectl)
				b.WriteString(wrap(catalog.Render(entry.Summary, captures), width, indent) + "\n")
				for _, f := range entry.Fixes {
					if f.Safe && f.Command != "" {
						b.WriteString(indent + t.Highlight(catalog.Render(f.Command, captures), "bash") + "\n")
						break
					}
				}
				if !seen[entry.ID] {
					seen[entry.ID] = true
					concepts = append(concepts, entry)
				}
			}
			continue
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(t.Title(fmt.Sprintf("%s %d/%d — %s", LabelProblem, i+1, problems, position(is))) + "\n")
		if is.FieldPath != "" && is.Kind != manifest.KindYAMLSyntax {
			b.WriteString(indent + t.Dim("champ « ") + t.Emphasis(is.FieldPath) + t.Dim(" »") + "\n")
		}
		captures := issueCaptures(is, kubectl)
		var entry *catalog.Entry
		if cat != nil {
			entry, _ = cat.ByIssueKind(is.Kind)
		}
		if entry != nil {
			b.WriteString(wrap(catalog.Render(entry.Summary, captures), width, indent) + "\n")
			if !seen[entry.ID] {
				seen[entry.ID] = true
				concepts = append(concepts, entry)
			}
		} else if is.Detail != "" {
			b.WriteString(wrap(is.Detail, width, indent) + "\n")
		}
		if len(is.Excerpt) > 0 {
			lines := make([]Line, len(is.Excerpt))
			for j, l := range is.Excerpt {
				lines[j] = Line{Number: l.Number, Text: l.Text}
			}
			b.WriteString("\n" + indentBlock(t.HighlightLines(lines, "yaml", is.Line, is.Column, is.RelatedLines...), indent) + "\n")
		}
		if len(is.Fixed) > 0 {
			b.WriteString("\n" + indent + t.Fix(LabelFixedPart) + "\n")
			b.WriteString(indentBlock(fixedBlock(t, is.Fixed), indent) + "\n")
		}
	}
	if len(concepts) > 0 {
		b.WriteString("\n")
		for i, e := range concepts {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(t.Concept(LabelConcept+" — "+e.Concept.Name) + "\n")
			b.WriteString(wrap(e.Concept.Definition, width, indent) + "\n")
		}
		b.WriteString("\n" + t.Hint(LabelDeepen+" : "+kubectl+" telemach deepen") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func entryFor(cat *catalog.Catalog, kind string) *catalog.Entry {
	if cat == nil {
		return nil
	}
	e, _ := cat.ByIssueKind(kind)
	return e
}

// issueCaptures fournit aux gabarits du catalogue les éléments du problème.
func issueCaptures(is manifest.Issue, kubectl string) map[string]string {
	kind, _, _ := strings.Cut(is.Resource, "/")
	c := map[string]string{
		"kubectl": kubectl, "field": is.FieldPath, "suggestion": is.Suggestion, "detail": is.Detail,
		"resource": is.Resource, "file": is.File, "kind": kind,
	}
	if is.Line > 0 {
		c["line"] = fmt.Sprint(is.Line)
	}
	if is.FieldPath != "" {
		c["explain_path"] = diagnose.ExplainPath(kind, is.FieldPath)
	}
	return c
}

// position écrit « fichier:ligne:colonne (document d, Kind/nom, template t) ».
func position(is manifest.Issue) string {
	pos := is.File
	if is.Line > 0 {
		pos += fmt.Sprintf(":%d", is.Line)
		if is.Column > 0 {
			pos += fmt.Sprintf(":%d", is.Column)
		}
	}
	var ctx []string
	if is.Document > 0 {
		ctx = append(ctx, fmt.Sprintf("document %d", is.Document))
	}
	if is.Resource != "" && !strings.HasSuffix(is.Resource, "/") {
		ctx = append(ctx, is.Resource)
	}
	if is.Source != "" {
		ctx = append(ctx, "template "+is.Source)
	}
	if len(ctx) > 0 {
		pos += " (" + strings.Join(ctx, ", ") + ")"
	}
	return pos
}

// fixedBlock affiche la version corrigée ; une ligne ajoutée est marquée « + ».
func fixedBlock(t Theme, lines []manifest.Line) string {
	width := 1
	for _, l := range lines {
		width = max(width, len(fmt.Sprint(l.Number)))
	}
	var b strings.Builder
	for _, l := range lines {
		num := "+"
		if l.Number > 0 {
			num = fmt.Sprint(l.Number)
		}
		gutter := fmt.Sprintf("  %*s │ ", width, num)
		b.WriteString(t.Fix(gutter) + t.Highlight(strings.ReplaceAll(l.Text, "\t", TabMarker), "yaml") + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func indentBlock(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func fileList(r manifest.Result) string {
	var names []string
	for _, f := range r.Files {
		names = append(names, f.Path)
	}
	if len(names) > 3 {
		return fmt.Sprintf("%s et %d autres fichiers", strings.Join(names[:2], ", "), len(names)-2)
	}
	return strings.Join(names, ", ")
}

func plural(n int, word string) string {
	if n > 1 {
		return fmt.Sprintf("%d %ss", n, word)
	}
	return fmt.Sprintf("%d %s", n, word)
}
