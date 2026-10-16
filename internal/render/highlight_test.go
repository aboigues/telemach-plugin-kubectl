package render

import (
	"strings"
	"testing"
)

func TestHighlightWithoutColor(t *testing.T) {
	th := NewTheme(false, true)
	code := "kubectl get pods -n demo"
	if got := th.Highlight(code, "bash"); got != code {
		t.Errorf("sans couleur, le code doit être inchangé : %q", got)
	}
}

func TestHighlightWithColor(t *testing.T) {
	th := NewTheme(true, true)
	got := th.Highlight("spec:\n  containers:\n  - image: nginx", "yaml")
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("une séquence de couleur était attendue : %q", got)
	}
	if strings.Count(got, "\n") != 2 {
		t.Errorf("le nombre de lignes doit être conservé : %q", got)
	}
}

func TestHighlightLinesPlain(t *testing.T) {
	th := NewTheme(false, true)
	lines := []Line{{8, "spec:"}, {9, "  containers:"}, {10, "\t- name: web"}}
	got := th.HighlightLines(lines, "yaml", 10, 1)
	want := "   8 │ spec:\n   9 │   containers:\n➤ 10 │ →- name: web\n       ^"
	if got != want {
		t.Errorf("rendu inattendu :\n%s\n--- attendu ---\n%s", got, want)
	}
	if strings.Contains(got, "\x1b") {
		t.Error("aucune séquence ESC attendue sans couleur")
	}
}

func TestThemeWithoutColorKeepsText(t *testing.T) {
	th := NewTheme(false, false)
	for _, s := range []string{th.Title("a"), th.Concept("a"), th.Dim("a"), th.Highlight("a", "bash")} {
		if s != "a" {
			t.Errorf("sans couleur, le texte doit être inchangé : %q", s)
		}
	}
}

func TestHighlightLinesRelated(t *testing.T) {
	th := NewTheme(false, true)
	lines := []Line{{7, "  selector:"}, {8, "    matchLabels:"}, {9, "      app: web"}, {14, "        app: api"}}
	got := th.HighlightLines(lines, "yaml", 9, 12, 14)
	want := "   7 │   selector:\n   8 │     matchLabels:\n➤  9 │       app: web\n" + strings.Repeat(" ", 18) + "^\n› 14 │         app: api"
	if got != want {
		t.Errorf("rendu inattendu :\n%s\n--- attendu ---\n%s", got, want)
	}
}
