package kuberead

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
)

// Ce paquet contient les SEULES lectures du cluster autorisées (FR-023, FR-029, research.md
// R8). Toute autre interaction est impossible : il n'expose que les fonctions ci-dessous, qui
// n'emploient que les verbes « get » et « logs », jamais sur des objets Secret. La liste est
// vérifiée par allowlist_test.go.

// Timeout est le délai maximal de chaque lecture.
const Timeout = 10 * time.Second

// LogLines est le nombre maximal de lignes de journal lues et affichées (FR-023).
const LogLines = 20

// Scope reprend les options de l'apprenant qui désignent le cluster et le namespace (FR-024).
type Scope struct {
	Context    string
	Namespace  string
	Kubeconfig string
	As         string // --as : agir sous une autre identité (pour vérifier ses droits)
}

// Reader lit le cluster avec la commande kubectl de l'apprenant.
type Reader struct {
	// Command est la commande kubectl réelle (TELEMACH_KUBECTL : « microk8s kubectl »), déjà
	// découpée en mots ; vide : « kubectl » du PATH.
	Command []string
	Scope   Scope
	// Budget limite les lectures (vérification différée, research.md S9) ; nil : aucune limite.
	Budget *Budget
}

// Budget compte les lectures permises, éventuellement partagées entre plusieurs lecteurs, et
// les lectures échouées.
type Budget struct {
	mu           sync.Mutex
	left, failed int
}

// NewBudget permet n lectures.
func NewBudget(n int) *Budget { return &Budget{left: n} }

// ErrBudget signale une lecture non faite : le budget est épuisé.
var ErrBudget = errors.New("budget de lectures épuisé")

func (b *Budget) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.left == 0 {
		return false
	}
	b.left--
	return true
}

func (b *Budget) fail() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failed++
}

// Failed renvoie le nombre de lectures échouées ou refusées faute de budget ; une ressource
// introuvable n'en est pas une, c'est un résultat.
func (b *Budget) Failed() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failed
}

// New construit un lecteur à partir de la variable TELEMACH_KUBECTL exportée par
// l'intégration shell (découpée en mots, sans shell ni évaluation).
func New(telemachKubectl string, scope Scope) *Reader {
	cmd := strings.Fields(telemachKubectl)
	if len(cmd) == 0 {
		cmd = []string{"kubectl"}
	}
	return &Reader{Command: cmd, Scope: scope}
}

// ErrDenied signale une lecture refusée par RBAC (FR-026).
type ErrDenied struct{ What, Message string }

func (e *ErrDenied) Error() string { return "lecture refusée : " + e.What }

// ErrKubectl est un échec de kubectl (cluster injoignable, ressource introuvable…) ; Stderr
// est nettoyé et masqué, pour être expliqué par le catalogue.
type ErrKubectl struct {
	ExitCode int
	Stderr   string
}

func (e *ErrKubectl) Error() string { return fmt.Sprintf("kubectl : code %d", e.ExitCode) }

// IsNotFound indique une ressource introuvable.
func IsNotFound(err error) bool {
	var ke *ErrKubectl
	return errors.As(err, &ke) && strings.Contains(ke.Stderr, "(NotFound)")
}

// ListPods lit les pods du namespace (diagnose sans argument).
func (r *Reader) ListPods() (PodList, error) {
	var l PodList
	err := r.getJSON("pods", &l, PodsArgs()...)
	return l, err
}

// GetPod lit un pod.
func (r *Reader) GetPod(name string) (Pod, error) {
	var p Pod
	err := r.getJSON("le pod "+name, &p, "get", "pod", name, "-o", "json")
	return p, err
}

// GetDeployment lit un deployment (pour son selector).
func (r *Reader) GetDeployment(name string) (Deployment, error) {
	var d Deployment
	err := r.getJSON("le deployment "+name, &d, DeploymentArgs(name)...)
	return d, err
}

// PodsBySelector lit les pods qui portent les labels donnés.
func (r *Reader) PodsBySelector(selector string) (PodList, error) {
	var l PodList
	err := r.getJSON("les pods "+selector, &l, PodsBySelectorArgs(selector)...)
	return l, err
}

// PodEvents lit les événements d'un pod.
func (r *Reader) PodEvents(pod string) (EventList, error) {
	var l EventList
	err := r.getJSON("les événements de "+pod, &l, PodEventsArgs(pod)...)
	return l, err
}

// Logs lit au plus LogLines lignes du journal d'un conteneur (courant ou précédent), nettoyées
// et masquées.
func (r *Reader) Logs(pod, container string, previous bool) ([]string, error) {
	args := []string{"logs", pod, "-c", container, fmt.Sprintf("--tail=%d", LogLines)}
	if previous {
		args = append(args, "--previous")
	}
	out, err := r.run("le journal de "+pod, args...)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l != "" {
			lines = append(lines, redact.Redact(sanitize.Clean(l)))
		}
	}
	if len(lines) > LogLines {
		lines = lines[len(lines)-LogLines:]
	}
	return lines, nil
}

// Lectures de la feature 002 (research.md S1), arguments définis dans commands.go.

// GetService lit un Service.
func (r *Reader) GetService(name string) (Service, error) {
	var v Service
	err := r.getJSON("le Service "+name, &v, ServiceArgs(name)...)
	return v, err
}

// ListServices lit les Services du namespace.
func (r *Reader) ListServices() (ServiceList, error) {
	var v ServiceList
	err := r.getJSON("les Services", &v, ServicesArgs()...)
	return v, err
}

// ServiceEndpointSlices lit les EndpointSlices d'un Service.
func (r *Reader) ServiceEndpointSlices(service string) (EndpointSliceList, error) {
	var v EndpointSliceList
	err := r.getJSON("les endpoints de "+service, &v, ServiceEndpointSlicesArgs(service)...)
	return v, err
}

// ListEndpointSlices lit les EndpointSlices du namespace.
func (r *Reader) ListEndpointSlices() (EndpointSliceList, error) {
	var v EndpointSliceList
	err := r.getJSON("les endpoints", &v, EndpointSlicesArgs()...)
	return v, err
}

// ListDeployments lit les Deployments du namespace.
func (r *Reader) ListDeployments() (DeploymentList, error) {
	var v DeploymentList
	err := r.getJSON("les deployments", &v, DeploymentsArgs()...)
	return v, err
}

// ReplicaSetsBySelector lit les ReplicaSets qui portent les labels donnés.
func (r *Reader) ReplicaSetsBySelector(selector string) (ReplicaSetList, error) {
	var v ReplicaSetList
	err := r.getJSON("les ReplicaSets "+selector, &v, ReplicaSetsArgs(selector)...)
	return v, err
}

// GetIngress lit un Ingress.
func (r *Reader) GetIngress(name string) (Ingress, error) {
	var v Ingress
	err := r.getJSON("l'Ingress "+name, &v, IngressArgs(name)...)
	return v, err
}

// ListIngresses lit les Ingress du namespace.
func (r *Reader) ListIngresses() (IngressList, error) {
	var v IngressList
	err := r.getJSON("les Ingress", &v, IngressesArgs()...)
	return v, err
}

// ListIngressClasses lit les classes d'Ingress du cluster.
func (r *Reader) ListIngressClasses() (IngressClassList, error) {
	var v IngressClassList
	err := r.getJSON("les classes d'Ingress", &v, IngressClassesArgs()...)
	return v, err
}

// GetPVC lit un PersistentVolumeClaim.
func (r *Reader) GetPVC(name string) (PVC, error) {
	var v PVC
	err := r.getJSON("le PVC "+name, &v, PVCArgs(name)...)
	return v, err
}

// ListPVCs lit les PersistentVolumeClaims du namespace.
func (r *Reader) ListPVCs() (PVCList, error) {
	var v PVCList
	err := r.getJSON("les PVC", &v, PVCsArgs()...)
	return v, err
}

// ListStorageClasses lit les classes de stockage du cluster.
func (r *Reader) ListStorageClasses() (StorageClassList, error) {
	var v StorageClassList
	err := r.getJSON("les classes de stockage", &v, StorageClassesArgs()...)
	return v, err
}

// ConfigMapDates lit les seules dates de modification d'une ConfigMap (research.md S3) :
// kubectl n'écrit que des dates, aucune valeur n'entre dans le plugin.
func (r *Reader) ConfigMapDates(name string) (ConfigMapDates, error) {
	out, err := r.run("les dates de la ConfigMap "+name, ConfigMapDatesArgs(name)...)
	if err != nil {
		return ConfigMapDates{Name: name}, err
	}
	return ParseConfigMapDates(name, out), nil
}

// getJSON exécute une lecture et décode sa sortie JSON dans v.
// Lectures de la feature 004 (research.md F1).

// GetJob lit un Job.
func (r *Reader) GetJob(name string) (Job, error) {
	var j Job
	err := r.getJSON("le Job "+name, &j, JobArgs(name)...)
	return j, err
}

// ListJobs lit les Jobs du namespace.
func (r *Reader) ListJobs() (JobList, error) {
	var l JobList
	err := r.getJSON("les Jobs", &l, JobsArgs()...)
	return l, err
}

// GetCronJob lit un CronJob.
func (r *Reader) GetCronJob(name string) (CronJob, error) {
	var c CronJob
	err := r.getJSON("le CronJob "+name, &c, CronJobArgs(name)...)
	return c, err
}

// GetDaemonSet lit un DaemonSet.
func (r *Reader) GetDaemonSet(name string) (DaemonSet, error) {
	var d DaemonSet
	err := r.getJSON("le DaemonSet "+name, &d, DaemonSetArgs(name)...)
	return d, err
}

// GetStatefulSet lit un StatefulSet.
func (r *Reader) GetStatefulSet(name string) (StatefulSet, error) {
	var s StatefulSet
	err := r.getJSON("le StatefulSet "+name, &s, StatefulSetArgs(name)...)
	return s, err
}

// GetHPA lit un HorizontalPodAutoscaler.
func (r *Reader) GetHPA(name string) (HPA, error) {
	var h HPA
	err := r.getJSON("le HPA "+name, &h, HPAArgs(name)...)
	return h, err
}

// ListNodes lit les nœuds du cluster.
func (r *Reader) ListNodes() (NodeList, error) {
	var l NodeList
	err := r.getJSON("les nœuds", &l, NodesArgs()...)
	return l, err
}

// GetMetricsAPIService lit l'APIService de l'API de métriques.
func (r *Reader) GetMetricsAPIService() (APIService, error) {
	var a APIService
	err := r.getJSON("l'API de métriques", &a, MetricsAPIServiceArgs()...)
	return a, err
}

// ReplicaSetEvents lit les événements d'un ReplicaSet.
func (r *Reader) ReplicaSetEvents(rs string) (EventList, error) {
	var l EventList
	err := r.getJSON("les événements du ReplicaSet "+rs, &l, ReplicaSetEventsArgs(rs)...)
	return l, err
}

func (r *Reader) getJSON(what string, v any, args ...string) error {
	out, err := r.run(what, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(out), v); err != nil {
		return fmt.Errorf("réponse de kubectl illisible pour %s : %w", what, err)
	}
	return nil
}

var forbidden = regexp.MustCompile(`\(Forbidden\)|is forbidden`)

// run exécute la commande kubectl de l'apprenant avec une liste d'arguments (jamais de shell)
// suivie de ses options de portée et d'un délai maximal.
func (r *Reader) run(what string, args ...string) (string, error) {
	out, err := r.exec(what, args...)
	if err != nil && r.Budget != nil && !IsNotFound(err) {
		r.Budget.fail()
	}
	return out, err
}

func (r *Reader) exec(what string, args ...string) (string, error) {
	if r.Budget != nil && !r.Budget.take() {
		return "", ErrBudget
	}
	full := append(append([]string(nil), r.Command[1:]...), args...)
	full = append(full, r.Scope.flags()...)
	full = append(full, "--request-timeout="+Timeout.String())
	ctx, cancel := context.WithTimeout(context.Background(), Timeout+2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Command[0], full...) //nolint:gosec // commande kubectl de l'apprenant, arguments en liste
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return out.String(), nil
	}
	stderr := redact.Redact(sanitize.Clean(errb.String()))
	if forbidden.MatchString(stderr) {
		return "", &ErrDenied{What: what, Message: firstLine(stderr)}
	}
	code := -1
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	}
	if stderr == "" {
		stderr = redact.Redact(sanitize.Clean(err.Error()))
	}
	return "", &ErrKubectl{ExitCode: code, Stderr: stderr}
}

// flags renvoie les options de portée de l'apprenant.
func (s Scope) flags() []string {
	var out []string
	if s.Context != "" {
		out = append(out, "--context", s.Context)
	}
	if s.Namespace != "" {
		out = append(out, "-n", s.Namespace)
	}
	if s.Kubeconfig != "" {
		out = append(out, "--kubeconfig", s.Kubeconfig)
	}
	if s.As != "" {
		out = append(out, "--as", s.As)
	}
	return out
}

// Flags renvoie les options de portée à reprendre dans une commande proposée.
func (s Scope) Flags() string { return strings.Join(s.flags(), " ") }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
