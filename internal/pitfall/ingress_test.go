package pitfall

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
)

// T040 : classe, backends et adresse d'un Ingress sur le corpus relevé sur kind (SC-001).
func TestIngressCorpus(t *testing.T) {
	cases := loadObjectCases(t, KindIngress)
	good := 0
	for _, c := range cases {
		snap, err := Diagnose(testContext(t, &fixture{Dir: c.dir}, c.at), KindIngress, c.exp.Name)
		if err != nil {
			t.Errorf("%s : %v", c.name, err)
			continue
		}
		ok := true
		if p := snap.Primary(); p == nil || p.ID != c.exp.Primary {
			t.Errorf("%s : piège principal %+v, attendu %s", c.name, p, c.exp.Primary)
			ok = false
		}
		if diff := cmp.Diff(c.exp.Links, linkStates(snap)); diff != "" {
			t.Errorf("%s : maillons (-attendu +obtenu) :\n%s", c.name, diff)
			ok = false
		}
		if ok {
			good++
		}
	}
	t.Logf("SC-001 (Ingress) : %d/%d cas correctement diagnostiqués", good, len(cases))
	if float64(good) < 0.9*float64(len(cases)) {
		t.Errorf("SC-001 non atteint : %d/%d", good, len(cases))
	}
}

// FR-012 : sans aucune classe, la leçon dit qu'aucun contrôleur n'est installé sans en nommer
// un (le choix dépend du cluster).
func TestIngressNoControllerNamesNone(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/ingress-no-class"}
	ctx := testContext(t, f, timeOf(t, f.Dir))
	snap, err := Diagnose(ctx, KindIngress, "web")
	if err != nil {
		t.Fatal(err)
	}
	p := snap.Primary()
	if p == nil || p.ID != IngressNoController {
		t.Fatalf("piège principal %+v, attendu %s", p, IngressNoController)
	}
	text := strings.ToLower(lessonText(t, ctx.Catalog, snap, *p))
	for _, name := range []string{"nginx", "traefik", "haproxy", "contour", "kong", "istio", "envoy"} {
		if strings.Contains(text, name) {
			t.Errorf("la leçon nomme un contrôleur (%s) :\n%s", name, text)
		}
	}
}

// La classe par défaut est retenue pour un Ingress sans classe ; l'ancienne annotation est
// lue mais signalée comme dépréciée.
func TestIngressEffectiveClass(t *testing.T) {
	defaultClass := `{"items":[{"metadata":{"name":"demo","annotations":{"ingressclass.kubernetes.io/is-default-class":"true"}},"spec":{"controller":"example.com/telemach-demo"}}]}`
	dir := variant(t, "ingress-no-class", map[string]string{"ingressclasses.json": defaultClass})
	snap, err := Diagnose(testContext(t, &fixture{Dir: dir}, timeOf(t, dir)), KindIngress, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got := observed(snap, StepClass); !strings.Contains(got, "classe par défaut") || !strings.Contains(got, "demo") {
		t.Errorf("classe par défaut attendue : %q", got)
	}
	if linkStates(snap)["class"] != "ok" {
		t.Errorf("maillon class : %+v", snap.Links)
	}

	annotated := `{"metadata":{"name":"web","annotations":{"kubernetes.io/ingress.class":"demo"}},"spec":{"rules":[{"http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"hello","port":{"number":80}}}}]}}]},"status":{"loadBalancer":{}}}`
	dir = variant(t, "ingress-backend-missing", map[string]string{"ingress.json": annotated})
	snap, err = Diagnose(testContext(t, &fixture{Dir: dir}, timeOf(t, dir)), KindIngress, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got := observed(snap, StepClass); !strings.Contains(got, "dépréciée") || !strings.Contains(got, "demo") {
		t.Errorf("annotation dépréciée non signalée : %q", got)
	}
}

// Au plus 10 Services sont lus, quel que soit le nombre de règles (research.md S6).
func TestIngressBackendBudget(t *testing.T) {
	var paths []string
	for i := range 15 {
		paths = append(paths, fmt.Sprintf(`{"path":"/p%d","pathType":"Prefix","backend":{"service":{"name":"svc%d","port":{"number":80}}}}`, i, i))
	}
	ing := `{"metadata":{"name":"many"},"spec":{"ingressClassName":"demo","rules":[{"http":{"paths":[` + strings.Join(paths, ",") + `]}}]},"status":{"loadBalancer":{}}}`
	f := &fixture{Dir: variant(t, "ingress-backend-missing", map[string]string{"ingress.json": ing})}
	if _, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindIngress, "many"); err != nil {
		t.Fatal(err)
	}
	reads := 0
	for _, c := range f.Calls {
		if strings.HasPrefix(c, "service") {
			reads++
		}
	}
	if reads > maxBackends {
		t.Errorf("%d Services lus, au plus %d attendus", reads, maxBackends)
	}
}

// FR-020 : IngressClasses non lisibles → maillon unknown, diagnostic partiel, aucune cause
// affirmée sur la classe.
func TestIngressDeniedClasses(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/ingress-unknown-class", Denied: map[string]bool{"ingressclasses.json": true}}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindIngress, "typo")
	if err != nil {
		t.Fatal(err)
	}
	if linkStates(snap)["class"] != "unknown" || !snap.Partial() {
		t.Errorf("maillon class non lu et diagnostic partiel attendus : %+v %v", snap.Links, snap.DeniedReads)
	}
	for _, p := range snap.Pitfalls {
		if p.Step == StepClass {
			t.Errorf("piège affirmé sur un maillon non lu : %+v", p)
		}
	}
}

// Les lectures faites sont celles de research.md S6, sans lecture de Secret.
func TestIngressReads(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/ingress-backend-missing"}
	if _, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindIngress, "shop"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls {
		if c != "ingress.json" && c != "ingressclasses.json" && !strings.HasPrefix(c, "service-") {
			t.Errorf("lecture inattendue : %s", c)
		}
	}
}

// observed joint les observations d'un maillon.
func observed(s ObjectSnapshot, step Step) string {
	for _, l := range s.Links {
		if l.Step == step {
			var parts []string
			for _, o := range l.Observed {
				parts = append(parts, strings.TrimSpace(o.Label+" "+o.Value))
			}
			return strings.Join(parts, " ; ")
		}
	}
	return ""
}

// lessonText rend le résumé, la cause et les corrections de la leçon d'un piège.
func lessonText(t *testing.T, cat *catalog.Catalog, s ObjectSnapshot, p Pitfall) string {
	t.Helper()
	e, ok := cat.ByPitfall(p.ID)
	if !ok {
		t.Fatalf("%s sans leçon", p.ID)
	}
	caps := s.LessonCaptures(p)
	parts := []string{catalog.Render(e.Summary, caps), catalog.Render(e.Cause, caps)}
	for _, f := range e.Fixes {
		parts = append(parts, catalog.Render(f.Description, caps), catalog.Render(f.Command, caps))
	}
	return strings.Join(parts, "\n")
}
