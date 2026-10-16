package workload

import (
	"regexp"
	"sort"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// États d'un pod pour le diagnostic (data-model.md § PodSnapshot).
const (
	StateHealthy  = "healthy"  // ignoré
	StateStarting = "starting" // en cours de création : jamais signalé
	StateFailing  = "failing"  // diagnostiqué
)

// Raisons normalisées. Les premières sont celles de Kubernetes ; les suivantes sont déduites
// du statut et des événements (FR-025).
const (
	ReasonImagePullBackOff     = "ImagePullBackOff"
	ReasonErrImagePull         = "ErrImagePull"
	ReasonInvalidImageName     = "InvalidImageName"
	ReasonCrashLoopBackOff     = "CrashLoopBackOff"
	ReasonCreateConfigError    = "CreateContainerConfigError"
	ReasonCreateContainerError = "CreateContainerError"
	ReasonRunContainerError    = "RunContainerError"
	ReasonOOMKilled            = "OOMKilled"
	ReasonError                = "Error"
	ReasonInsufficient         = "InsufficientResources"
	ReasonUnboundPVC           = "UnboundPersistentVolumeClaim"
	ReasonNodeSelector         = "NodeSelectorMismatch"
	ReasonUnschedulable        = "Unschedulable"
	ReasonReadiness            = "ReadinessProbeFailed"
	ReasonLiveness             = "LivenessProbeFailed"
	ReasonNotReady             = "NotReady" // pas prêt depuis longtemps, cause à confirmer par les événements
)

// failingWaiting sont les raisons d'attente qui signalent un échec.
var failingWaiting = map[string]bool{
	ReasonImagePullBackOff: true, ReasonErrImagePull: true, ReasonInvalidImageName: true,
	ReasonCrashLoopBackOff: true, ReasonCreateConfigError: true, ReasonCreateContainerError: true,
	ReasonRunContainerError: true, "StartError": true,
}

// NotReadyGrace est la durée au-delà de laquelle un conteneur démarré mais pas prêt est
// considéré comme en difficulté (avant, il est probablement en train de démarrer).
const NotReadyGrace = 30 * time.Second

var (
	insufficient = regexp.MustCompile(`Insufficient (\w+)`)
	unboundPVC   = regexp.MustCompile(`unbound immediate PersistentVolumeClaims|persistentvolumeclaim "[^"]+" not found`)
	nodeSelector = regexp.MustCompile(`didn't match Pod's node affinity/selector`)
)

// Classification est le résultat de l'analyse du seul statut d'un pod.
type Classification struct {
	State     string
	Reason    string
	Container string // conteneur en cause
	Message   string // message du statut (non masqué : à masquer avant affichage)
	ExitCode  int
	Restarts  int
	Resource  string // ressource insuffisante (cpu, memory)
}

// Classify range un pod d'après son seul statut, sans lecture supplémentaire.
func Classify(p kuberead.Pod, now time.Time) Classification {
	s := p.Status
	if s.Phase == "Succeeded" {
		return Classification{State: StateHealthy}
	}
	for _, c := range s.Conditions {
		if c.Type == "PodScheduled" && c.Status == "False" {
			cl := Classification{State: StateFailing, Reason: ReasonUnschedulable, Message: c.Message}
			switch {
			case insufficient.MatchString(c.Message):
				cl.Reason = ReasonInsufficient
				cl.Resource = insufficient.FindStringSubmatch(c.Message)[1]
			case unboundPVC.MatchString(c.Message):
				cl.Reason = ReasonUnboundPVC
			case nodeSelector.MatchString(c.Message):
				cl.Reason = ReasonNodeSelector
			}
			return cl
		}
	}
	statuses := append(append([]kuberead.ContainerStatus(nil), s.InitContainerStatuses...), s.ContainerStatuses...)
	starting := false
	var notReady *Classification
	for i, cs := range statuses {
		cl := Classification{Container: cs.Name, Restarts: cs.RestartCount}
		if i < len(s.InitContainerStatuses) && cs.State.Running != nil {
			// Un conteneur d'initialisation qui tourne fait partie du démarrage normal.
			starting = true
			continue
		}
		last := cs.LastState.Terminated
		switch {
		case cs.State.Waiting != nil && failingWaiting[cs.State.Waiting.Reason]:
			cl.State, cl.Reason, cl.Message = StateFailing, cs.State.Waiting.Reason, cs.State.Waiting.Message
			if cl.Reason == "StartError" {
				cl.Reason = ReasonRunContainerError
			}
			if cl.Reason == ReasonCrashLoopBackOff && last != nil {
				cl.ExitCode, cl.Message = last.ExitCode, last.Message
				if last.Reason == ReasonOOMKilled {
					cl.Reason = ReasonOOMKilled
				}
			}
			return cl
		case cs.State.Waiting != nil:
			starting = true
		case cs.State.Terminated != nil && cs.State.Terminated.Reason == ReasonOOMKilled:
			cl.State, cl.Reason, cl.ExitCode = StateFailing, ReasonOOMKilled, cs.State.Terminated.ExitCode
			return cl
		case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
			cl.State, cl.ExitCode, cl.Message = StateFailing, cs.State.Terminated.ExitCode, cs.State.Terminated.Message
			cl.Reason = ReasonError
			if cs.RestartCount > 0 {
				cl.Reason = ReasonCrashLoopBackOff
			}
			return cl
		case cs.State.Running != nil && cs.RestartCount > 0 && last != nil && last.Reason == ReasonOOMKilled:
			cl.State, cl.Reason, cl.ExitCode = StateFailing, ReasonOOMKilled, last.ExitCode
			return cl
		case cs.State.Running != nil && cs.RestartCount > 0 && last != nil && last.ExitCode != 0 &&
			(!cs.Ready || now.Sub(last.FinishedAt) < RecentRestart):
			cl.State, cl.Reason, cl.ExitCode, cl.Message = StateFailing, ReasonCrashLoopBackOff, last.ExitCode, last.Message
			return cl
		case cs.State.Running != nil && !cs.Ready:
			if now.Sub(cs.State.Running.StartedAt) > NotReadyGrace || cs.RestartCount > 0 {
				c := cl
				c.State, c.Reason = StateFailing, ReasonNotReady
				if last != nil {
					c.ExitCode = last.ExitCode
				}
				notReady = &c
			} else {
				starting = true
			}
		}
	}
	if notReady != nil {
		return *notReady
	}
	if starting || s.Phase == "Pending" {
		return Classification{State: StateStarting, Reason: waitingReason(statuses)}
	}
	if s.Phase == "Failed" {
		return Classification{State: StateFailing, Reason: ReasonError, Message: s.Message}
	}
	return Classification{State: StateHealthy}
}

func waitingReason(statuses []kuberead.ContainerStatus) string {
	for _, cs := range statuses {
		if cs.State.Waiting != nil {
			return cs.State.Waiting.Reason
		}
	}
	return ""
}

// Candidate est un pod en difficulté, d'après son seul statut.
type Candidate struct {
	Pod   kuberead.Pod
	Class Classification
}

// SelectFailing garde les pods en difficulté, du plus récent au plus ancien (FR-022).
func SelectFailing(pods []kuberead.Pod, now time.Time) []Candidate {
	var out []Candidate
	for _, p := range pods {
		if c := Classify(p, now); c.State == StateFailing {
			out = append(out, Candidate{Pod: p, Class: c})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Pod.Metadata.CreationTimestamp.After(out[j].Pod.Metadata.CreationTimestamp)
	})
	return out
}

// RecentRestart : un conteneur qui a redémarré après une erreur il y a moins de ce délai est en
// difficulté, même s'il tourne de nouveau (boucle de redémarrage, sonde de vie en échec).
const RecentRestart = 10 * time.Minute

// MaxOthers est le nombre maximal d'autres pods listés sous l'explication.
const MaxOthers = 10
