package pitfall

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// Feature 004 (T038, T046, SC-003) : Jobs, CronJobs, DaemonSets, StatefulSets et HPA du corpus
// relevé sur kind : piège principal, état des maillons et pod en difficulté.
func TestWorkloadCorpus(t *testing.T) {
	for _, kind := range []Kind{KindJob, KindCronJob, KindDaemonSet, KindStatefulSet, KindHPA} {
		cases := loadObjectCases(t, kind)
		good := 0
		for _, c := range cases {
			f := &fixture{Dir: c.dir}
			snap, err := Diagnose(testContext(t, f, c.at), kind, c.exp.Name)
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
			reason := ""
			if snap.Failing != nil {
				reason = snap.Failing.Pod.Reason
			}
			if c.exp.Failing != "" && sameReason(reason) != sameReason(c.exp.Failing) {
				t.Errorf("%s : pod en difficulté %q, attendu %q", c.name, reason, c.exp.Failing)
				ok = false
			}
			if ok {
				good++
			}
		}
		rate := float64(good) / float64(len(cases))
		t.Logf("SC-003 (%s) : %d/%d cas correctement diagnostiqués (%.0f %%)", kind, good, len(cases), rate*100)
		if rate < 0.9 {
			t.Errorf("SC-003 non atteint pour %s : %.0f %% < 90 %%", kind, rate*100)
		}
	}
}

// Lecture refusée : le maillon devient inconnu, jamais cassé, et le diagnostic est partiel
// (FR-028).
func TestWorkloadDeniedReads(t *testing.T) {
	for _, tc := range []struct {
		kind  Kind
		cas   string
		name  string
		file  string
		step  Step
		state LinkState
	}{
		{KindDaemonSet, "daemonset-tainted-worker", "agent", "nodes.json", StepNodes, StateUnknown},
		{KindHPA, "hpa-missing-request", "web", "apiservice.json", StepMetricsAPI, StateUnknown},
		{KindCronJob, "cronjob-last-failed", "echec", "jobs.json", StepHistory, StateUnknown},
	} {
		f := &fixture{Dir: objectsCorpus + "/" + tc.cas, Denied: map[string]bool{tc.file: true}}
		snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), tc.kind, tc.name)
		if err != nil {
			t.Fatalf("%s : %v", tc.cas, err)
		}
		if got := linkStates(snap)[string(tc.step)]; got != string(tc.state) || !snap.Partial() {
			t.Errorf("%s : maillon %s %q, partiel %v", tc.cas, tc.step, got, snap.Partial())
		}
	}
}

// Feature 004 (T020, FR-005) : le refus vu depuis le Deployment reçoit la leçon du quota.
func TestReplicaFailureLesson(t *testing.T) {
	f := &fixture{Dir: objectsCorpus + "/replica-failure"}
	snap, err := Diagnose(testContext(t, f, timeOf(t, f.Dir)), KindDeployment, "api")
	if err != nil {
		t.Fatal(err)
	}
	p := snap.Primary()
	if p == nil || p.ID != DeploymentReplicaFailure || p.Lesson != "admission.quota-exceeded" ||
		p.Captures["quota"] != "quota-equipe" || p.Captures["resource"] != "limits.cpu" {
		t.Errorf("leçon du refus : %+v", p)
	}
	// Condition absente : l'événement FailedCreate du ReplicaSet est lu.
	f2 := &fixture{Dir: variant(t, "replica-failure", map[string]string{"deployment.json": withoutReplicaFailure(t)})}
	snap, err = Diagnose(testContext(t, f2, timeOf(t, f.Dir)), KindDeployment, "api")
	if err != nil {
		t.Fatal(err)
	}
	if p := snap.Primary(); p == nil || p.Lesson != "admission.quota-exceeded" {
		t.Errorf("refus lu dans l'événement FailedCreate : %+v", p)
	}
}

// withoutReplicaFailure renvoie le Deployment relevé sans sa condition ReplicaFailure.
func withoutReplicaFailure(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(objectsCorpus + "/replica-failure/deployment.json")
	if err != nil {
		t.Fatal(err)
	}
	var dep map[string]any
	if err := json.Unmarshal(data, &dep); err != nil {
		t.Fatal(err)
	}
	status := dep["status"].(map[string]any)
	var kept []any
	for _, c := range status["conditions"].([]any) {
		if c.(map[string]any)["type"] != "ReplicaFailure" {
			kept = append(kept, c)
		}
	}
	status["conditions"] = kept
	out, _ := json.Marshal(dep)
	return string(out)
}

// sameReason confond ErrImagePull et ImagePullBackOff, qui alternent selon le moment du relevé
// et donnent la même leçon (pod.image-pull).
func sameReason(r string) string {
	if r == "ErrImagePull" {
		return "ImagePullBackOff"
	}
	return r
}
