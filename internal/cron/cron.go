// Package cron analyse l'expression « spec.schedule » d'un CronJob, comme le contrôleur de
// Kubernetes (format standard à 5 champs), sans dépendance externe (feature 004,
// research.md F9) : check la valide hors ligne, diagnose calcule la prochaine échéance.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Error est une expression refusée. Field est le nom français du champ en cause (vide pour
// une erreur sur l'expression entière) et Position sa colonne dans l'expression (1-based).
type Error struct {
	Field    string
	Position int
	Message  string
}

func (e *Error) Error() string { return e.Message }

// Schedule est une expression analysée : pour chaque champ, l'ensemble des valeurs permises.
type Schedule struct {
	Expr              string
	minute, hour, dom uint64
	month, dow        uint64
	domStar, dowStar  bool // « * » ou « ? » : le jour se décide sur l'autre champ
	any               bool // @every : accepté, sans échéance calculée
}

// field décrit un des 5 champs.
type field struct {
	name     string
	min, max int
	names    map[string]int
}

var fields = []field{
	{name: "minute", min: 0, max: 59},
	{name: "heure", min: 0, max: 23},
	{name: "jour du mois", min: 1, max: 31},
	{name: "mois", min: 1, max: 12, names: map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}},
	{name: "jour de la semaine", min: 0, max: 6, names: map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
	}},
}

// macros sont les raccourcis acceptés par le contrôleur CronJob.
var macros = map[string]string{
	"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
	"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
}

// Parse analyse une expression cron. L'erreur renvoyée est toujours une *Error, en français.
func Parse(expr string) (*Schedule, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, &Error{Message: "L'expression cron est vide : il faut 5 champs (minute, heure, jour du mois, mois, jour de la semaine)."}
	}
	if strings.HasPrefix(trimmed, "TZ=") || strings.HasPrefix(trimmed, "CRON_TZ=") {
		return nil, &Error{Position: 1, Message: "Le fuseau horaire ne s'écrit pas dans l'expression (TZ= ou CRON_TZ=) : utilise le champ spec.timeZone du CronJob, par exemple « Europe/Paris »."}
	}
	if strings.HasPrefix(trimmed, "@") {
		std, ok := macros[strings.ToLower(trimmed)]
		if !ok {
			if strings.HasPrefix(strings.ToLower(trimmed), "@every") {
				// Accepté par la bibliothèque du contrôleur : pas de cause inventée.
				return &Schedule{Expr: expr, any: true}, nil
			}
			return nil, &Error{Position: 1, Message: fmt.Sprintf("« %s » n'est pas un raccourci cron reconnu (@hourly, @daily, @weekly, @monthly, @yearly).", trimmed)}
		}
		s, err := Parse(std)
		if err == nil {
			s.Expr = expr
		}
		return s, err
	}
	parts, positions := splitFields(expr)
	if len(parts) != len(fields) {
		return nil, &Error{Message: fmt.Sprintf("L'expression cron « %s » a %d champs ; il en faut 5 : minute, heure, jour du mois, mois, jour de la semaine.", trimmed, len(parts))}
	}
	s := &Schedule{Expr: expr}
	sets := []*uint64{&s.minute, &s.hour, &s.dom, &s.month, &s.dow}
	for i, part := range parts {
		set, star, err := parseField(part, fields[i])
		if err != nil {
			err.Field, err.Position = fields[i].name, positions[i]
			return nil, err
		}
		*sets[i] = set
		switch i {
		case 2:
			s.domStar = star
		case 4:
			s.dowStar = star
		}
	}
	return s, nil
}

// splitFields découpe l'expression en champs et retient la colonne de chacun.
func splitFields(expr string) ([]string, []int) {
	var parts []string
	var positions []int
	start := -1
	for i, r := range expr + " " {
		if r == ' ' || r == '\t' {
			if start >= 0 {
				parts = append(parts, expr[start:i])
				positions = append(positions, start+1)
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return parts, positions
}

// parseField analyse un champ : listes (,), intervalles (-), pas (/), noms, « * » et « ? ».
func parseField(s string, f field) (uint64, bool, *Error) {
	var set uint64
	star := s == "*" || s == "?"
	for _, item := range strings.Split(s, ",") {
		rangePart, stepPart, hasStep := strings.Cut(item, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepPart)
			if err != nil || n <= 0 {
				return 0, false, &Error{Message: fmt.Sprintf("Le pas « /%s » du champ %s doit être un nombre supérieur à 0, par exemple */5.", stepPart, f.name)}
			}
			step = n
		}
		lo, hi := f.min, f.max
		switch {
		case rangePart == "*" || rangePart == "?":
		case strings.Contains(rangePart, "-"):
			a, b, _ := strings.Cut(rangePart, "-")
			var err *Error
			if lo, err = value(a, f); err != nil {
				return 0, false, err
			}
			if hi, err = value(b, f); err != nil {
				return 0, false, err
			}
			if lo > hi {
				return 0, false, &Error{Message: fmt.Sprintf("L'intervalle « %s » du champ %s va à rebours : écris la plus petite valeur d'abord.", rangePart, f.name)}
			}
		default:
			v, err := value(rangePart, f)
			if err != nil {
				return 0, false, err
			}
			lo = v
			if !hasStep {
				hi = v
			}
		}
		for v := lo; v <= hi; v += step {
			set |= 1 << uint(v)
		}
	}
	return set, star, nil
}

// value lit une valeur numérique ou un nom (JAN, MON) et vérifie ses bornes.
func value(s string, f field) (int, *Error) {
	if v, ok := f.names[strings.ToUpper(s)]; ok {
		return v, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		hint := ""
		if f.names != nil {
			hint = " ou un nom anglais à trois lettres"
		}
		return 0, &Error{Message: fmt.Sprintf("« %s » n'est pas une valeur du champ %s : il attend un nombre de %d à %d%s.", s, f.name, f.min, f.max, hint)}
	}
	if v < f.min || v > f.max {
		return 0, &Error{Message: fmt.Sprintf("La valeur %d du champ %s est hors limites : il attend un nombre de %d à %d.", v, f.name, f.min, f.max)}
	}
	return v, nil
}

// maxSearch borne la recherche de la prochaine échéance (une expression comme le 31 février
// ne se produit jamais).
const maxSearch = 5 * 366 * 24 * time.Hour

// Next renvoie la première échéance strictement postérieure à after, dans le fuseau loc
// (spec.timeZone, UTC par défaut), ou l'instant zéro s'il n'y en a pas dans les 5 ans. Une
// heure qui n'existe pas (passage à l'heure d'été) est sautée, comme le fait le contrôleur.
func (s *Schedule) Next(after time.Time, loc *time.Location) time.Time {
	if s.any {
		return time.Time{}
	}
	if loc == nil {
		loc = time.UTC
	}
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(maxSearch)
	for t.Before(limit) {
		switch {
		case s.month&(1<<uint(t.Month())) == 0:
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
		case !s.dayMatches(t):
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
		case s.hour&(1<<uint(t.Hour())) == 0:
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
		case s.minute&(1<<uint(t.Minute())) == 0:
			t = t.Add(time.Minute)
		default:
			return t
		}
	}
	return time.Time{}
}

// dayMatches applique la règle de cron : si les deux champs de jour sont restreints, l'un ou
// l'autre suffit ; sinon les deux doivent correspondre.
func (s *Schedule) dayMatches(t time.Time) bool {
	dom := s.dom&(1<<uint(t.Day())) != 0
	dow := s.dow&(1<<uint(t.Weekday())) != 0
	if s.domStar || s.dowStar {
		return dom && dow
	}
	return dom || dow
}
