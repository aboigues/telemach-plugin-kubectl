package render

import (
	"embed"
	"fmt"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
)

// Seules les définitions utilisées sont embarquées et chargées à la première coloration :
// les paquets lexers et styles de chroma enregistrent au démarrage plusieurs centaines de
// définitions (environ 17 ms à chaque appel du plugin, voir SC-003 de la feature 003).
//
//go:embed syntax/*.xml
var syntaxFiles embed.FS

var (
	syntaxOnce sync.Once
	syntaxLex  *chroma.LexerRegistry
	syntaxCSS  map[string]*chroma.Style
)

// loadSyntax charge les langages (bash, json, yaml) et les styles (github, monokai).
func loadSyntax() {
	syntaxLex = chroma.NewLexerRegistry()
	for _, name := range []string{"bash", "json", "yaml"} {
		if l, err := chroma.NewXMLLexer(syntaxFiles, "syntax/"+name+".xml"); err == nil {
			syntaxLex.Register(l)
		}
	}
	syntaxCSS = map[string]*chroma.Style{}
	for _, name := range []string{"github", "monokai"} {
		f, err := syntaxFiles.Open("syntax/" + name + ".xml")
		if err != nil {
			continue
		}
		if st, err := chroma.NewXMLStyle(f); err == nil {
			syntaxCSS[name] = st
		}
		_ = f.Close()
	}
}

// TabMarker représente une tabulation dans les extraits affichés.
const TabMarker = "→"

// Line est une ligne numérotée d'un extrait de fichier.
type Line struct {
	Number int
	Text   string
}

// Highlight colore du code (lexer « bash », « yaml » ou « json »). Sans couleur, le code
// est renvoyé tel quel.
func (t Theme) Highlight(code, lexer string) string {
	if !t.Color || code == "" {
		return code
	}
	syntaxOnce.Do(loadSyntax)
	l := syntaxLex.Get(lexer)
	if l == nil {
		return code
	}
	l = chroma.Coalesce(l)
	styleName := "github"
	if t.Dark {
		styleName = "monokai"
	}
	it, err := l.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var b strings.Builder
	style := syntaxCSS[styleName]
	if style == nil {
		return code
	}
	if err := formatters.TTY16m.Format(&b, style, it); err != nil {
		return code
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// HighlightLines affiche un extrait numéroté. La ligne focusLine est marquée par « ➤ »
// (et surlignée en couleur) ; si focusCol > 0, un « ^ » est placé sous la colonne fautive.
// Les lignes de related (autres lignes concernées, par exemple les labels d'un template
// pour un selector incohérent) sont marquées par « › ».
func (t Theme) HighlightLines(lines []Line, lexer string, focusLine, focusCol int, related ...int) string {
	if len(lines) == 0 {
		return ""
	}
	width := len(fmt.Sprint(lines[len(lines)-1].Number))
	texts := make([]string, len(lines))
	for i, l := range lines {
		// Les tabulations sont rendues visibles (une cellule) : c'est pédagogique pour les
		// erreurs d'indentation YAML et cela garde le marqueur de colonne aligné.
		texts[i] = strings.ReplaceAll(l.Text, "\t", TabMarker)
	}
	colored := strings.Split(t.Highlight(strings.Join(texts, "\n"), lexer), "\n")
	if len(colored) != len(lines) {
		colored = texts
	}
	var b strings.Builder
	for i, l := range lines {
		marker := "  "
		gutter := fmt.Sprintf("%*d │ ", width, l.Number)
		switch {
		case l.Number == focusLine:
			marker = t.Title("➤ ")
			gutter = t.Title(gutter)
		case containsInt(related, l.Number):
			marker = t.Cause("› ")
			gutter = t.Cause(gutter)
		default:
			gutter = t.Dim(gutter)
		}
		b.WriteString(marker + gutter + colored[i] + "\n")
		if l.Number == focusLine && focusCol > 0 {
			pad := strings.Repeat(" ", 2+width+3+visualColumn(texts[i], focusCol))
			b.WriteString(pad + t.Title("^") + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// visualColumn convertit une colonne (à partir de 1, en runes) en décalage d'affichage,
// une tabulation comptant pour une seule cellule comme dans l'extrait affiché.
func visualColumn(text string, col int) int {
	n := 0
	for i := range []rune(text) {
		if i >= col-1 {
			break
		}
		n++
	}
	return n
}

func containsInt(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}
