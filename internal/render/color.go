package render

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// ColorMode est la valeur de l'option --color.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// ParseColorMode valide la valeur de l'option --color.
func ParseColorMode(s string) (ColorMode, error) {
	switch ColorMode(s) {
	case "", ColorAuto:
		return ColorAuto, nil
	case ColorAlways, ColorNever:
		return ColorMode(s), nil
	}
	return "", fmt.Errorf("valeur invalide pour --color : %q (valeurs possibles : auto, always, never)", s)
}

// ResolveColor décide si la sortie doit être colorée.
//   - --color=always ou --color=never l'emportent sur tout le reste ;
//   - en mode auto : NO_COLOR (non vide) désactive, CLICOLOR_FORCE=1 force,
//     sinon la couleur n'est active que si la sortie est un terminal.
func ResolveColor(flag string, env func(string) string, isTTY bool) bool {
	switch ColorMode(flag) {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	}
	if env("NO_COLOR") != "" {
		return false
	}
	if env("CLICOLOR_FORCE") == "1" {
		return true
	}
	return isTTY
}

// IsTerminal indique si w est un terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd())) //nolint:gosec // descripteur de fichier valide
}

// NewWriter enveloppe w pour adapter les couleurs aux capacités du terminal (16, 256 ou
// 16 millions de couleurs). Quand la couleur est forcée sur une sortie qui n'est pas un
// terminal, le profil 256 couleurs est utilisé.
func NewWriter(w io.Writer, color bool) io.Writer {
	if !color {
		return w
	}
	p := colorprofile.Detect(w, os.Environ())
	if p < colorprofile.ANSI {
		p = colorprofile.ANSI256
	}
	return &colorprofile.Writer{Forward: w, Profile: p}
}

// HasDarkBackground devine si le fond du terminal est sombre : d'abord par la variable
// COLORFGBG (« texte;fond »), puis en interrogeant le terminal quand stdin et la sortie en
// sont un. Par défaut, le fond est supposé sombre.
func HasDarkBackground(out io.Writer, env func(string) string) bool {
	if v := env("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if bg, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			// Couleurs ANSI 0 à 6 et 8 : fonds sombres ; 7 et 9 à 15 : fonds clairs.
			return bg <= 6 || bg == 8
		}
	}
	f, ok := out.(*os.File)
	if !ok || !IsTerminal(f) || !IsTerminal(os.Stdin) {
		return true
	}
	return lipgloss.HasDarkBackground(os.Stdin, f)
}

// TerminalWidth renvoie la largeur du terminal associé à w, ou 80 par défaut.
func TerminalWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width >= 40 { //nolint:gosec // descripteur valide
			return min(width, 100)
		}
	}
	return 80
}
