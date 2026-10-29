package render

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Libellés et symboles de chaque bloc. Ils restent affichés sans couleur : la couleur
// n'est jamais le seul porteur d'information (FR-007a).
const (
	SymbolError    = "✖"
	LabelConcept   = "📘 Concept"
	LabelCause     = "🔎 Cause probable"
	LabelFixes     = "💡 Corrections"
	LabelAnti      = "⚠ Anti-pattern"
	SymbolSuccess  = "✔"
	LabelDeepen    = "➜ Pour aller plus loin"
	LabelOriginal  = "Message kubectl d'origine"
	LabelUnknown   = "Erreur non reconnue"
	LabelExercise  = "🧪 Exercice"
	LabelExample   = "▶ Exemple"
	LabelDocLink   = "📖 Documentation"
	LabelRepeated  = "↺ Même erreur que précédemment"
	LabelLast      = "↺ Explication complète"
	LabelProblem   = "✖ Problème"
	LabelInfo      = "Information"
	LabelFixedPart = "💡 Version corrigée"
	SymbolWarning  = "⚠"
	SymbolInfo     = "ℹ"
	SymbolReminder = "↺"
	LabelFix       = "💡 Correction"
	LabelPartial   = "◐ Diagnostic partiel"
	LabelRules     = "Règles Kubernetes"
	LabelLog       = "📜 Fin du journal"
	LabelOthers    = "Autres pods en difficulté"
	LabelPlacement = "Nœuds par cause"
)

// Theme applique les styles de chaque rôle. Sans couleur, il renvoie le texte tel quel.
type Theme struct {
	Color bool
	Dark  bool

	title, concept, cause, fix, anti, dim, highlight, success, hint, label lipgloss.Style
}

// NewTheme construit le thème. La palette unique s'adapte au fond clair ou sombre.
func NewTheme(colorOn, dark bool) Theme {
	t := Theme{Color: colorOn, Dark: dark}
	if !colorOn {
		return t
	}
	ld := lipgloss.LightDark(dark)
	c := func(light, darkHex string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(darkHex)) }

	t.title = lipgloss.NewStyle().Bold(true).Foreground(c("#B3261E", "#FF7A7A"))
	t.concept = lipgloss.NewStyle().Bold(true).Foreground(c("#1D4ED8", "#7AA2F7"))
	t.cause = lipgloss.NewStyle().Bold(true).Foreground(c("#9A6700", "#E0AF68"))
	t.fix = lipgloss.NewStyle().Bold(true).Foreground(c("#1A7F37", "#9ECE6A"))
	t.anti = lipgloss.NewStyle().Bold(true).Foreground(c("#A21CAF", "#D7A0FF"))
	t.dim = lipgloss.NewStyle().Faint(true).Foreground(c("#6B7280", "#8A8F98"))
	t.highlight = lipgloss.NewStyle().Bold(true).Foreground(c("#0E7490", "#7DCFFF"))
	t.success = lipgloss.NewStyle().Bold(true).Foreground(c("#1A7F37", "#9ECE6A"))
	t.hint = lipgloss.NewStyle().Italic(true).Foreground(c("#0E7490", "#7DCFFF"))
	t.label = lipgloss.NewStyle().Bold(true)
	return t
}

func (t Theme) apply(s lipgloss.Style, text string) string {
	if !t.Color || text == "" {
		return text
	}
	return s.Render(text)
}

func (t Theme) Title(s string) string    { return t.apply(t.title, s) }
func (t Theme) Concept(s string) string  { return t.apply(t.concept, s) }
func (t Theme) Cause(s string) string    { return t.apply(t.cause, s) }
func (t Theme) Fix(s string) string      { return t.apply(t.fix, s) }
func (t Theme) Anti(s string) string     { return t.apply(t.anti, s) }
func (t Theme) Dim(s string) string      { return t.apply(t.dim, s) }
func (t Theme) Emphasis(s string) string { return t.apply(t.highlight, s) }
func (t Theme) Success(s string) string  { return t.apply(t.success, s) }
func (t Theme) Hint(s string) string     { return t.apply(t.hint, s) }
func (t Theme) Label(s string) string    { return t.apply(t.label, s) }
