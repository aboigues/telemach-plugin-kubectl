package cron

import (
	"strings"
	"testing"
	"time"
)

// Expressions acceptées par le contrôleur CronJob (format standard à 5 champs).
func TestParseValid(t *testing.T) {
	for _, expr := range []string{
		"*/5 * * * *", "0 2 * * *", "30 8 1 * *", "0 0 * * 0", "0 9-17 * * MON-FRI",
		"15,45 * * * *", "0 0 1 JAN *", "0 12 ? * SUN", "5/15 * * * *", "0 0 1-7 * 1",
		"@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually",
		"  0  2  *  *  *  ",
	} {
		if _, err := Parse(expr); err != nil {
			t.Errorf("%q refusée : %v", expr, err)
		}
	}
}

// Chaque erreur nomme le champ en cause, en français, avec sa position.
func TestParseInvalid(t *testing.T) {
	for _, tc := range []struct{ expr, field, want string }{
		{"*/5 * * *", "", "4 champs"},
		{"* * * * * *", "", "6 champs"},
		{"", "", "vide"},
		{"60 * * * *", "minute", "0 à 59"},
		{"0 24 * * *", "heure", "0 à 23"},
		{"0 0 0 * *", "jour du mois", "1 à 31"},
		{"0 0 * 13 *", "mois", "1 à 12"},
		{"0 0 * * 8", "jour de la semaine", "0 à 6"},
		{"0 0 * FOO *", "mois", "FOO"},
		{"*/0 * * * *", "minute", "pas"},
		{"10-5 * * * *", "minute", "10-5"},
		{"a * * * *", "minute", "« a »"},
		{"TZ=Europe/Paris 0 2 * * *", "", "spec.timeZone"},
		{"CRON_TZ=UTC 0 2 * * *", "", "spec.timeZone"},
		{"@reboot", "", "@reboot"},
	} {
		_, err := Parse(tc.expr)
		var e *Error
		if err == nil {
			t.Errorf("%q acceptée", tc.expr)
			continue
		}
		e, _ = err.(*Error)
		if e == nil || e.Field != tc.field || !strings.Contains(e.Message, tc.want) {
			t.Errorf("%q : erreur %#v, attendu champ %q et message contenant %q", tc.expr, err, tc.field, tc.want)
		}
	}
	if _, err := Parse("0 0 * 13 *"); err.(*Error).Position != 7 {
		t.Errorf("position du champ mois : %d, attendu 7", err.(*Error).Position)
	}
}

// Next calcule la prochaine échéance, fuseau compris (spec.timeZone).
func TestNext(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("base des fuseaux indisponible")
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, paris)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct{ expr, after, want string }{
		{"*/5 * * * *", "2026-10-29 10:02", "2026-10-29 10:05"},
		{"0 2 * * *", "2026-10-29 10:02", "2026-10-30 02:00"},
		{"0 0 1 1 *", "2026-10-29 10:02", "2027-01-01 00:00"},
		{"0 9 * * MON-FRI", "2026-10-30 10:00", "2026-11-02 09:00"}, // vendredi → lundi
		{"0 0 13 * 5", "2026-10-29 10:00", "2026-10-30 00:00"},      // jour 13 OU vendredi
		{"@monthly", "2026-10-29 10:02", "2026-11-01 00:00"},
		// Passage à l'heure d'hiver le 2026-10-25 à 3 h : l'échéance de 2 h existe toujours.
		{"0 2 * * *", "2026-10-24 23:00", "2026-10-25 02:00"},
		// Passage à l'heure d'été le 2026-03-29 : 2 h 30 n'existe pas, l'échéance suivante est le lendemain.
		{"30 2 * * *", "2026-03-29 00:00", "2026-03-30 02:30"},
	} {
		s, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("%q : %v", tc.expr, err)
		}
		got := s.Next(at(tc.after), paris)
		if want := at(tc.want); !got.Equal(want) {
			t.Errorf("%q après %s : %s, attendu %s", tc.expr, tc.after, got.In(paris).Format("2006-01-02 15:04"), tc.want)
		}
	}
}

// Une expression qui ne se produit jamais (31 février) n'a pas d'échéance.
func TestNextNever(t *testing.T) {
	s, err := Parse("0 0 31 2 *")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC); !got.IsZero() {
		t.Errorf("aucune échéance attendue, obtenu %s", got)
	}
}
