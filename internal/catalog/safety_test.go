package catalog

import (
	"regexp"
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/tests/manifests"
)

// Formes interdites dans une commande d'exemple ou une correction sûre (FR-010, principe III).
var forbidden = []*regexp.Regexp{
	regexp.MustCompile(`--privileged`),
	regexp.MustCompile(`\bcluster-admin\b`),
	regexp.MustCompile(`privileged:\s*true`),
	regexp.MustCompile(`hostNetwork:\s*true`),
	regexp.MustCompile(`hostPID:\s*true`),
	regexp.MustCompile(`\bhostPath:`),
	regexp.MustCompile(`--as=system:admin|--as=system:masters`),
}

// TestExamplesAreSafe applique les règles de sécurité de la constitution aux exemples produits
// par le catalogue : manifestes (règles de manifest-lint) et commandes. Une correction
// marquée « safe: false » est exemptée : le chargeur exige alors un anti_pattern.
func TestExamplesAreSafe(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Entries {
		type sample struct{ label, text string }
		samples := []sample{{"deepen.example", e.Deepen.Example}, {"deepen.exercise", e.Deepen.Exercise}}
		for i, f := range e.Fixes {
			if !f.Safe {
				continue
			}
			samples = append(samples, sample{fixLabel(i, "command"), f.Command}, sample{fixLabel(i, "manifest"), f.Manifest})
		}
		for _, s := range samples {
			text := Render(s.text, map[string]string{"kubectl": "kubectl"})
			if text == "" {
				continue
			}
			for _, re := range forbidden {
				if re.MatchString(text) {
					t.Errorf("%s (%s) : forme interdite %q :\n%s", e.ID, s.label, re, text)
				}
			}
			if redact.Redact(text) != text {
				t.Errorf("%s (%s) : secret littéral dans l'exemple :\n%s", e.ID, s.label, text)
			}
			if strings.Contains(text, "apiVersion:") {
				violations, warnings := manifests.Check([]byte(text))
				for _, v := range violations {
					t.Errorf("%s (%s) : %s", e.ID, s.label, v)
				}
				for _, w := range warnings {
					t.Logf("⚠ %s (%s) : %s (avertissement non bloquant)", e.ID, s.label, w)
				}
			}
		}
	}
}

func fixLabel(i int, field string) string {
	return "fixes[" + string(rune('0'+i)) + "]." + field
}
