package workload

import (
	"regexp"
	"strconv"
	"strings"
)

// Causes de placement d'un pod Pending (feature 004, research.md F6, data-model.md
// § PlacementCause) : le message du scheduler agrège une cause par groupe de nœuds.

// Raisons ajoutées par la feature 004.
const (
	ReasonUntoleratedTaint  = "UntoleratedTaint"
	ReasonPodAntiAffinity   = "PodAntiAffinity"
	ReasonPodAffinity       = "PodAffinity"
	ReasonNodeUnschedulable = "NodeUnschedulable"
	ReasonPlacementUnknown  = "Unknown" // cause citée telle quelle, sans explication
)

// PlacementCause est une cause élémentaire du message du scheduler.
type PlacementCause struct {
	Reason    string
	Nodes     int
	Detail    string   // taint (« dedie=gpu »), ressource (« cpu ») ou texte d'origine
	Normal    bool     // cause attendue : taint du plan de contrôle
	NodeNames []string // nœuds concernés, si la lecture des nœuds est permise
}

var (
	schedulerHead = regexp.MustCompile(`^0/(\d+) nodes are available: `)
	causeItem     = regexp.MustCompile(`^(\d+) (?:node\(s\) )?(.+)$`)
	taintDetail   = regexp.MustCompile(`^had untolerated taint(?:\(s\)| \{([^}]*)\})`)
)

// causeOrder donne la priorité des raisons à égalité de nœuds.
var causeOrder = map[string]int{
	ReasonUntoleratedTaint: 0, ReasonNodeUnschedulable: 1, ReasonPodAntiAffinity: 2, ReasonPodAffinity: 3,
	ReasonInsufficient: 4, ReasonNodeSelector: 5, ReasonUnboundPVC: 6, ReasonPlacementUnknown: 7,
}

// ParseScheduler découpe « 0/N nodes are available: <cause>, <cause>. preemption: … » ; il
// renvoie N et les causes dans l'ordre du message. Un message d'une autre forme donne 0 et nil.
func ParseScheduler(msg string) (int, []PlacementCause) {
	m := schedulerHead.FindStringSubmatch(msg)
	if m == nil {
		return 0, nil
	}
	total, _ := strconv.Atoi(m[1])
	body := msg[len(m[0]):]
	if i := strings.Index(body, " preemption:"); i >= 0 {
		body = body[:i]
	}
	body = strings.TrimSuffix(strings.TrimSpace(body), ".")
	var out []PlacementCause
	for _, item := range splitCauses(body) {
		cm := causeItem.FindStringSubmatch(strings.TrimSpace(item))
		if cm == nil {
			continue
		}
		n, _ := strconv.Atoi(cm[1])
		out = append(out, classifyCause(n, cm[2]))
	}
	return total, out
}

// splitCauses sépare les causes par « , », sans couper à l'intérieur d'accolades (taint).
func splitCauses(body string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range body {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, body[start:i])
				start = i + 1
			}
		}
	}
	return append(out, body[start:])
}

func classifyCause(n int, text string) PlacementCause {
	c := PlacementCause{Nodes: n}
	switch {
	case taintDetail.MatchString(text):
		c.Reason = ReasonUntoleratedTaint
		if t := taintDetail.FindStringSubmatch(text)[1]; t != "" {
			c.Detail = taintText(t)
		}
	case strings.HasPrefix(text, "were unschedulable"):
		c.Reason = ReasonNodeUnschedulable
	case strings.HasPrefix(text, "didn't match pod anti-affinity rules"):
		c.Reason = ReasonPodAntiAffinity
	case strings.HasPrefix(text, "didn't match pod affinity rules"):
		c.Reason = ReasonPodAffinity
	case strings.HasPrefix(text, "Insufficient "):
		c.Reason, c.Detail = ReasonInsufficient, strings.TrimPrefix(text, "Insufficient ")
	case nodeSelector.MatchString(text):
		c.Reason = ReasonNodeSelector
	case unboundPVC.MatchString(text):
		c.Reason = ReasonUnboundPVC
	default:
		c.Reason, c.Detail = ReasonPlacementUnknown, text
	}
	if c.Reason == ReasonUntoleratedTaint && strings.HasPrefix(c.Detail, controlPlaneTaint) {
		c.Normal = true
	}
	return c
}

// controlPlaneTaint est la clé du taint posé sur les nœuds du plan de contrôle.
const controlPlaneTaint = "node-role.kubernetes.io/control-plane"

// taintText réécrit « clé: valeur » (forme du scheduler) en « clé=valeur » (forme de kubectl).
func taintText(t string) string {
	k, v, ok := strings.Cut(t, ":")
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	if !ok || v == "" {
		return k
	}
	return k + "=" + v
}

// MainCause renvoie la cause qui concerne le plus de nœuds, hors cause normale ; à égalité,
// la raison la plus prioritaire. Sans cause anormale, la première cause.
func MainCause(causes []PlacementCause) PlacementCause {
	var best *PlacementCause
	for i := range causes {
		c := &causes[i]
		if c.Normal {
			continue
		}
		if best == nil || c.Nodes > best.Nodes || (c.Nodes == best.Nodes && causeOrder[c.Reason] < causeOrder[best.Reason]) {
			best = c
		}
	}
	if best == nil && len(causes) > 0 {
		return causes[0]
	}
	if best == nil {
		return PlacementCause{}
	}
	return *best
}
