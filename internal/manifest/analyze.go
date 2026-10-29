package manifest

import (
	"io"
	"sort"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
	"github.com/aboigues/telemach-plugin-kubectl/internal/schema"
)

// Result est le résultat de l'analyse de manifestes.
type Result struct {
	Files         []*File
	Issues        []Issue
	RulesVersion  string // version des règles Kubernetes appliquée (FR-014a)
	RulesFallback bool   // version du client inconnue ou hors plage : règles les plus récentes
	Documents     int
}

// Problems compte les vrais problèmes, informations exclues (issue #12).
func (r Result) Problems() int {
	n := 0
	for _, is := range r.Issues {
		if !is.Info() {
			n++
		}
	}
	return n
}

// Blocking indique si au moins un problème empêche l'analyse du reste.
func (r Result) Blocking() bool {
	for _, is := range r.Issues {
		if is.Blocking {
			return true
		}
	}
	return false
}

// Analyze relit les manifestes désignés (lecture seule), puis applique la syntaxe, les règles
// hors schéma et le schéma de la version choisie d'après le client kubectl. Une erreur de
// syntaxe bloque le reste : seules les erreurs de syntaxe sont alors listées.
func Analyze(paths []string, recursive bool, kubectlVersion string) Result {
	return AnalyzeInput(paths, recursive, kubectlVersion, nil)
}

// AnalyzeInput est Analyze avec l'entrée standard, lue quand paths contient « - »
// (check -f -, feature 004).
func AnalyzeInput(paths []string, recursive bool, kubectlVersion string, stdin io.Reader) Result {
	var r Result
	rules, fallback, err := schema.Select(kubectlVersion)
	if err == nil {
		r.RulesVersion, r.RulesFallback = rules.Version, fallback
	}
	expanded, issues := Expand(paths, recursive)
	for _, p := range expanded {
		var f *File
		var readIssues []Issue
		if p == "-" {
			f, readIssues = ReadStdin(stdin)
		} else {
			f, readIssues = Read(p)
		}
		issues = append(issues, readIssues...)
		if !f.Readable {
			continue
		}
		for _, d := range f.Documents {
			maskDocument(d)
		}
		r.Files = append(r.Files, f)
		// Un template Helm n'est pas encore du YAML : il n'est pas analysé (issue #12).
		if is, ok := helmTemplateIssue(f); ok {
			issues = append(issues, is)
			continue
		}
		r.Documents += len(f.Documents)
		issues = append(issues, SyntaxIssues(f)...)
	}
	syntaxOnly := false
	for _, is := range issues {
		if is.Blocking {
			syntaxOnly = true
		}
	}
	if !syntaxOnly {
		for _, f := range r.Files {
			if isHelmTemplate(f) {
				continue
			}
			if is, ok := helmValuesIssue(f); ok {
				issues = append(issues, is)
				continue
			}
			for _, d := range f.Documents {
				issues = append(issues, SemanticIssues(f, d, rules)...)
			}
		}
		issues = append(issues, hpaMissingRequests(r.Files)...)
	}
	r.Issues = finish(issues, r.Files)
	return r
}

// AddLocations ajoute les positions désignées par un message de kubectl qui ne correspondent
// à aucun problème déjà trouvé (règle propre au serveur, par exemple).
func (r *Result) AddLocations(locs []Location, detail string) {
	for _, l := range locs {
		known := false
		for _, is := range r.Issues {
			if is.File == l.File && is.Covers(l) {
				known = true
				break
			}
		}
		if known {
			continue
		}
		r.Issues = append(r.Issues, Issue{
			Kind: KindServerRejected, File: l.File, Document: l.Document, Line: l.Line, Column: l.Column,
			Resource: l.Resource, FieldPath: l.FieldPath, Detail: detail,
		})
	}
	r.Issues = finish(r.Issues, r.Files)
}

// finish trie les problèmes (bloquants d'abord, puis fichier, document, ligne) et prépare
// leurs extraits masqués et leurs versions corrigées.
func finish(issues []Issue, files []*File) []Issue {
	order := map[string]int{}
	byPath := map[string]*File{}
	for i, f := range files {
		order[f.Path] = i
		byPath[f.Path] = f
	}
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Blocking != b.Blocking {
			return a.Blocking
		}
		if a.Info() != b.Info() {
			return b.Info() // les informations après les problèmes
		}
		if order[a.File] != order[b.File] {
			return order[a.File] < order[b.File]
		}
		if a.Document != b.Document {
			return a.Document < b.Document
		}
		return a.Line < b.Line
	})
	for i := range issues {
		is := &issues[i]
		is.Suggestion = redact.Redact(is.Suggestion)
		is.Detail = redact.Redact(sanitize.Clean(is.Detail))
		if f := byPath[is.File]; f != nil && is.Document > 0 {
			for _, d := range f.Documents {
				if d.Index == is.Document {
					is.Source = redact.Redact(sanitize.Clean(d.Source))
				}
			}
		}
		if f := byPath[is.File]; f != nil && is.Line > 0 {
			is.Excerpt = excerpt(f, is.Line, is.RelatedLines)
			is.Fixed = fixedLines(f, *is)
		}
	}
	return issues
}

// excerpt renvoie les lignes autour de line (et des lignes liées), masquées et nettoyées.
func excerpt(f *File, line int, related []int) []Line {
	lo, hi := line-2, line+2
	for _, r := range related {
		lo, hi = min(lo, r-1), max(hi, r+1)
	}
	if hi-lo > 16 {
		lo, hi = line-2, line+2
	}
	lo, hi = max(lo, 1), min(hi, len(f.Lines))
	masked := maskedColumns(f)
	var out []Line
	for n := lo; n <= hi; n++ {
		out = append(out, Line{Number: n, Text: safeLine(f.Lines[n-1], masked[n])})
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1].Text) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// maskedColumns indexe, par ligne, les colonnes des valeurs masquées.
func maskedColumns(f *File) map[int][]int {
	out := map[int][]int{}
	for _, d := range f.Documents {
		for _, p := range d.Masked {
			out[p.Line] = append(out[p.Line], p.Column)
		}
	}
	return out
}

// safeLine masque une ligne : à partir de chaque colonne de valeur sensible, puis par motifs,
// et neutralise les séquences de contrôle.
func safeLine(text string, cols []int) string {
	for _, c := range cols {
		if r := []rune(text); c-1 < len(r) {
			text = string(r[:c-1]) + redact.Mask
		}
	}
	return redact.Redact(sanitize.Clean(text))
}

// Covers indique si une position désignée par kubectl correspond à ce problème : même
// ligne, ligne liée, ou même champ.
func (is Issue) Covers(l Location) bool {
	if is.Line == l.Line || (l.FieldPath != "" && is.FieldPath == l.FieldPath) {
		return true
	}
	for _, r := range is.RelatedLines {
		if r == l.Line {
			return true
		}
	}
	for _, p := range is.RelatedPaths {
		if p == l.FieldPath {
			return true
		}
	}
	return false
}
