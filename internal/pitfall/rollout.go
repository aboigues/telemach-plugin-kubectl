package pitfall

import (
	"strconv"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// Pièges d'un Deployment (catalogue : internal/catalog/data/rollout.yaml).
const (
	RolloutStalled     = "rollout.stalled"
	RolloutProgressing = "rollout.progressing"
	RolloutPaused      = "rollout.paused"
	RolloutComplete    = "rollout.complete"
	ImageMutableTag    = "image.mutable-tag"
)

// Phases d'un rollout (data-model.md § RolloutState).
const (
	PhaseComplete    = "complete"
	PhaseProgressing = "progressing"
	PhaseStalled     = "stalled"
	PhasePaused      = "paused"
)

// defaultDeadline est l'échéance de progression par défaut d'un Deployment.
const defaultDeadline = 600 * time.Second

func init() {
	knownIDs = append(knownIDs, RolloutStalled, RolloutProgressing, RolloutPaused, RolloutComplete, ImageMutableTag)
	diagnosers[KindDeployment] = DiagnoseDeployment
}

// DiagnoseDeployment décrit l'état du rollout d'un Deployment (research.md S5), signale un tag
// d'image mutable et une configuration figée (S3), puis diagnostique le pod en difficulté le
// plus récent de la nouvelle version avec les causes de la feature 001.
func DiagnoseDeployment(ctx Context, name string) (ObjectSnapshot, error) {
	dep, err := ctx.Reader.GetDeployment(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindDeployment, name)
	s.Captures["deployment"] = name
	desired := 1
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	s.Facts = append(s.Facts, Observation{Value: plural(desired, "réplique")})
	sel := selectorString(dep.Spec.Selector.MatchLabels)
	s.Captures["selector"] = sel

	pods, podsErr := ctx.Reader.PodsBySelector(sel)
	s.Deny(podsErr)
	rolloutLink(ctx, &s, dep, desired, pods.Items, podsErr == nil)
	imageLink(&s, dep)
	configLink(ctx, &s, dep, pods.Items, podsErr == nil)

	if len(s.Pitfalls) == 0 && s.Rollout != nil && s.Rollout.Phase == PhaseComplete {
		s.Add(Pitfall{ID: RolloutComplete, Step: StepRollout, Severity: OK, Captures: map[string]string{
			"ready": strconv.Itoa(s.Rollout.NewReady), "desired": strconv.Itoa(desired), "image": s.Rollout.NewImage,
		}})
	}
	return s, nil
}

func rolloutLink(ctx Context, s *ObjectSnapshot, dep kuberead.Deployment, desired int, pods []kuberead.Pod, podsRead bool) {
	sets, err := ctx.Reader.ReplicaSetsBySelector(s.Captures["selector"])
	if err != nil {
		s.Deny(err)
		s.AddLink(StepRollout, StateUnknown)
		return
	}
	var owned []kuberead.ReplicaSet
	for _, rs := range sets.Items {
		if rs.OwnedBy(dep.Metadata.Name) {
			owned = append(owned, rs)
		}
	}
	newRS, oldRS := versions(owned)
	r := &RolloutState{Phase: rolloutPhase(dep, desired), NewDesired: desired, Deadline: defaultDeadline}
	if dep.Spec.ProgressDeadlineSeconds != nil {
		r.Deadline = time.Duration(*dep.Spec.ProgressDeadlineSeconds) * time.Second
	}
	if newRS != nil {
		r.NewImage, r.NewReady = images(newRS.Spec.Template), newRS.Status.ReadyReplicas
	} else {
		r.NewImage = images(dep.Spec.Template)
	}
	obs := []Observation{{Label: "nouvelle version", Value: r.NewImage + " (" + strconv.Itoa(r.NewReady) + "/" + strconv.Itoa(desired) + " prêt(s))"}}
	if oldRS != nil {
		r.OldImage, r.OldReady = images(oldRS.Spec.Template), oldRS.Status.ReadyReplicas
		obs = append(obs, Observation{Label: "version précédente", Value: r.OldImage + " (" + strconv.Itoa(r.OldReady) + " prêt(s))"})
	}
	s.Rollout = r
	s.AddLink(StepRollout, StateOK, obs...)
	s.Facts = append(s.Facts, Observation{Label: "rollout", Value: phaseLabels[r.Phase]})
	if newRS != nil && podsRead {
		failingPod(ctx, s, r, newRS, pods)
	}
	caps := map[string]string{
		"newImage": r.NewImage, "oldImage": r.OldImage, "oldReady": strconv.Itoa(r.OldReady),
		"newReady": strconv.Itoa(r.NewReady), "desired": strconv.Itoa(desired),
		"deadline": strconv.Itoa(int(r.Deadline.Seconds())),
	}
	if r.FailingPod != nil {
		caps["pod"], caps["reason"] = r.FailingPod.Name, r.FailingPod.Reason
	}
	switch r.Phase {
	case PhaseStalled:
		s.Add(Pitfall{ID: RolloutStalled, Step: StepRollout, Severity: Warning, Captures: caps})
	case PhasePaused:
		s.Add(Pitfall{ID: RolloutPaused, Step: StepRollout, Severity: Info, Captures: caps})
	case PhaseProgressing:
		s.Add(Pitfall{ID: RolloutProgressing, Step: StepRollout, Severity: Info, Transient: true, Captures: caps})
	}
}

var phaseLabels = map[string]string{
	PhaseComplete: "terminé", PhaseProgressing: "en cours", PhaseStalled: "bloqué", PhasePaused: "en pause",
}

// rolloutPhase lit l'état du rollout sur les conditions du Deployment.
func rolloutPhase(dep kuberead.Deployment, desired int) string {
	if dep.Spec.Paused {
		return PhasePaused
	}
	for _, c := range dep.Status.Conditions {
		if c.Type == "Progressing" && c.Status == "False" && c.Reason == "ProgressDeadlineExceeded" {
			return PhaseStalled
		}
	}
	st := dep.Status
	if st.ObservedGeneration >= dep.Metadata.Generation && st.UpdatedReplicas == desired &&
		st.Replicas == desired && st.AvailableReplicas == desired {
		return PhaseComplete
	}
	return PhaseProgressing
}

// versions renvoie le ReplicaSet de la révision la plus haute (nouvelle version) et le plus
// récent des autres qui a encore des pods (version précédente).
func versions(sets []kuberead.ReplicaSet) (newRS, oldRS *kuberead.ReplicaSet) {
	for i := range sets {
		if newRS == nil || sets[i].Metadata.Annotations.Revision > newRS.Metadata.Annotations.Revision {
			newRS = &sets[i]
		}
	}
	for i := range sets {
		rs := &sets[i]
		if rs == newRS || rs.Status.Replicas == 0 {
			continue
		}
		if oldRS == nil || rs.Metadata.Annotations.Revision > oldRS.Metadata.Annotations.Revision {
			oldRS = rs
		}
	}
	return newRS, oldRS
}

// failingPod diagnostique le pod en difficulté le plus récent de la nouvelle version.
func failingPod(ctx Context, s *ObjectSnapshot, r *RolloutState, newRS *kuberead.ReplicaSet, pods []kuberead.Pod) {
	hash := newRS.Metadata.Labels["pod-template-hash"]
	var fresh []kuberead.Pod
	for _, p := range pods {
		if hash != "" && p.Metadata.Labels["pod-template-hash"] == hash {
			fresh = append(fresh, p)
		}
	}
	for _, c := range workload.SelectFailing(fresh, ctx.Now) {
		snap, d := workload.Diagnose(ctx.Reader, c.Pod, ctx.Scope, ctx.Catalog, ctx.Now)
		if snap.State != workload.StateFailing {
			continue
		}
		r.FailingPod = &snap
		r.FailingOrigin, r.FailingCaptures = string(d.Origin), d.Captures
		if d.Entry != nil {
			r.FailingEntry = d.Entry.ID
		}
		return
	}
}

// imageLink signale, sous forme d'information, une image au tag mutable (FR-009).
func imageLink(s *ObjectSnapshot, dep kuberead.Deployment) {
	var obs []Observation
	var mutable []string
	for _, c := range dep.Spec.Template.Spec.Containers {
		obs = append(obs, Observation{Label: c.Name, Value: c.Image})
		if ParseImage(c.Image).Mutable() {
			mutable = append(mutable, c.Image)
		}
	}
	s.AddLink(StepImage, StateOK, obs...)
	for _, img := range mutable {
		s.Add(Pitfall{ID: ImageMutableTag, Step: StepImage, Severity: Info, Captures: map[string]string{"image": img}})
	}
}

func images(t kuberead.PodTemplate) string {
	var out []string
	for _, c := range t.Spec.Containers {
		out = append(out, c.Image)
	}
	return strings.Join(out, ", ")
}
