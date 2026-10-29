package pitfall

import (
	"sort"

	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Refus d'admission vu depuis un Deployment (feature 004, US1, research.md F4) : « kubectl
// scale » répond « scaled », mais le ReplicaSet se voit refuser ses pods (quota, LimitRange,
// PodSecurity). Le message du refus est celui de la commande refusée : il est expliqué par la
// même entrée du catalogue.

// DeploymentReplicaFailure est le piège du maillon admission ; sa leçon est celle du refus.
const DeploymentReplicaFailure = "deployment.replica-failure"

func init() {
	knownIDs = append(knownIDs, DeploymentReplicaFailure)
}

// admissionLink ajoute le maillon admission quand le Deployment signale ReplicaFailure, ou,
// sans cette condition, quand son ReplicaSet le plus récent a un événement FailedCreate. Sans
// refus, le maillon n'est pas affiché (les autres diagnostics de Deployment sont inchangés).
func admissionLink(ctx Context, s *ObjectSnapshot, dep kuberead.Deployment, desired int) {
	message := ""
	if c := findDeploymentCondition(dep, "ReplicaFailure"); c != nil && c.Status == "True" {
		message = c.Message
	} else if dep.Status.Replicas < desired {
		message = failedCreateEvent(ctx, s, dep)
	}
	if message == "" {
		return
	}
	message = clean(message)
	s.AddLink(StepAdmission, StateOK, Observation{Label: "refus", Value: "« " + message + " »"})
	p := Pitfall{ID: DeploymentReplicaFailure, Step: StepAdmission, Severity: Warning, Captures: map[string]string{"refusal": message}}
	d := diagnose.Diagnose(diagnose.Input{Command: command.Parse([]string{"scale"}), ExitCode: 1, Stderr: message}, ctx.Catalog)
	if d.Entry != nil {
		p.Lesson = d.Entry.ID
		for k, v := range d.Captures {
			if k != "kubectl" && k != "scope" && k != "sub" && k != "exit_code" {
				p.Captures[k] = v
			}
		}
	}
	s.Add(p)
}

func findDeploymentCondition(dep kuberead.Deployment, t string) *kuberead.DeploymentCondition {
	for i := range dep.Status.Conditions {
		if dep.Status.Conditions[i].Type == t {
			return &dep.Status.Conditions[i]
		}
	}
	return nil
}

// failedCreateEvent lit le dernier événement FailedCreate du ReplicaSet le plus récent.
func failedCreateEvent(ctx Context, s *ObjectSnapshot, dep kuberead.Deployment) string {
	sets, err := ctx.Reader.ReplicaSetsBySelector(selectorString(dep.Spec.Selector.MatchLabels))
	if err != nil {
		s.Deny(err)
		return ""
	}
	var owned []kuberead.ReplicaSet
	for _, rs := range sets.Items {
		if rs.OwnedBy(dep.Metadata.Name) {
			owned = append(owned, rs)
		}
	}
	newRS, _ := versions(owned)
	if newRS == nil {
		return ""
	}
	events, err := ctx.Reader.ReplicaSetEvents(newRS.Metadata.Name)
	if err != nil {
		s.Deny(err)
		return ""
	}
	sort.SliceStable(events.Items, func(i, j int) bool { return events.Items[i].LastTimestamp.After(events.Items[j].LastTimestamp) })
	for _, e := range events.Items {
		if e.Reason == "FailedCreate" {
			return e.Message
		}
	}
	return ""
}
