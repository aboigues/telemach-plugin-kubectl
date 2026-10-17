package pitfall

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// T023 : chemin du trafic d'un Service sur le corpus relevé sur kind (SC-001 ≥ 90 %, SC-002 :
// aucun piège sur un Service sain).
func TestServiceCorpus(t *testing.T) {
	cases := loadObjectCases(t, KindService)
	good := 0
	for _, c := range cases {
		f := &fixture{Dir: c.dir}
		snap, err := Diagnose(testContext(t, f, c.at), KindService, c.exp.Name)
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
		if len(c.exp.Closest) > 0 && snap.Primary() != nil {
			got := snap.Primary().Captures["closest"]
			for _, want := range c.exp.Closest {
				if !strings.Contains(got, want) {
					t.Errorf("%s : labels proches %q, attendu %q", c.name, got, want)
					ok = false
				}
			}
		}
		if c.exp.Primary == "service.healthy" && !snap.Healthy() {
			t.Errorf("%s : un Service sain ne doit porter aucun avertissement (SC-002) : %+v", c.name, snap.Pitfalls)
			ok = false
		}
		if c.exp.Also != nil {
			other, err := Diagnose(testContext(t, f, c.at), KindService, c.exp.Also.Name)
			if err != nil || other.Primary() == nil || other.Primary().ID != c.exp.Also.Primary {
				t.Errorf("%s : %s : %+v %v, attendu %s", c.name, c.exp.Also.Name, other.Primary(), err, c.exp.Also.Primary)
				ok = false
			}
		}
		if ok {
			good++
		}
	}
	rate := float64(good) / float64(len(cases))
	t.Logf("SC-001 (Services) : %d/%d cas correctement diagnostiqués (%.0f %%)", good, len(cases), rate*100)
	if rate < 0.9 {
		t.Errorf("SC-001 non atteint : %.0f %% < 90 %%", rate*100)
	}
}

// FR-020 : une lecture d'EndpointSlices refusée rend le maillon « non lu » et le diagnostic
// partiel ; aucune cause n'est affirmée sur ce maillon.
func TestServiceDeniedEndpoints(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/healthy", Denied: map[string]bool{"endpointslices.json": true}}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindService, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if linkStates(snap)["endpoints"] != "unknown" || !snap.Partial() {
		t.Errorf("maillon endpoints non lu et diagnostic partiel attendus : %+v %v", snap.Links, snap.DeniedReads)
	}
	for _, p := range snap.Pitfalls {
		if p.Step == StepEndpoints {
			t.Errorf("piège affirmé sur un maillon non lu : %+v", p)
		}
	}
}

// Les lectures faites sont celles de research.md S1, sans lecture de Secret.
func TestServiceReads(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/selector-typo"}
	if _, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindService, "web"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls {
		if !slices.Contains([]string{"service.json", "selected-pods.json", "pods.json", "endpointslices.json"}, c) {
			t.Errorf("lecture inattendue : %s", c)
		}
	}
}
