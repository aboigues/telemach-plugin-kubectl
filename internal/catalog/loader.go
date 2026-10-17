package catalog

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

//go:embed data
var embedded embed.FS

// Catalog est l'ensemble des entrées, dans l'ordre des fichiers puis de déclaration.
type Catalog struct {
	Entries []*Entry
	byID    map[string]*Entry
}

var idPattern = regexp.MustCompile(`^[a-z]+(\.[a-z0-9-]+)+$`)

// Mots-outils dont l'un doit figurer dans les textes rédigés en français (SC-005).
var frenchWords = []string{
	"le", "la", "les", "de", "du", "des", "un", "une", "est", "pas", "et", "en", "au", "aux",
	"ta", "ton", "tes", "ce", "cette", "il", "pour", "dans", "sur", "avec", "par", "aucun",
	"l'", "d'", "n'", "qu'", "s'", "c'",
}

// Default charge le catalogue embarqué dans le binaire.
func Default() (*Catalog, error) {
	sub, err := fs.Sub(embedded, "data")
	if err != nil {
		return nil, err
	}
	return Load(sub)
}

// Load charge et valide tous les fichiers *.yaml de fsys.
func Load(fsys fs.FS) (*Catalog, error) {
	files, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	c := &Catalog{byID: map[string]*Entry{}}
	var errs []error
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		var entries []*Entry
		if err := dec.Decode(&entries); err != nil {
			errs = append(errs, fmt.Errorf("%s : %w", f, err))
			continue
		}
		for _, e := range entries {
			for _, verr := range validate(e) {
				errs = append(errs, fmt.Errorf("%s : %s : %w", f, e.ID, verr))
			}
			if _, dup := c.byID[e.ID]; dup {
				errs = append(errs, fmt.Errorf("%s : identifiant en double : %s", f, e.ID))
			}
			c.byID[e.ID] = e
			c.Entries = append(c.Entries, e)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

// Get renvoie l'entrée d'identifiant id.
func (c *Catalog) Get(id string) (*Entry, bool) {
	e, ok := c.byID[id]
	return e, ok
}

// validate applique les règles du modèle de données (data-model.md) et compile les motifs.
func validate(e *Entry) []error {
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !idPattern.MatchString(e.ID) {
		fail("identifiant invalide %q (format attendu : ^[a-z]+(\\.[a-z0-9-]+)+$)", e.ID)
	}
	if !Categories[e.Category] {
		fail("catégorie inconnue %q", e.Category)
	}
	if len(e.Patterns) == 0 && len(e.ExitCodes) == 0 && len(e.PodReasons) == 0 && e.Fallback == "" {
		fail("au moins un motif est requis (ou pod_reasons, exit_codes, ou fallback)")
	}
	if e.Fallback != "" && e.Fallback != FallbackApplication && e.Fallback != FallbackManifestIssue {
		fail("fallback inconnu %q (valeurs possibles : application, manifest-issue)", e.Fallback)
	}
	if (e.Fallback == FallbackManifestIssue) != (len(e.IssueKinds) > 0) {
		fail("fallback: manifest-issue va de pair avec issue_kinds")
	}
	e.compiled = nil
	for _, p := range e.Patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			fail("motif invalide %q : %v", p, err)
			continue
		}
		e.compiled = append(e.compiled, re)
	}
	if e.Concept.Name == "" || e.Concept.Definition == "" {
		fail("concept incomplet (nom et définition obligatoires)")
	}
	if e.Summary == "" {
		fail("résumé obligatoire")
	} else if n := utf8.RuneCountInString(e.Summary); n > 160 {
		fail("résumé trop long : %d caractères (160 maximum)", n)
	}
	if e.Cause == "" {
		fail("cause obligatoire")
	}
	if len(e.Fixes) == 0 {
		fail("au moins une correction est requise")
	}
	for i, f := range e.Fixes {
		if f.Description == "" {
			fail("correction %d sans description", i+1)
		}
		if !f.Safe && e.AntiPattern == nil {
			fail("correction %d non sûre sans anti_pattern explicatif", i+1)
		}
	}
	if e.Deepen.Explanation == "" || e.Deepen.Example == "" || e.Deepen.Exercise == "" {
		fail("approfondissement incomplet (explication, exemple et exercice obligatoires)")
	}
	if !strings.HasPrefix(e.DocURL, "https://kubernetes.io/docs/") {
		fail("doc_url doit pointer vers https://kubernetes.io/docs/")
	}
	if len(e.Tests) == 0 {
		fail("au moins un cas de test du corpus est requis")
	}
	for _, field := range []struct{ name, text string }{
		{"summary", e.Summary}, {"cause", e.Cause}, {"concept.definition", e.Concept.Definition},
	} {
		if field.text != "" && !looksFrench(field.text) {
			fail("%s ne semble pas rédigé en français : %q", field.name, field.text)
		}
	}
	texts := append([]string{e.Summary, e.Cause, e.Deepen.Explanation, e.Deepen.Example, e.Deepen.Exercise}, fixTexts(e)...)
	if e.AntiPattern != nil {
		texts = append(texts, e.AntiPattern.Description)
	}
	for _, tpl := range texts {
		if _, err := template.New("").Parse(tpl); err != nil {
			fail("gabarit invalide %q : %v", tpl, err)
		}
	}
	return errs
}

func fixTexts(e *Entry) []string {
	var out []string
	for _, f := range e.Fixes {
		out = append(out, f.Description, f.Command, f.Manifest)
	}
	return out
}

// ByIssueKind renvoie l'entrée pédagogique d'un type de problème de manifeste.
func (c *Catalog) ByIssueKind(kind string) (*Entry, bool) {
	for _, e := range c.Entries {
		for _, k := range e.IssueKinds {
			if k == kind {
				return e, true
			}
		}
	}
	return nil, false
}

// ByPodReason renvoie l'entrée qui explique un état de pod (CrashLoopBackOff…).
func (c *Catalog) ByPodReason(reason string) (*Entry, bool) {
	for _, e := range c.Entries {
		for _, r := range e.PodReasons {
			if r == reason {
				return e, true
			}
		}
	}
	return nil, false
}

// looksFrench vérifie la présence d'une lettre accentuée française ou d'un mot-outil.
func looksFrench(s string) bool {
	if strings.ContainsAny(s, "àâçéèêëîïôûùüÿœÀÂÇÉÈÊËÎÏÔÛÙÜŸŒ") {
		return true
	}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == ':' || r == ';' || r == '(' || r == ')' || r == '\n'
	})
	for _, w := range words {
		for _, fw := range frenchWords {
			if w == fw || (strings.HasSuffix(fw, "'") && strings.HasPrefix(w, fw)) {
				return true
			}
		}
	}
	return false
}

// Render remplit un gabarit avec les valeurs capturées. Une valeur absente est rendue
// par « … » plutôt que par une chaîne vide.
func Render(tpl string, captures map[string]string) string {
	t, err := template.New("").Option("missingkey=error").Parse(tpl)
	if err != nil {
		return tpl
	}
	var b strings.Builder
	if err := t.Execute(&b, captures); err != nil {
		fallback := map[string]string{}
		for _, name := range conditionFields(tpl) {
			fallback[name] = ""
		}
		for _, name := range templateFields(tpl) {
			if _, isCond := fallback[name]; !isCond {
				fallback[name] = "…"
			}
		}
		for k, v := range captures {
			fallback[k] = v
		}
		b.Reset()
		if err := t.Execute(&b, fallback); err != nil {
			return tpl
		}
	}
	return b.String()
}

var (
	fieldPattern = regexp.MustCompile(`\{\{\s*\.([A-Za-z0-9_]+)\s*\}\}`)
	condPattern  = regexp.MustCompile(`\{\{-?\s*(?:if|with)\s+\.([A-Za-z0-9_]+)`)
)

func conditionFields(tpl string) []string {
	var out []string
	for _, m := range condPattern.FindAllStringSubmatch(tpl, -1) {
		out = append(out, m[1])
	}
	return out
}

func templateFields(tpl string) []string {
	var out []string
	for _, m := range fieldPattern.FindAllStringSubmatch(tpl, -1) {
		out = append(out, m[1])
	}
	return out
}
