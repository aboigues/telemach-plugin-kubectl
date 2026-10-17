package pitfall

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// data-model.md § Link : un maillon non lu (unknown) ne porte jamais de piège affirmé.
func TestUnknownLinkNeverBroken(t *testing.T) {
	var s ObjectSnapshot
	s.AddLink(StepSelector, StateOK)
	s.AddLink(StepEndpoints, LinkStateFor(&kuberead.ErrDenied{What: "les endpoints de web"}))
	if s.Links[1].State != StateUnknown || len(s.DeniedReads) != 0 {
		t.Fatalf("lecture refusée : maillon unknown attendu : %+v", s.Links)
	}
	s.Deny(&kuberead.ErrDenied{What: "les endpoints de web"})
	if !s.Partial() || s.DeniedReads[0] != "les endpoints de web" {
		t.Errorf("diagnostic partiel attendu : %+v", s.DeniedReads)
	}
	if s.Add(Pitfall{ID: "service.pods-not-ready", Step: StepEndpoints, Severity: Warning}) {
		t.Error("un piège ne peut pas être affirmé sur un maillon non lu")
	}
	if !s.Add(Pitfall{ID: "service.no-matching-pods", Step: StepSelector, Severity: Warning}) {
		t.Error("piège sur un maillon lu : accepté")
	}
	if s.Links[0].State != StateBroken {
		t.Error("un piège d'avertissement casse son maillon")
	}
}

// Le premier avertissement dans l'ordre des maillons porte la leçon ; une information ne passe
// qu'en l'absence d'avertissement.
func TestPrimary(t *testing.T) {
	var s ObjectSnapshot
	for _, st := range []Step{StepSelector, StepReadiness, StepPorts} {
		s.AddLink(st, StateOK)
	}
	s.Add(Pitfall{ID: "image.mutable-tag", Step: StepPorts, Severity: Info})
	if p := s.Primary(); p == nil || p.ID != "image.mutable-tag" || !s.Healthy() {
		t.Fatalf("information seule : %+v", p)
	}
	s.Add(Pitfall{ID: "service.targetport-name", Step: StepPorts, Severity: Warning})
	s.Add(Pitfall{ID: "service.pods-not-ready", Step: StepReadiness, Severity: Warning})
	if p := s.Primary(); p.ID != "service.pods-not-ready" || s.Healthy() {
		t.Errorf("maillon le plus en amont attendu : %+v", p)
	}
	if got := s.Secondary(); len(got) != 2 || got[0].ID != "service.targetport-name" {
		t.Errorf("autres problèmes : %+v", got)
	}
}

func TestLinkStateFor(t *testing.T) {
	if LinkStateFor(nil) != StateOK || LinkStateFor(errors.New("x")) != StateUnknown {
		t.Error("état d'une lecture")
	}
}

// Chaque piège connu du code doit avoir sa leçon dans le catalogue, une seule.
func TestCheckCatalog(t *testing.T) {
	entry := `
- id: service.no-matching-pods
  category: service
  pitfalls: [service.no-matching-pods]
  concept: {name: label selector, definition: Un Service trouve ses pods par leurs labels.}
  summary: Le Service ne désigne aucun pod.
  cause: Aucun pod du namespace ne porte les labels du selector.
  fixes: [{description: Comparer les labels des pods au selector., safe: true}]
  deepen: {explanation: Un Service suit des labels., example: kubectl get pods --show-labels, exercise: Crée un namespace d'exercice avec kubectl create namespace exo.}
  doc_url: https://kubernetes.io/docs/concepts/services-networking/service/
  tests: [selector-typo]
`
	cat, err := catalog.Load(fstest.MapFS{"x.yaml": {Data: []byte(entry)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := checkIDs(cat, []string{"service.no-matching-pods"}); err != nil {
		t.Errorf("piège couvert : %v", err)
	}
	if err := checkIDs(cat, []string{"service.no-matching-pods", "service.inconnu"}); err == nil || !strings.Contains(err.Error(), "service.inconnu") {
		t.Errorf("piège sans leçon : erreur attendue, obtenu %v", err)
	}
	def, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCatalog(def); err != nil {
		t.Errorf("catalogue embarqué :\n%v", err)
	}
}
