package workload

import (
	"errors"
	"sort"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// NodeReader est la lecture facultative des nœuds (feature 004, research.md F6) : elle précise
// les causes de taint et de cordon (nom des nœuds, taint exact, plan de contrôle). Un lecteur
// qui ne la propose pas laisse les causes du message telles quelles.
type NodeReader interface {
	ListNodes() (kuberead.NodeList, error)
}

// refineNodes remplace les causes « taint » et « nœud non planifiable » du message par leur
// détail nœud par nœud, si la lecture des nœuds est permise et donne les mêmes totaux que le
// scheduler (sinon l'état a changé depuis : le message reste la référence).
func refineNodes(r Reader, p kuberead.Pod, snap *PodSnapshot) {
	nr, ok := r.(NodeReader)
	if !ok || !needsNodes(snap.Placement) {
		return
	}
	nodes, err := nr.ListNodes()
	var denied *kuberead.ErrDenied
	switch {
	case errors.As(err, &denied):
		snap.DeniedReads = append(snap.DeniedReads, "les nœuds (taints, cordon)")
		return
	case err != nil:
		return
	}
	type group struct {
		cause PlacementCause
		names []string
	}
	groups := map[string]*group{}
	var keys []string
	add := func(key string, c PlacementCause, name string) {
		g, ok := groups[key]
		if !ok {
			g = &group{cause: c}
			groups[key] = g
			keys = append(keys, key)
		}
		g.names = append(g.names, name)
	}
	for _, n := range nodes.Items {
		name := clean(n.Metadata.Name)
		if n.Spec.Unschedulable {
			add("unschedulable", PlacementCause{Reason: ReasonNodeUnschedulable}, name)
			continue
		}
		var taints []string
		normal := true
		for _, t := range n.Spec.Taints {
			if (t.Effect != "NoSchedule" && t.Effect != "NoExecute") || tolerated(t, p.Spec.Tolerations) {
				continue
			}
			taints = append(taints, clean(t.String()))
			normal = normal && t.Key == kuberead.ControlPlaneTaint
		}
		if len(taints) == 0 {
			continue
		}
		detail := strings.Join(taints, ", ")
		add("taint:"+detail, PlacementCause{Reason: ReasonUntoleratedTaint, Detail: detail, Normal: normal}, name)
	}
	var refined []PlacementCause
	counts := map[string]int{}
	for _, k := range keys {
		g := groups[k]
		c := g.cause
		c.Nodes = len(g.names)
		sort.Strings(g.names)
		c.NodeNames = g.names
		counts[c.Reason] += c.Nodes
		refined = append(refined, c)
	}
	for _, c := range snap.Placement {
		if c.Reason == ReasonUntoleratedTaint || c.Reason == ReasonNodeUnschedulable {
			counts[c.Reason] -= c.Nodes
		}
	}
	for _, v := range counts {
		if v != 0 {
			return // l'état des nœuds a changé depuis le message du scheduler
		}
	}
	for _, c := range snap.Placement {
		if c.Reason != ReasonUntoleratedTaint && c.Reason != ReasonNodeUnschedulable {
			refined = append(refined, c)
		}
	}
	snap.Placement = refined
	if main := MainCause(refined); main.Reason != "" && main.Reason != ReasonPlacementUnknown {
		snap.Reason = main.Reason
	}
}

func needsNodes(causes []PlacementCause) bool {
	for _, c := range causes {
		if c.Reason == ReasonUntoleratedTaint || c.Reason == ReasonNodeUnschedulable {
			return true
		}
	}
	return false
}

// tolerated indique si une des tolérances du pod accepte le taint.
func tolerated(t kuberead.Taint, tols []kuberead.Toleration) bool {
	for _, tol := range tols {
		if tol.Effect != "" && tol.Effect != t.Effect {
			continue
		}
		switch {
		case tol.Operator == "Exists" && (tol.Key == "" || tol.Key == t.Key):
			return true
		case (tol.Operator == "" || tol.Operator == "Equal") && tol.Key == t.Key && tol.Value == t.Value:
			return true
		}
	}
	return false
}
