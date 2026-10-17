package render

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
)

// Options règle le rendu d'une explication.
type Options struct {
	Theme          Theme
	Width          int    // largeur de mise en page (40 minimum)
	KubectlVersion string // version du client kubectl, si connue (FR-001b)
	Repeated       bool   // même erreur depuis moins de 10 minutes : rendu court
	Interactive    bool   // stderr non capturé
	ExitCode       int
}

const indent = "   "

// RenderExplanation écrit l'explication d'un diagnostic, en français, dans l'ordre :
// résumé, concept, cause, corrections, anti-pattern, message d'origine, approfondissement.
func RenderExplanation(w io.Writer, d diagnose.Diagnosis, original string, o Options) error {
	t := o.Theme
	width := max(o.Width, 40)
	var b strings.Builder

	if d.Entry == nil {
		renderUnknown(&b, d, original, o, width)
		_, err := io.WriteString(w, b.String())
		return err
	}
	e := d.Entry
	summary := catalog.Render(e.Summary, d.Captures)

	if o.Repeated {
		b.WriteString(t.Title(SymbolError+" "+summary) + "\n")
		b.WriteString(t.Dim(LabelRepeated+" (moins de 10 minutes).") + "\n")
		b.WriteString(t.Hint(LabelLast+" : "+telemach(d.Captures)+" last") + "\n")
		b.WriteString(t.Hint(LabelDeepen+" : "+telemach(d.Captures)+" deepen") + "\n")
		_, err := io.WriteString(w, b.String())
		return err
	}

	b.WriteString(t.Title(SymbolError+" "+summary) + "\n")
	if d.Origin == diagnose.OriginApplication {
		b.WriteString(indent + t.Dim("(l'erreur vient du programme exécuté dans le conteneur, pas de Kubernetes)") + "\n")
	}
	b.WriteString("\n")

	writeLesson(&b, e, d.Captures, o, width)
	writeOriginal(&b, original, d.Captures, o, width)
	b.WriteString("\n" + t.Hint(LabelDeepen+" : "+telemach(d.Captures)+" deepen") + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// writeLesson écrit le cœur d'une leçon : concept, cause, corrections et anti-pattern
// éventuel. Il sert aux explications d'erreurs et aux diagnostics d'objets (feature 002).
func writeLesson(b *strings.Builder, e *catalog.Entry, captures map[string]string, o Options, width int) {
	t := o.Theme
	b.WriteString(t.Concept(LabelConcept+" — "+e.Concept.Name) + "\n")
	b.WriteString(wrap(e.Concept.Definition, width, indent) + "\n\n")

	b.WriteString(t.Cause(LabelCause) + "\n")
	b.WriteString(wrap(catalog.Render(e.Cause, captures), width, indent) + "\n\n")

	b.WriteString(t.Fix(LabelFixes) + "\n")
	for i, f := range e.Fixes {
		prefix := fmt.Sprintf("%s%d. ", indent, i+1)
		desc := catalog.Render(f.Description, captures)
		if !f.Safe {
			desc = t.Anti("(déconseillé)") + " " + desc
		}
		b.WriteString(prefix + wrapRest(desc, width, strings.Repeat(" ", utf8.RuneCountInString(prefix))) + "\n")
		pad := strings.Repeat(" ", utf8.RuneCountInString(prefix))
		if f.Command != "" {
			b.WriteString(pad + t.Highlight(catalog.Render(f.Command, captures), "bash") + "\n")
		}
		if f.Manifest != "" {
			code := t.Highlight(strings.TrimRight(catalog.Render(f.Manifest, captures), "\n"), "yaml")
			b.WriteString(pad + strings.ReplaceAll(code, "\n", "\n"+pad) + "\n")
		}
	}
	if e.AntiPattern != nil {
		b.WriteString("\n" + t.Anti(LabelAnti) + "\n")
		b.WriteString(wrap(catalog.Render(e.AntiPattern.Description, captures), width, indent) + "\n")
	}
}

// renderUnknown affiche une erreur non reconnue sans jamais avancer de cause (FR-008).
func renderUnknown(b *strings.Builder, d diagnose.Diagnosis, original string, o Options, width int) {
	t := o.Theme
	b.WriteString(t.Title(SymbolError+" "+LabelUnknown) + "\n")
	b.WriteString(wrap("TELEMACH ne reconnaît pas encore cette erreur et préfère ne pas deviner sa cause.", width, indent) + "\n")
	ctx := fmt.Sprintf("Commande : %s %s — code de sortie %d", kubectlName(d.Captures), d.Captures["sub"], o.ExitCode)
	if o.KubectlVersion != "" {
		ctx += " — kubectl " + o.KubectlVersion
	}
	b.WriteString(indent + t.Dim(ctx) + "\n")
	if d.Origin == diagnose.OriginApplication {
		b.WriteString(wrap("Le code de sortie indique que l'erreur vient du programme exécuté dans le conteneur : consulte ses journaux avec « kubectl logs ».", width, indent) + "\n")
	}
	if o.Interactive {
		b.WriteString(wrap("Le message d'erreur n'a pas été capté, car la commande était interactive.", width, indent) + "\n")
	}
	b.WriteString(wrap("Piste : relis attentivement le message de kubectl ; la documentation officielle est sur https://kubernetes.io/docs/", width, indent) + "\n")
	writeOriginal(b, original, d.Captures, o, width)
}

// writeOriginal réaffiche le message kubectl d'origine, atténué, avec les fragments
// reconnus (port, image, clé, chemin…) surlignés (FR-007).
func writeOriginal(b *strings.Builder, original string, captures map[string]string, o Options, width int) {
	original = strings.TrimRight(original, "\n")
	if original == "" {
		return
	}
	t := o.Theme
	title := "── " + LabelOriginal + " "
	b.WriteString("\n" + t.Dim(title+strings.Repeat("─", max(3, width-utf8.RuneCountInString(title)))) + "\n")
	fragments := highlightFragments(captures)
	for _, line := range strings.Split(original, "\n") {
		b.WriteString(emphasize(line, fragments, t) + "\n")
	}
}

// highlightFragments renvoie les valeurs capturées à surligner, les plus longues d'abord.
func highlightFragments(captures map[string]string) []string {
	var out []string
	for k, v := range captures {
		if !highlightable[k] || len(v) < 2 || strings.Contains(v, "••") {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// emphasize atténue la ligne et surligne chaque occurrence des fragments.
func emphasize(line string, fragments []string, t Theme) string {
	if !t.Color {
		return line
	}
	var b strings.Builder
	rest := line
	for rest != "" {
		pos, frag := -1, ""
		for _, f := range fragments {
			if i := strings.Index(rest, f); i >= 0 && (pos < 0 || i < pos) {
				pos, frag = i, f
			}
		}
		if pos < 0 {
			b.WriteString(t.Dim(rest))
			break
		}
		b.WriteString(t.Dim(rest[:pos]))
		b.WriteString(t.Emphasis(frag))
		rest = rest[pos+len(frag):]
	}
	return b.String()
}

// wrap coupe un paragraphe à la largeur donnée, chaque ligne étant précédée de prefix.
func wrap(text string, width int, prefix string) string {
	return prefix + wrapRest(text, width, prefix)
}

// wrapRest coupe un paragraphe dont la première ligne est déjà préfixée par l'appelant.
func wrapRest(text string, width int, prefix string) string {
	avail := max(width-utf8.RuneCountInString(prefix), 20)
	// Typographie française : les guillemets restent collés à leur contenu (espace insécable).
	text = strings.NewReplacer("« ", "«\u00a0", " »", "\u00a0»").Replace(text)
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.FieldsFunc(para, func(r rune) bool { return r == ' ' || r == '\t' })
		cur := ""
		for _, w := range words {
			if cur != "" && utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) > avail {
				lines = append(lines, cur)
				cur = w
				continue
			}
			if cur != "" {
				cur += " "
			}
			cur += w
		}
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n"+prefix)
}

// Wrap coupe un paragraphe à la largeur donnée, chaque ligne étant précédée de prefix.
func Wrap(text string, width int, prefix string) string { return wrap(text, max(width, 40), prefix) }

// highlightable liste les captures surlignées dans le message d'origine (FR-007) : les
// captures « techniques » (sous-commande, nom de la commande, options de portée) en sont
// exclues.
var highlightable = map[string]bool{}

func init() {
	for _, k := range []string{
		"type", "name", "namespace", "context", "field", "file", "line",
		"resource", "kind", "verb", "user", "group", "path", "flag", "command", "host", "port",
		"api_version", "value", "reason", "image", "container", "subject",
	} {
		highlightable[k] = true
	}
}

// kubectlName renvoie « kubectl » ou « k », selon ce qu'a tapé l'apprenant.
func kubectlName(captures map[string]string) string {
	if k := captures["kubectl"]; k != "" {
		return k
	}
	return "kubectl"
}

// telemach renvoie la commande du plugin à proposer (« kubectl telemach » ou « k telemach »).
func telemach(captures map[string]string) string { return kubectlName(captures) + " telemach" }
