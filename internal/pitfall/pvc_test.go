package pitfall

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// T041 : classe de stockage, liaison et pod consommateur d'un PVC sur le corpus relevé sur
// kind (SC-001).
func TestPVCCorpus(t *testing.T) {
	cases := loadObjectCases(t, KindPVC)
	good := 0
	for _, c := range cases {
		snap, err := Diagnose(testContext(t, &fixture{Dir: c.dir}, c.at), KindPVC, c.exp.Name)
		if err != nil {
			t.Errorf("%s : %v", c.name, err)
			continue
		}
		ok := true
		switch p := snap.Primary(); {
		case c.exp.Primary == "" && p != nil:
			t.Errorf("%s : aucun piège attendu sur le PVC : %+v", c.name, p)
			ok = false
		case c.exp.Primary != "" && (p == nil || p.ID != c.exp.Primary):
			t.Errorf("%s : piège principal %+v, attendu %s", c.name, p, c.exp.Primary)
			ok = false
		}
		// Research.md S7 : un pod consommateur en attente est diagnostiqué comme dans la feature 001.
		if f := snap.Failing; (c.exp.Failing == "") != (f == nil) || (f != nil && f.Pod.Reason != c.exp.Failing) {
			t.Errorf("%s : pod consommateur en difficulté %+v, attendu %q", c.name, f, c.exp.Failing)
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
	t.Logf("SC-001 (PVC) : %d/%d cas correctement diagnostiqués", good, len(cases))
	if float64(good) < 0.9*float64(len(cases)) {
		t.Errorf("SC-001 non atteint : %d/%d", good, len(cases))
	}
}

// FR-014 : un PVC Pending en WaitForFirstConsumer sans pod est un état normal : information
// transitoire, aucun avertissement, jamais présenté comme une erreur.
func TestPVCWaitingIsNormal(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/pvc-waiting"}
	ctx := testContext(t, f, timeOf(t, f.Dir))
	snap, err := Diagnose(ctx, KindPVC, "data")
	if err != nil {
		t.Fatal(err)
	}
	p := snap.Primary()
	if p == nil || p.ID != PVCWaitingFirst || p.Severity != Info || !p.Transient {
		t.Fatalf("information transitoire %s attendue : %+v", PVCWaitingFirst, p)
	}
	if !snap.Healthy() {
		t.Errorf("aucun avertissement attendu : %+v", snap.Pitfalls)
	}
	text := strings.ToLower(lessonText(t, ctx.Catalog, snap, *p))
	if !strings.Contains(text, "normal") || strings.Contains(text, "erreur") {
		t.Errorf("la leçon doit présenter l'attente comme normale :\n%s", text)
	}
}

// Sans storageClassName, la classe par défaut est retenue ; la chaîne vide est expliquée.
func TestPVCEffectiveClass(t *testing.T) {
	noClass := `{"metadata":{"name":"data"},"spec":{"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"16Mi"}}},"status":{"phase":"Pending"}}`
	dir := variant(t, "pvc-waiting", map[string]string{"pvc.json": noClass})
	snap, err := Diagnose(testContext(t, &fixture{Dir: dir}, timeOf(t, dir)), KindPVC, "data")
	if err != nil {
		t.Fatal(err)
	}
	if got := observed(snap, StepStorageClass); !strings.Contains(got, "classe par défaut") || !strings.Contains(got, "standard") {
		t.Errorf("classe par défaut attendue : %q", got)
	}

	f := &fixture{Dir: objectsCorpus + "/pvc-empty-class"}
	snap, err = Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindPVC, "static-data")
	if err != nil {
		t.Fatal(err)
	}
	if got := observed(snap, StepStorageClass); !strings.Contains(got, "provisionnement dynamique désactivé") {
		t.Errorf("classe vide non expliquée : %q", got)
	}
}

// FR-020 : StorageClasses non lisibles → maillon unknown, diagnostic partiel, aucune cause
// affirmée sur la classe.
func TestPVCDeniedClasses(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/pvc-unknown-class", Denied: map[string]bool{"storageclasses.json": true}}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindPVC, "fast-data")
	if err != nil {
		t.Fatal(err)
	}
	if linkStates(snap)["storageclass"] != "unknown" || !snap.Partial() {
		t.Errorf("maillon storageclass non lu et diagnostic partiel attendus : %+v %v", snap.Links, snap.DeniedReads)
	}
	if !snap.Healthy() {
		t.Errorf("aucun avertissement attendu sur un maillon non lu : %+v", snap.Pitfalls)
	}
}

// Les lectures faites sont celles de research.md S7, sans PersistentVolume ni Secret.
func TestPVCReads(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/pvc-unknown-class"}
	if _, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindPVC, "fast-data"); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Calls {
		if c != "pvc.json" && c != "storageclasses.json" && c != "pods.json" {
			t.Errorf("lecture inattendue : %s", c)
		}
	}
}
