package pitfall

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// Kind est le type d'objet diagnostiqué.
type Kind string

// Types d'objets pris en charge par diagnose (contracts/cli.md).
const (
	KindService    Kind = "service"
	KindDeployment Kind = "deployment"
	KindIngress    Kind = "ingress"
	KindPVC        Kind = "pvc"
	// Feature 004.
	KindJob         Kind = "job"
	KindCronJob     Kind = "cronjob"
	KindDaemonSet   Kind = "daemonset"
	KindStatefulSet Kind = "statefulset"
	KindHPA         Kind = "hpa"
)

// Step est un maillon vérifié (data-model.md § Link).
type Step string

// Maillons, dans l'ordre de vérification de chaque type d'objet.
const (
	StepType         Step = "type"
	StepSelector     Step = "selector"
	StepReadiness    Step = "readiness"
	StepEndpoints    Step = "endpoints"
	StepPorts        Step = "ports"
	StepRollout      Step = "rollout"
	StepImage        Step = "image"
	StepConfig       Step = "config"
	StepClass        Step = "class"
	StepBackend      Step = "backend"
	StepAddress      Step = "address"
	StepStorageClass Step = "storageclass"
	StepBinding      Step = "binding"
	StepConsumer     Step = "consumer"
	// Feature 004 (data-model.md § Step).
	StepAdmission  Step = "admission"
	StepStatus     Step = "status"
	StepPods       Step = "pods"
	StepSchedule   Step = "schedule"
	StepHistory    Step = "history"
	StepLastJob    Step = "last-job"
	StepNodes      Step = "nodes"
	StepService    Step = "service"
	StepOrdinal    Step = "ordinal"
	StepVolumes    Step = "volumes"
	StepMetricsAPI Step = "metrics-api"
	StepTarget     Step = "target"
	StepRequests   Step = "requests"
	StepConditions Step = "conditions"
)

// LinkState est l'état d'un maillon.
type LinkState string

// États d'un maillon : unknown quand la lecture nécessaire a échoué ou a été refusée (jamais
// broken sur une information non lue, FR-020).
const (
	StateOK      LinkState = "ok"
	StateBroken  LinkState = "broken"
	StateUnknown LinkState = "unknown"
	StateSkipped LinkState = "skipped"
)

// Observation est une valeur observée, affichée telle quelle (déjà masquée).
type Observation struct {
	Label, Value string
}

// Link est un maillon et ce qui y a été observé.
type Link struct {
	Step     Step
	State    LinkState
	Observed []Observation
}

// Severity distingue un avertissement d'une information non bloquante.
type Severity string

// Gravités d'un piège ; OK confirme un objet sain (sa leçon donne les commandes de test).
const (
	Warning Severity = "warning"
	Info    Severity = "info"
	OK      Severity = "ok"
)

// Pitfall est un piège trouvé (data-model.md § Pitfall) ; ID désigne sa leçon du catalogue.
type Pitfall struct {
	ID        string
	Step      Step
	Severity  Severity
	Transient bool              // état transitoire : jamais signalé par la vérification différée
	Captures  map[string]string // valeurs insérées dans la leçon, masquées
	// Lesson désigne une autre entrée du catalogue qui explique mieux ce piège (feature 004 :
	// refus d'admission vu depuis un Deployment, expliqué par la leçon du quota).
	Lesson string `json:",omitempty"`
}

// LessonEntry renvoie l'entrée qui porte la leçon du piège : Lesson si elle existe, sinon
// l'entrée du piège.
func LessonEntry(cat *catalog.Catalog, p Pitfall) (*catalog.Entry, bool) {
	if cat == nil {
		return nil, false
	}
	if p.Lesson != "" {
		if e, ok := cat.Get(p.Lesson); ok {
			return e, true
		}
	}
	return cat.ByPitfall(p.ID)
}

// RolloutState est l'état d'un rollout (data-model.md § RolloutState).
type RolloutState struct {
	Phase      string // complete, progressing, stalled, paused
	NewImage   string
	OldImage   string
	NewReady   int
	NewDesired int
	OldReady   int
	Deadline   time.Duration
}

// FailingPod est un pod en difficulté lié à l'objet (pod de la nouvelle version d'un rollout,
// pod qui utilise un PVC), diagnostiqué comme dans la feature 001 ; Entry, Origin et Captures
// permettent de réafficher sa leçon sans relire le cluster (last).
type FailingPod struct {
	Pod      workload.PodSnapshot
	Entry    string
	Origin   string
	Captures map[string]string
}

// ObjectSummary est un objet en difficulté de la liste courte de diagnose sans argument.
type ObjectSummary struct {
	Kind    Kind
	Name    string
	Pitfall string // identifiant du premier piège
	// Captures complète le résumé de la leçon (classe demandée…), masquées.
	Captures map[string]string `json:",omitempty"`
}

// ObjectSnapshot est la vue masquée d'un objet diagnostiqué (data-model.md § ObjectSnapshot).
type ObjectSnapshot struct {
	Kind        Kind
	Name        string
	Namespace   string
	Context     string
	Facts       []Observation // détails de l'en-tête (type de Service, ports, image…)
	Links       []Link
	Pitfalls    []Pitfall
	Pods        []workload.PodSummary
	Rollout     *RolloutState
	Failing     *FailingPod
	DeniedReads []string
	Others      []ObjectSummary
	// Captures sont les valeurs communes à toutes les leçons de l'objet (kubectl, scope,
	// nom…), déjà masquées ; celles d'un piège les complètent.
	Captures map[string]string
}

// AddLink ajoute un maillon.
func (s *ObjectSnapshot) AddLink(step Step, state LinkState, observed ...Observation) {
	s.Links = append(s.Links, Link{Step: step, State: state, Observed: observed})
}

// Observe ajoute une observation à un maillon existant.
func (s *ObjectSnapshot) Observe(step Step, label, value string) {
	if l := s.link(step); l != nil {
		l.Observed = append(l.Observed, Observation{label, value})
	}
}

// LessonCaptures renvoie les valeurs d'une leçon : celles de l'objet complétées par celles du
// piège.
func (s *ObjectSnapshot) LessonCaptures(p Pitfall) map[string]string {
	out := map[string]string{}
	for k, v := range s.Captures {
		out[k] = v
	}
	for k, v := range p.Captures {
		out[k] = v
	}
	return out
}

// Add ajoute un piège et casse son maillon s'il s'agit d'un avertissement. Il refuse un
// avertissement dont le maillon n'a pas pu être lu ou n'a pas été vérifié : aucune cause n'est
// affirmée sur une information manquante (FR-020). Une information peut, elle, porter sur un
// maillon non vérifiable (port non déclaré).
func (s *ObjectSnapshot) Add(p Pitfall) bool {
	l := s.link(p.Step)
	if l == nil || (p.Severity == Warning && (l.State == StateUnknown || l.State == StateSkipped)) {
		return false
	}
	if p.Severity == Warning {
		l.State = StateBroken
	}
	s.Pitfalls = append(s.Pitfalls, p)
	return true
}

// Deny enregistre une lecture refusée par RBAC (diagnostic partiel) ; une autre erreur est
// ignorée ici (elle rend seulement le maillon unknown).
func (s *ObjectSnapshot) Deny(err error) {
	var d *kuberead.ErrDenied
	if errors.As(err, &d) {
		for _, w := range s.DeniedReads {
			if w == d.What {
				return
			}
		}
		s.DeniedReads = append(s.DeniedReads, d.What)
	}
}

// Partial indique un diagnostic incomplet faute de droits.
func (s *ObjectSnapshot) Partial() bool { return len(s.DeniedReads) > 0 }

// Primary renvoie le piège qui porte la leçon : le premier avertissement dans l'ordre des
// maillons, sinon la première information, sinon la confirmation d'un objet sain ; nil si rien
// n'a été trouvé.
func (s *ObjectSnapshot) Primary() *Pitfall {
	for _, sev := range []Severity{Warning, Info, OK} {
		for _, l := range s.Links {
			for i := range s.Pitfalls {
				if p := &s.Pitfalls[i]; p.Step == l.Step && p.Severity == sev {
					return p
				}
			}
		}
	}
	return nil
}

// Secondary renvoie les autres pièges : les avertissements puis les informations, chacun dans
// l'ordre des maillons.
func (s *ObjectSnapshot) Secondary() []Pitfall {
	primary := s.Primary()
	var out []Pitfall
	for _, sev := range []Severity{Warning, Info} {
		for _, l := range s.Links {
			for i := range s.Pitfalls {
				if p := &s.Pitfalls[i]; p.Step == l.Step && p.Severity == sev && p != primary {
					out = append(out, *p)
				}
			}
		}
	}
	return out
}

// Healthy indique qu'aucun avertissement n'a été trouvé (des informations peuvent exister).
func (s *ObjectSnapshot) Healthy() bool {
	for _, p := range s.Pitfalls {
		if p.Severity == Warning {
			return false
		}
	}
	return true
}

func (s *ObjectSnapshot) link(step Step) *Link {
	for i := range s.Links {
		if s.Links[i].Step == step {
			return &s.Links[i]
		}
	}
	return nil
}

// LinkStateFor donne l'état d'un maillon d'après le résultat de sa lecture : une erreur rend
// le maillon unknown, jamais broken.
func LinkStateFor(err error) LinkState {
	if err != nil {
		return StateUnknown
	}
	return StateOK
}

// knownIDs regroupe les pièges que le code peut produire ; chaque fichier de diagnostic y
// ajoute les siens. CheckCatalog vérifie que chacun a sa leçon.
var knownIDs []string

// CheckCatalog vérifie que chaque piège connu du code a une leçon dans le catalogue.
func CheckCatalog(cat *catalog.Catalog) error { return checkIDs(cat, knownIDs) }

func checkIDs(cat *catalog.Catalog, ids []string) error {
	var missing []string
	for _, id := range ids {
		if _, ok := cat.ByPitfall(id); !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("piège(s) sans leçon dans le catalogue : %s", strings.Join(missing, ", "))
	}
	return nil
}

// Reader regroupe les lectures utilisées par les diagnostics d'objets (kuberead.Reader, ou un
// faux pour les tests).
type Reader interface {
	workload.Reader
	GetPod(name string) (kuberead.Pod, error)
	ListPods() (kuberead.PodList, error)
	PodsBySelector(selector string) (kuberead.PodList, error)
	GetService(name string) (kuberead.Service, error)
	ListServices() (kuberead.ServiceList, error)
	ServiceEndpointSlices(service string) (kuberead.EndpointSliceList, error)
	ListEndpointSlices() (kuberead.EndpointSliceList, error)
	GetDeployment(name string) (kuberead.Deployment, error)
	ListDeployments() (kuberead.DeploymentList, error)
	ReplicaSetsBySelector(selector string) (kuberead.ReplicaSetList, error)
	GetIngress(name string) (kuberead.Ingress, error)
	ListIngresses() (kuberead.IngressList, error)
	ListIngressClasses() (kuberead.IngressClassList, error)
	GetPVC(name string) (kuberead.PVC, error)
	ListPVCs() (kuberead.PVCList, error)
	ListStorageClasses() (kuberead.StorageClassList, error)
	ConfigMapDates(name string) (kuberead.ConfigMapDates, error)
	// Feature 004 (research.md F1).
	GetJob(name string) (kuberead.Job, error)
	ListJobs() (kuberead.JobList, error)
	GetCronJob(name string) (kuberead.CronJob, error)
	GetDaemonSet(name string) (kuberead.DaemonSet, error)
	GetStatefulSet(name string) (kuberead.StatefulSet, error)
	GetHPA(name string) (kuberead.HPA, error)
	ListNodes() (kuberead.NodeList, error)
	GetMetricsAPIService() (kuberead.APIService, error)
	ReplicaSetEvents(rs string) (kuberead.EventList, error)
}

var _ Reader = (*kuberead.Reader)(nil)
