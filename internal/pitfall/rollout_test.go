package pitfall

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// T031 : état du rollout sur le corpus relevé sur kind (research.md S5).
func TestDeploymentCorpus(t *testing.T) {
	cases := loadObjectCases(t, KindDeployment)
	good := 0
	for _, c := range cases {
		f := &fixture{Dir: c.dir}
		snap, err := Diagnose(testContext(t, f, c.at), KindDeployment, c.exp.Name)
		if err != nil {
			t.Errorf("%s : %v", c.name, err)
			continue
		}
		ok := true
		if p := snap.Primary(); p == nil || p.ID != c.exp.Primary {
			t.Errorf("%s : piège principal %+v, attendu %s", c.name, p, c.exp.Primary)
			ok = false
		}
		if snap.Rollout == nil || snap.Rollout.Phase != c.exp.Phase {
			t.Errorf("%s : phase %+v, attendu %s", c.name, snap.Rollout, c.exp.Phase)
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
	rate := float64(good) / float64(len(cases))
	t.Logf("SC-001 (Deployments) : %d/%d cas correctement diagnostiqués (%.0f %%)", good, len(cases), rate*100)
	if rate < 0.9 {
		t.Errorf("SC-001 non atteint : %.0f %% < 90 %%", rate*100)
	}
}

// Rollout bloqué : ancienne et nouvelle version nommées, pod en échec de la nouvelle version
// diagnostiqué avec les causes de la feature 001, retour arrière présenté comme un choix
// (FR-007, FR-008).
func TestRolloutStalledDetails(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/rollout-stalled"}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindDeployment, "stalled")
	if err != nil {
		t.Fatal(err)
	}
	r := snap.Rollout
	if r == nil || !strings.Contains(r.NewImage, "0.0.0-inexistant") || !strings.Contains(r.OldImage, "1.30-alpine") || r.OldReady != 1 {
		t.Fatalf("versions du rollout : %+v", r)
	}
	if f := snap.Failing; f == nil || f.Pod.Reason != "ImagePullBackOff" || f.Entry != "pod.image-pull" {
		t.Errorf("pod en échec de la nouvelle version : %+v", snap.Failing)
	}
	cat := testContext(t, f, timeOf(t, f.Dir)).Catalog
	e, _ := cat.ByPitfall(snap.Primary().ID)
	var undo bool
	for _, fix := range e.Fixes {
		undo = undo || strings.Contains(fix.Command, "rollout undo")
	}
	if !undo {
		t.Error("kubectl rollout undo doit être proposé (FR-008)")
	}
}

// T032 : référence d'image OCI (FR-009).
func TestImageRef(t *testing.T) {
	tests := []struct {
		ref     string
		mutable bool
	}{
		{"nginx", true},
		{"nginx:latest", true},
		{"nginxinc/nginx-unprivileged", true},
		{"registre:5000/app", true},
		{"registre:5000/app:1.2", false},
		{"nginx:1.30-alpine", false},
		{"nginx@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"nginx:latest@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
	}
	for _, tt := range tests {
		if got := ParseImage(tt.ref).Mutable(); got != tt.mutable {
			t.Errorf("ParseImage(%q).Mutable() = %v, attendu %v", tt.ref, got, tt.mutable)
		}
	}
	if r := ParseImage("registre:5000/équipe/app:2.0"); r.Tag != "2.0" || r.Name != "registre:5000/équipe/app" {
		t.Errorf("référence mal découpée : %+v", r)
	}
}

// T033 : configuration figée au démarrage (research.md S3) ; aucune valeur de ConfigMap ni de
// Secret dans l'instantané (SC-006).
func TestConfigDetails(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/config-env-stale"}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindDeployment, "conf")
	if err != nil {
		t.Fatal(err)
	}
	var stale, secret *Pitfall
	for i, p := range snap.Pitfalls {
		switch p.ID {
		case ConfigEnvStale:
			stale = &snap.Pitfalls[i]
		case ConfigSecretEnv:
			secret = &snap.Pitfalls[i]
		}
	}
	if stale == nil || stale.Captures["configmap"] != "conf-settings" || !strings.Contains(stale.Captures["refs"], "subPath") {
		t.Errorf("ConfigMap figée (env et subPath) attendue : %+v", stale)
	}
	if secret == nil || secret.Severity != Info || secret.Captures["secret"] != "conf-secret" {
		t.Errorf("information sur le Secret attendue, sans détection : %+v", secret)
	}
	if dump := goDump(snap); strings.Contains(dump, "prod") || strings.Contains(dump, "fake-token") {
		t.Errorf("valeur sensible dans l'instantané : %s", dump)
	}
	for _, c := range f.Calls {
		if strings.Contains(c, "secret") {
			t.Errorf("lecture d'un Secret : %s", c)
		}
	}
}
