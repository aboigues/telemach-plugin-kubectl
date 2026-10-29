package render

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/pitfall"
)

// Libellés du diagnostic d'un objet (feature 002).
const (
	LabelOtherIssues  = "Autres points relevés"
	LabelOtherObjects = "Autres objets en difficulté"
	LabelTest         = "▶ Pour tester"
	LabelNewPod       = "── Pod en difficulté de la nouvelle version ──"
	LabelConsumerPod  = "── Pod qui utilise ce PVC ──"
)

// stepLabels nomme chaque maillon dans la liste affichée ; les termes Kubernetes restent tels
// quels.
var stepLabels = map[pitfall.Step]string{
	pitfall.StepType:         "type",
	pitfall.StepSelector:     "selector",
	pitfall.StepReadiness:    "pods prêts",
	pitfall.StepEndpoints:    "endpoints",
	pitfall.StepPorts:        "ports",
	pitfall.StepRollout:      "rollout",
	pitfall.StepImage:        "image",
	pitfall.StepConfig:       "configuration",
	pitfall.StepClass:        "classe",
	pitfall.StepBackend:      "backends",
	pitfall.StepAddress:      "adresse",
	pitfall.StepStorageClass: "classe de stockage",
	pitfall.StepBinding:      "liaison",
	pitfall.StepConsumer:     "pod consommateur",
	// Feature 004.
	pitfall.StepAdmission:  "admission",
	pitfall.StepStatus:     "état",
	pitfall.StepPods:       "pods",
	pitfall.StepSchedule:   "planification",
	pitfall.StepHistory:    "historique",
	pitfall.StepLastJob:    "dernier Job",
	pitfall.StepNodes:      "nœuds",
	pitfall.StepService:    "Service headless",
	pitfall.StepOrdinal:    "ordre des pods",
	pitfall.StepVolumes:    "volumes",
	pitfall.StepMetricsAPI: "API de métriques",
	pitfall.StepTarget:     "cible",
	pitfall.StepRequests:   "requests",
	pitfall.StepConditions: "conditions",
}

// kindLabels nomme le type d'objet dans l'en-tête.
var kindLabels = map[pitfall.Kind]string{
	pitfall.KindService:    "Service",
	pitfall.KindDeployment: "Deployment",
	pitfall.KindIngress:    "Ingress",
	pitfall.KindPVC:        "PVC",
	// Feature 004.
	pitfall.KindJob:         "Job",
	pitfall.KindCronJob:     "CronJob",
	pitfall.KindDaemonSet:   "DaemonSet",
	pitfall.KindStatefulSet: "StatefulSet",
	pitfall.KindHPA:         "HPA",
}

// RenderObject affiche le diagnostic d'un objet (FR-021) : résumé du premier piège, identité de
// l'objet, maillons vérifiés, leçon (concept, cause, corrections, anti-pattern), autres points
// relevés, lectures refusées, liste courte des autres objets, puis le renvoi à deepen.
func RenderObject(w io.Writer, snap pitfall.ObjectSnapshot, cat *catalog.Catalog, o Options) error {
	t := o.Theme
	width := max(o.Width, 40)
	var b strings.Builder
	primary := snap.Primary()
	var entry *catalog.Entry
	captures := snap.LessonCaptures(pitfall.Pitfall{})
	if primary != nil && cat != nil {
		entry, _ = pitfall.LessonEntry(cat, *primary)
		captures = snap.LessonCaptures(*primary)
	}
	if captures["kubectl"] == "" {
		captures["kubectl"] = "kubectl"
	}

	switch {
	case entry == nil && snap.Failing != nil:
		// Aucun piège sur l'objet lui-même : la leçon est celle du pod en difficulté (PVC qui attend
		// un pod qui ne démarre pas, research.md S7).
		b.WriteString(t.Title(SymbolError+" "+kindLabels[snap.Kind]+" « "+snap.Name+" » attend le pod « "+snap.Failing.Pod.Name+" », qui ne démarre pas ("+snap.Failing.Pod.Reason+").") + "\n")
	case entry == nil:
		b.WriteString(t.Success(SymbolSuccess+" "+kindLabels[snap.Kind]+" « "+snap.Name+" » : aucun problème relevé.") + "\n")
	case primary.Severity == pitfall.Warning:
		b.WriteString(t.Title(SymbolError+" "+catalog.Render(entry.Summary, captures)) + "\n")
	case primary.Severity == pitfall.Info:
		b.WriteString(t.Hint(SymbolInfo+" "+catalog.Render(entry.Summary, captures)) + "\n")
	default:
		b.WriteString(t.Success(SymbolSuccess+" "+catalog.Render(entry.Summary, captures)) + "\n")
	}
	b.WriteString(indent + t.Dim(objectIdentity(snap)) + "\n")

	if len(snap.Links) > 0 {
		b.WriteString("\n")
		for _, l := range snap.Links {
			b.WriteString(linkLine(l, t, width) + "\n")
		}
	}

	if entry != nil {
		b.WriteString("\n")
		if primary.Severity == pitfall.OK {
			writeConfirmation(&b, entry, captures, o, width)
		} else {
			writeLesson(&b, entry, captures, o, width)
		}
	}

	if f := snap.Failing; f != nil {
		label := LabelNewPod
		if snap.Kind == pitfall.KindPVC {
			label = LabelConsumerPod
		}
		b.WriteString("\n" + t.Label(label) + "\n")
		d := diagnose.Diagnosis{Origin: diagnose.Origin(f.Origin), Captures: f.Captures}
		if cat != nil && f.Entry != "" {
			d.Entry, _ = cat.Get(f.Entry)
		}
		var pod strings.Builder
		if err := RenderPod(&pod, f.Pod, d, o); err != nil {
			return err
		}
		// L'invitation à approfondir du pod est remplacée par celle de l'objet, en fin de rendu,
		// sauf quand la leçon du pod est la seule.
		text := strings.TrimRight(pod.String(), "\n")
		if i := strings.LastIndex(text, "\n"); entry != nil && i >= 0 && strings.Contains(text[i:], "telemach deepen") {
			text = strings.TrimRight(text[:i], "\n")
		}
		b.WriteString(text + "\n")
	}

	if others := snap.Secondary(); len(others) > 0 && cat != nil {
		b.WriteString("\n" + t.Label(LabelOtherIssues) + "\n")
		for _, p := range others {
			e, ok := pitfall.LessonEntry(cat, p)
			if !ok {
				continue
			}
			symbol := SymbolWarning
			if p.Severity != pitfall.Warning {
				symbol = SymbolInfo
			}
			text := symbol + " " + catalog.Render(e.Summary, snap.LessonCaptures(p))
			b.WriteString(wrap(text, width, indent) + "\n")
		}
	}

	if snap.Partial() {
		b.WriteString("\n" + t.Cause(LabelPartial) + "\n")
		b.WriteString(wrap("Lecture refusée par RBAC : "+strings.Join(snap.DeniedReads, ", ")+
			". Le diagnostic repose sur les seules informations lisibles ; les maillons marqués « ? » n'ont pas pu être vérifiés.", width, indent) + "\n")
	}

	if len(snap.Others) > 0 {
		b.WriteString("\n")
		writeOthers(&b, snap, cat, captures, t)
	}

	if entry != nil {
		b.WriteString("\n" + t.Hint(LabelDeepen+" : "+telemach(captures)+" deepen") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// RenderObjectSummaries affiche la liste courte des objets en difficulté de diagnose sans
// argument (FR-015), avec la commande à copier pour chacun.
func RenderObjectSummaries(w io.Writer, list []pitfall.ObjectSummary, cat *catalog.Catalog, captures map[string]string, o Options) error {
	if len(list) == 0 {
		return nil
	}
	snap := pitfall.ObjectSnapshot{Others: list, Captures: captures}
	var b strings.Builder
	writeOthers(&b, snap, cat, captures, o.Theme)
	_, err := io.WriteString(w, "\n"+b.String())
	return err
}

// writeConfirmation écrit la leçon d'un objet sain : concept, puis les commandes pour le tester.
func writeConfirmation(b *strings.Builder, e *catalog.Entry, captures map[string]string, o Options, width int) {
	t := o.Theme
	b.WriteString(t.Concept(LabelConcept+" — "+e.Concept.Name) + "\n")
	b.WriteString(wrap(e.Concept.Definition, width, indent) + "\n\n")
	b.WriteString(wrap(catalog.Render(e.Cause, captures), width, indent) + "\n\n")
	b.WriteString(t.Fix(LabelTest) + "\n")
	for i, f := range e.Fixes {
		prefix := fmt.Sprintf("%s%d. ", indent, i+1)
		pad := strings.Repeat(" ", utf8.RuneCountInString(prefix))
		b.WriteString(prefix + wrapRest(catalog.Render(f.Description, captures), width, pad) + "\n")
		if f.Command != "" {
			b.WriteString(pad + t.Highlight(catalog.Render(f.Command, captures), "bash") + "\n")
		}
	}
}

// objectIdentity résume l'objet : type, nom, namespace, contexte, puis ses particularités.
func objectIdentity(s pitfall.ObjectSnapshot) string {
	parts := []string{strings.ToLower(kindLabels[s.Kind]) + " " + s.Name}
	if s.Namespace != "" {
		parts = append(parts, "namespace "+s.Namespace)
	}
	if s.Context != "" {
		parts = append(parts, "contexte "+s.Context)
	}
	for _, f := range s.Facts {
		if f.Label == "" {
			parts = append(parts, f.Value)
		} else {
			parts = append(parts, f.Label+" "+f.Value)
		}
	}
	return strings.Join(parts, " · ")
}

// linkLine écrit un maillon : symbole d'état, nom aligné, observations. Les observations qui
// ne tiennent pas sur la ligne passent chacune sur une ligne alignée.
func linkLine(l pitfall.Link, t Theme, width int) string {
	symbol, detail := "·", "non vérifié"
	switch l.State {
	case pitfall.StateOK:
		symbol, detail = SymbolSuccess, ""
	case pitfall.StateBroken:
		symbol, detail = SymbolError, ""
	case pitfall.StateUnknown:
		symbol, detail = "?", "non lu"
	}
	var obs []string
	for _, o := range l.Observed {
		if o.Label == "" {
			obs = append(obs, o.Value)
		} else {
			obs = append(obs, o.Label+" : "+o.Value)
		}
	}
	label := stepLabels[l.Step]
	if label == "" {
		label = string(l.Step)
	}
	head := indent + symbol + " " + label + strings.Repeat(" ", max(1, linkColumn-utf8.RuneCountInString(label)))
	pad := strings.Repeat(" ", utf8.RuneCountInString(head))
	if len(obs) > 0 {
		detail = strings.Join(obs, " · ")
		if utf8.RuneCountInString(head+detail) > width {
			detail = strings.Join(obs, "\n"+pad)
		}
	}
	var lines []string
	for _, line := range strings.Split(head+detail, "\n") {
		switch l.State {
		case pitfall.StateBroken:
			line = t.Title(line)
		case pitfall.StateOK:
		default:
			line = t.Dim(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// linkColumn est la largeur de la colonne des noms de maillons.
const linkColumn = 19

// writeOthers écrit la liste des autres objets en difficulté.
func writeOthers(b *strings.Builder, snap pitfall.ObjectSnapshot, cat *catalog.Catalog, captures map[string]string, t Theme) {
	b.WriteString(t.Label(LabelOtherObjects) + "\n")
	kubectl := captures["kubectl"]
	if kubectl == "" {
		kubectl = "kubectl"
	}
	for _, other := range snap.Others {
		summary := other.Pitfall
		if cat != nil {
			if e, ok := cat.ByPitfall(other.Pitfall); ok {
				summary = catalog.Render(e.Summary, mergeName(captures, other))
			}
		}
		cmd := kubectl + " telemach diagnose " + string(other.Kind) + "/" + other.Name
		if scope := captures["scope"]; scope != "" {
			cmd += " " + scope
		}
		fmt.Fprintf(b, "%s• %s/%s — %s\n%s  %s\n", indent, other.Kind, other.Name, summary, indent, t.Highlight(cmd, "bash"))
	}
}

// mergeName complète les valeurs communes avec le nom d'un autre objet de la liste courte.
func mergeName(base map[string]string, other pitfall.ObjectSummary) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	out[string(other.Kind)] = other.Name
	out["name"] = other.Name
	for k, v := range other.Captures {
		out[k] = v
	}
	return out
}
