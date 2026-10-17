package sanitize

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"texte simple", `pods "web" not found`, `pods "web" not found`},
		{"accents conservés", "Le pod n'est pas démarré : vérifiez", "Le pod n'est pas démarré : vérifiez"},
		{"saut de ligne et tabulation", "ligne 1\n\tligne 2", "ligne 1\n\tligne 2"},
		{"séquence CSI couleur", "\x1b[31merreur\x1b[0m", "erreur"},
		{"séquence CSI curseur", "a\x1b[2Kb\x1b[1;1Hc", "abc"},
		{"séquence OSC titre (BEL)", "x\x1b]0;titre piégé\x07y", "xy"},
		{"séquence OSC hyperlien (ST)", "x\x1b]8;;https://exemple\x1b\\lien\x1b]8;;\x1b\\y", "xlieny"},
		{"ESC final", "ab\x1b", "ab"},
		{"séquence ESC à deux caractères", "a\x1bcb", "ab"},
		{"caractères de contrôle C0", "a\x00b\x07c\x08d\re", "abcde"},
		{"DEL", "a\x7fb", "ab"},
		{"contrôle C1", "a\u0085b", "ab"},
		{"CSI sur 8 bits", "a\u009b31mb", "ab"},
		{"UTF-8 invalide", "a\xffb", "a�b"},
		{"vide", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean(%q) = %q, attendu %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzSanitize(f *testing.F) {
	for _, seed := range []string{"", "plain", "\x1b[31mred\x1b[0m", "\x1b]0;t\x07", "é\x1b", "\x9b31m"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Clean(in)
		if strings.ContainsRune(out, 0x1b) {
			t.Fatalf("séquence ESC présente en sortie : %q", out)
		}
		for _, r := range out {
			if isControl(r) {
				t.Fatalf("caractère de contrôle %U présent en sortie : %q", r, out)
			}
		}
		if Clean(out) != out {
			t.Fatalf("Clean n'est pas idempotent : %q", out)
		}
	})
}
