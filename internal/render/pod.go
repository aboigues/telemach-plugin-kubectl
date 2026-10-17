package render

import (
	"fmt"
	"io"
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
		b.WriteString(wrap("Lecture refusée par RBAC : "+strings.Join(snap.DeniedReads, ", ")+
			". Le diagnostic repose sur les seules informations lisibles ; « "+kubectl+" auth can-i list events » vérifie ce droit.", width, indent) + "\n")
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
