package cron

import (
	"testing"
	"time"
)

// FuzzParse : aucune expression ne provoque de panique ; une expression acceptée a une
// échéance postérieure à l'instant donné, ou aucune.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"*/5 * * * *", "0 9-17 * * MON-FRI", "@daily", "1-5/2 */3 1,15 JAN-MAR ?", "TZ=UTC 0 0 * * *", "60 * * * *"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		s, err := Parse(expr)
		if err != nil {
			if err.Error() == "" {
				t.Errorf("%q : message d'erreur vide", expr)
			}
			return
		}
		from := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
		if next := s.Next(from, time.UTC); !next.IsZero() && !next.After(from) {
			t.Errorf("%q : échéance %s non postérieure à %s", expr, next, from)
		}
	})
}
