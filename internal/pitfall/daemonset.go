package pitfall

import (
	"sort"
	"strconv"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un DaemonSet (feature 004, research.md F8 ; catalogue : batch.yaml).
const (
	DaemonSetExcludedNodes = "daemonset.excluded-nodes"
	DaemonSetRolloutStuck  = "daemonset.rollout-stuck"
	DaemonSetOK            = "daemonset.ok"
)

func init() {
	knownIDs = append(knownIDs, DaemonSetExcludedNodes, DaemonSetRolloutStuck, DaemonSetOK)
	diagnosers[KindDaemonSet] = DiagnoseDaemonSet
}

// DiagnoseDaemonSet compare les pods voulus aux nœuds du cluster (nœuds exclus et leurs taints,
// plan de contrôle compris), puis l'état du rollout et le pod en difficulté le plus récent.
func DiagnoseDaemonSet(ctx Context, name string) (ObjectSnapshot, error) {
	ds, err := ctx.Reader.GetDaemonSet(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindDaemonSet, name)
	s.Captures["daemonset"] = name
	st := ds.Status
	desired := st.DesiredNumberScheduled
	caps := map[string]string{"desired": strconv.Itoa(desired), "ready": strconv.Itoa(st.NumberReady)}
	s.Facts = append(s.Facts, Observation{Value: strconv.Itoa(st.NumberReady) + "/" + strconv.Itoa(desired) + " prêt(s)"})

	nodes, nerr := ctx.Reader.ListNodes()
	s.Deny(nerr)
	if nerr != nil {
		s.AddLink(StepNodes, StateUnknown)
	} else {
		total := len(nodes.Items)
		caps["nodes"] = strconv.Itoa(total)
		s.AddLink(StepNodes, StateOK, Observation{Label: "pods voulus", Value: strconv.Itoa(desired) + " sur " + plural(total, "nœud")})
		if desired < total {
			excludedNodes(&s, ds.Spec.Template.Spec.Tolerations, nodes.Items, caps)
		}
	}

	sel := selectorString(ds.Spec.Selector.MatchLabels)
	pods, perr := ctx.Reader.PodsBySelector(sel)
	s.Deny(perr)
	s.AddLink(StepRollout, StateOK, Observation{Label: "à jour", Value: strconv.Itoa(st.UpdatedNumberScheduled) + "/" + strconv.Itoa(desired)})
	if perr == nil {
		s.Failing = firstFailing(ctx, pods.Items)
	}
	if f := s.Failing; f != nil {
		caps["pod"], caps["reason"] = f.Pod.Name, f.Pod.Reason
		s.Add(Pitfall{ID: DaemonSetRolloutStuck, Step: StepRollout, Severity: Warning, Captures: caps})
	} else if st.NumberReady < desired || st.UpdatedNumberScheduled < desired {
		s.Add(Pitfall{ID: DaemonSetRolloutStuck, Step: StepRollout, Severity: Info, Transient: true, Captures: caps})
	}
	if len(s.Pitfalls) == 0 {
		s.Add(Pitfall{ID: DaemonSetOK, Step: StepNodes, Severity: OK, Captures: caps})
	}
	return s, nil
}

// excludedNodes nomme les nœuds sans pod du DaemonSet et le taint qui les exclut. Le seul taint
// du plan de contrôle est une exclusion normale (information) ; un autre taint, un oubli de
// toleration (avertissement).
func excludedNodes(s *ObjectSnapshot, tols []kuberead.Toleration, nodes []kuberead.Node, caps map[string]string) {
	var names, taints []string
	normal := true
	for _, n := range nodes {
		var own []string
		for _, t := range n.Spec.Taints {
			if (t.Effect == "NoSchedule" || t.Effect == "NoExecute") && !tolerates(tols, t) {
				own = append(own, t.String())
				normal = normal && t.Key == kuberead.ControlPlaneTaint
			}
		}
		if len(own) > 0 {
			names = append(names, n.Metadata.Name)
			taints = append(taints, own...)
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	caps["excluded"], caps["taints"] = strings.Join(names, ", "), strings.Join(dedupe(taints), ", ")
	s.Observe(StepNodes, "exclus", caps["excluded"]+" ("+caps["taints"]+")")
	sev := Warning
	if normal {
		sev, caps["normal"] = Info, "oui"
	}
	s.Add(Pitfall{ID: DaemonSetExcludedNodes, Step: StepNodes, Severity: sev, Captures: caps})
}

// tolerates indique si une des tolérances accepte le taint.
func tolerates(tols []kuberead.Toleration, t kuberead.Taint) bool {
	for _, tol := range tols {
		if tol.Effect != "" && tol.Effect != t.Effect {
			continue
		}
		if (tol.Operator == "Exists" && (tol.Key == "" || tol.Key == t.Key)) ||
			((tol.Operator == "" || tol.Operator == "Equal") && tol.Key == t.Key && tol.Value == t.Value) {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
