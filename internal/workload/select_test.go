package workload

import (
	"reflect"
	"testing"
)

// Feature 004 (T030, research.md F6) : le message du scheduler est découpé cause par cause ;
// la partie « preemption: » est ignorée. Messages relevés sur kind 1.37
// (testdata/objects/pending-*) et en formation (issue #15).
func TestParseScheduler(t *testing.T) {
	for _, tc := range []struct {
		msg   string
		total int
		want  []PlacementCause
	}{
		{
			"0/3 nodes are available: 1 node(s) were unschedulable, 2 node(s) had untolerated taint(s). preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.",
			3,
			[]PlacementCause{{Reason: ReasonNodeUnschedulable, Nodes: 1}, {Reason: ReasonUntoleratedTaint, Nodes: 2}},
		},
		{
			"0/3 nodes are available: 1 node(s) had untolerated taint(s), 2 node(s) didn't match pod anti-affinity rules. preemption: 0/3 nodes are available: 1 Preemption is not helpful for scheduling, 2 No preemption victims found for incoming pod.",
			3,
			[]PlacementCause{{Reason: ReasonUntoleratedTaint, Nodes: 1}, {Reason: ReasonPodAntiAffinity, Nodes: 2}},
		},
		{
			"0/3 nodes are available: 1 node(s) had untolerated taint {dedie: gpu}, 2 Insufficient cpu.",
			3,
			[]PlacementCause{{Reason: ReasonUntoleratedTaint, Nodes: 1, Detail: "dedie=gpu"}, {Reason: ReasonInsufficient, Nodes: 2, Detail: "cpu"}},
		},
		{
			"0/2 nodes are available: 1 node(s) didn't match Pod's node affinity/selector, 1 node(s) had volume node affinity conflict.",
			2,
			[]PlacementCause{{Reason: ReasonNodeSelector, Nodes: 1}, {Reason: ReasonPlacementUnknown, Nodes: 1, Detail: "had volume node affinity conflict"}},
		},
	} {
		total, got := ParseScheduler(tc.msg)
		if total != tc.total || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q :\n%d %+v\nattendu %d %+v", tc.msg, total, got, tc.total, tc.want)
		}
	}
}

// La cause principale est celle du plus grand nombre de nœuds, hors cause normale (taint du
// plan de contrôle) ; à égalité, l'ordre de priorité des raisons.
func TestMainCause(t *testing.T) {
	causes := []PlacementCause{
		{Reason: ReasonNodeUnschedulable, Nodes: 1},
		{Reason: ReasonUntoleratedTaint, Nodes: 1, Detail: "node-role.kubernetes.io/control-plane", Normal: true},
		{Reason: ReasonUntoleratedTaint, Nodes: 1, Detail: "dedie=gpu"},
	}
	if got := MainCause(causes); got.Reason != ReasonUntoleratedTaint || got.Detail != "dedie=gpu" {
		t.Errorf("cause principale : %+v", got)
	}
	if got := MainCause([]PlacementCause{{Reason: ReasonPodAntiAffinity, Nodes: 2}, {Reason: ReasonUntoleratedTaint, Nodes: 1}}); got.Reason != ReasonPodAntiAffinity {
		t.Errorf("cause principale : %+v", got)
	}
}
