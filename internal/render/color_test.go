package render

import "testing"

func TestResolveColor(t *testing.T) {
	envOf := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}
	tests := []struct {
		name  string
		flag  string
		env   map[string]string
		isTTY bool
		want  bool
	}{
		{"auto sur un terminal", "auto", nil, true, true},
		{"auto hors terminal", "auto", nil, false, false},
		{"valeur vide = auto", "", nil, true, true},
		{"always hors terminal", "always", nil, false, true},
		{"never sur un terminal", "never", nil, true, false},
		{"NO_COLOR désactive en auto", "auto", map[string]string{"NO_COLOR": "1"}, true, false},
		{"NO_COLOR toute valeur non vide", "auto", map[string]string{"NO_COLOR": "oui"}, true, false},
		{"NO_COLOR vide ignoré", "auto", map[string]string{"NO_COLOR": ""}, true, true},
		{"CLICOLOR_FORCE=1 équivaut à always", "auto", map[string]string{"CLICOLOR_FORCE": "1"}, false, true},
		{"--color=never l'emporte sur CLICOLOR_FORCE", "never", map[string]string{"CLICOLOR_FORCE": "1"}, true, false},
		{"--color=always l'emporte sur NO_COLOR", "always", map[string]string{"NO_COLOR": "1"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveColor(tt.flag, envOf(tt.env), tt.isTTY); got != tt.want {
				t.Errorf("ResolveColor(%q, %v, %v) = %v, attendu %v", tt.flag, tt.env, tt.isTTY, got, tt.want)
			}
		})
	}
}

func TestParseColorMode(t *testing.T) {
	for _, ok := range []string{"auto", "always", "never", ""} {
		if _, err := ParseColorMode(ok); err != nil {
			t.Errorf("ParseColorMode(%q) : erreur inattendue %v", ok, err)
		}
	}
	if _, err := ParseColorMode("rainbow"); err == nil {
		t.Error("ParseColorMode(rainbow) : une erreur était attendue")
	}
}
