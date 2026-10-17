package sanitize

import (
	"strings"
	"unicode/utf8"
)

// Clean retire d'un texte non fiable toutes les séquences d'échappement ANSI (CSI, OSC,
// DCS…) et tous les caractères de contrôle, sauf le saut de ligne et la tabulation.
// Les octets UTF-8 invalides sont remplacés par U+FFFD. Le résultat peut être affiché
// sans risque d'injection de séquences de terminal.
func Clean(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == 0x1b {
			i = skipEscape(s, i+size)
			continue
		}
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
			i++
			continue
		}
		i += size
		if isControl(r) {
			// Un CSI codé sur 8 bits (U+009B) ou un OSC (U+009D) introduit aussi une séquence.
			switch r {
			case 0x9b:
				i = skipCSI(s, i)
			case 0x90, 0x9d, 0x9e, 0x9f:
				i = skipString(s, i)
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// isControl indique si r est un caractère de contrôle à supprimer.
func isControl(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// skipEscape saute la séquence qui suit un ESC situé juste avant l'indice i.
func skipEscape(s string, i int) int {
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[':
		return skipCSI(s, i+1)
	case ']', 'P', '^', '_', 'X':
		return skipString(s, i+1)
	default:
		// Séquence à deux caractères (ESC suivi d'un octet intermédiaire ou final).
		if s[i] >= 0x20 && s[i] <= 0x7e {
			return i + 1
		}
		return i
	}
}

// skipCSI saute les paramètres et l'octet final d'une séquence CSI.
func skipCSI(s string, i int) int {
	for i < len(s) {
		c := s[i]
		i++
		if c >= 0x40 && c <= 0x7e {
			return i
		}
		if c < 0x20 || c > 0x3f && c < 0x40 {
			return i
		}
	}
	return i
}

// skipString saute une chaîne de contrôle (OSC, DCS…) terminée par BEL ou ST (ESC \).
func skipString(s string, i int) int {
	for i < len(s) {
		switch {
		case s[i] == 0x07:
			return i + 1
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\':
			return i + 2
		case s[i] == 0x1b:
			return i + 1
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == 0x9c {
			return i + size
		}
		i += size
	}
	return i
}
