package render

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// RenderPod affiche le diagnostic d'un pod (FR-022 à FR-026) : identité du pod, explication
// (résumé, concept, cause, corrections), fin du journal, lectures refusées, puis la liste
// courte des autres pods en difficulté.
func RenderPod(w io.Writer, snap workload.PodSnapshot, d diagnose.Diagnosis, o Options) error {
	t := o.Theme
	width := max(o.Width, 40)
	kubectl := kubectlName(d.Captures)
	var b strings.Builder
	if snap.State != workload.StateFailing {
		b.WriteString(t.Success(SymbolSuccess+" Le pod « "+snap.Name+" » n'est pas en difficulté") + t.Dim(" ("+podState(snap)+")") + "\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	if err := RenderExplanation(&b, d, "", o); err != nil {
		return err
	}
	text := strings.TrimRight(b.String(), "\n")
	// L'invitation à approfondir est replacée à la toute fin.
	deepen := ""
	if i := strings.LastIndex(text, "\n"); i >= 0 && strings.Contains(text[i:], "telemach deepen") {
		deepen, text = text[i+1:], strings.TrimRight(text[:i], "\n")
	}
	// L'identité du pod est insérée juste après la ligne de résumé.
	if first, rest, ok := strings.Cut(text, "\n"); ok {
		text = first + "\n" + indent + t.Dim(podIdentity(snap)) + "\n" + rest
	}
	b.Reset()
	b.WriteString(text + "\n")
	if len(snap.Placement) > 0 {
		b.WriteString("\n" + t.Label(fmt.Sprintf("%s (0/%d disponibles)", LabelPlacement, snap.PlacementTotal)) + "\n")
		for _, c := range snap.Placement {
			symbol := SymbolError
			if c.Normal {
				symbol = SymbolInfo
			}
			line := fmt.Sprintf("%s %-9s %s", symbol, plural(c.Nodes, "nœud"), placementLabel(c))
			if len(c.NodeNames) > 0 {
				line += " (" + strings.Join(c.NodeNames, ", ") + ")"
			}
			if c.Normal {
				b.WriteString(indent + t.Hint(line) + "\n")
			} else {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	if len(snap.LogTail) > 0 {
		label := LabelLog + " (conteneur " + snap.Container
		if snap.LogPrevious {
			label += ", exécution précédente"
		}
		b.WriteString("\n" + t.Label(label+")") + "\n")
		for _, l := range snap.LogTail {
			b.WriteString(indent + t.Dim(l) + "\n")
		}
	}
	if len(snap.DeniedReads) > 0 {
		b.WriteString("\n" + t.Cause(LabelPartial) + "\n")
		var checks []string
		for _, r := range snap.DeniedReads {
			res := "events"
			if strings.Contains(r, "nœuds") {
				res = "nodes" // feature 004 : lecture des nœuds pour les causes de placement
			}
			if c := "« " + kubectl + " auth can-i list " + res + " »"; !slices.Contains(checks, c) {
				checks = append(checks, c)
			}
		}
		verb := "vérifie ce droit"
		if len(checks) > 1 {
			verb = "vérifient ces droits"
		}
		b.WriteString(wrap("Lecture refusée par RBAC : "+strings.Join(snap.DeniedReads, ", ")+
			". Le diagnostic repose sur les seules informations lisibles ; "+strings.Join(checks, " et ")+" "+verb+".", width, indent) + "\n")
	}
	if len(snap.Others) > 0 {
		b.WriteString("\n" + t.Label(LabelOthers) + "\n")
		for _, other := range snap.Others {
			cmd := kubectl + " telemach diagnose pod/" + other.Name
			if scope := d.Captures["scope"]; scope != "" {
				cmd += " " + scope
			}
			fmt.Fprintf(&b, "%s• %s — %s\n%s  %s\n", indent, other.Name, other.Reason, indent, t.Highlight(cmd, "bash"))
		}
	}
	if deepen != "" {
		b.WriteString("\n" + deepen + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// placementLabel décrit une cause de placement en français (feature 004, research.md F6).
func placementLabel(c workload.PlacementCause) string {
	switch c.Reason {
	case workload.ReasonUntoleratedTaint:
		switch {
		case c.Normal:
			return "taint du plan de contrôle (normal)"
		case c.Detail != "":
			return "taint " + c.Detail + " non toléré"
		}
		return "taint non toléré"
	case workload.ReasonNodeUnschedulable:
		return "nœud non planifiable (cordon)"
	case workload.ReasonPodAntiAffinity:
		return "anti-affinité de pods non satisfaite"
	case workload.ReasonPodAffinity:
		return "affinité de pods non satisfaite"
	case workload.ReasonInsufficient:
		return "ressource insuffisante : " + c.Detail
	case workload.ReasonNodeSelector:
		return "nodeSelector ou affinité de nœud non satisfaits"
	case workload.ReasonUnboundPVC:
		return "PersistentVolumeClaim non lié"
	}
	return "« " + c.Detail + " »"
}

// podIdentity résume le pod : namespace, propriétaire, image, état, redémarrages.
func podIdentity(s workload.PodSnapshot) string {
	parts := []string{"pod " + s.Name}
	if s.Namespace != "" {
		parts = append(parts, "namespace "+s.Namespace)
	}
	if s.Context != "" {
		parts = append(parts, "contexte "+s.Context)
	}
	if s.Owner != "" {
		parts = append(parts, s.Owner)
	}
	if s.Image != "" {
		parts = append(parts, "image "+s.Image)
	}
	parts = append(parts, "état "+s.Reason)
	if s.Restarts > 0 {
		parts = append(parts, plural(s.Restarts, "redémarrage"))
	}
	return strings.Join(parts, " · ")
}

func podState(s workload.PodSnapshot) string {
	if s.State == workload.StateStarting {
		return "en cours de démarrage"
	}
	return "phase " + s.Phase
}
