package workload

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
)

// PodSnapshot est le pod diagnostiqué (data-model.md § PodSnapshot). Toutes ses valeurs
// textuelles sont nettoyées et masquées ; il ne contient jamais la valeur d'une variable
// d'environnement ni le contenu d'un Secret.
type PodSnapshot struct {
	Context     string
	Namespace   string
	Name        string
	Owner       string
	CreatedAt   time.Time
	Phase       string
	State       string
	Reason      string
	Container   string
	Image       string
	ExitCode    int
	Restarts    int
	Message     string
	EnvNames    []string
	SecretRefs  []string
	ConfigRefs  []string
	Events      []EventSummary
	LogTail     []string
	LogPrevious bool
	DeniedReads []string
	Others      []PodSummary
	// Feature 004 : causes de placement d'un pod Pending, sur PlacementTotal nœuds.
	Placement      []PlacementCause
	PlacementTotal int
}

// EventSummary est un événement résumé, masqué.
type EventSummary struct {
	Reason  string
	Message string
	Count   int
}

// PodSummary est un autre pod en difficulté, pour la liste courte (FR-022).
type PodSummary struct {
	Name   string
	Reason string
}

// Reader regroupe les lectures nécessaires au diagnostic (kuberead.Reader, ou des données de
// test).
type Reader interface {
	PodEvents(pod string) (kuberead.EventList, error)
	Logs(pod, container string, previous bool) ([]string, error)
}

// missingRef reconnaît l'objet manquant d'un CreateContainerConfigError.
var missingRef = regexp.MustCompile(`(secret|configmap) "([^"]+)" not found`)

// noLogs signale une réponse de kubectl logs qui n'est pas un journal.
var noLogs = regexp.MustCompile(`^unable to retrieve container logs|^Error from server`)

// Diagnose diagnostique un pod (FR-025, FR-026) : classement par son statut, puis événements
// et fin du journal du conteneur en cause. Une lecture refusée rend le diagnostic partiel.
func Diagnose(r Reader, p kuberead.Pod, scope kuberead.Scope, cat *catalog.Catalog, now time.Time) (PodSnapshot, diagnose.Diagnosis) {
	cl := Classify(p, now)
	snap := PodSnapshot{
		Context: scope.Context, Namespace: p.Metadata.Namespace, Name: p.Metadata.Name,
		CreatedAt: p.Metadata.CreationTimestamp, Phase: p.Status.Phase, State: cl.State,
		Reason: cl.Reason, Container: cl.Container, ExitCode: cl.ExitCode, Restarts: cl.Restarts,
		Message: clean(cl.Message),
	}
	if len(p.Metadata.OwnerReferences) > 0 {
		o := p.Metadata.OwnerReferences[0]
		snap.Owner = o.Kind + "/" + o.Name
	}
	fillContainer(&snap, p)
	if len(cl.Causes) > 0 {
		snap.Placement, snap.PlacementTotal = cl.Causes, cl.Total
		refineNodes(r, p, &snap)
	}
	if cl.State == StateFailing {
		readEvents(r, &snap)
		refine(&snap)
		readLogs(r, &snap)
	}
	d := diagnose.Diagnosis{Source: diagnose.SourcePod, Confidence: diagnose.Unknown, Origin: diagnose.OriginCluster, Captures: captures(snap, scope)}
	if cat != nil {
		if e, ok := cat.ByPodReason(snap.Reason); ok {
			d.Entry, d.Confidence = e, diagnose.Recognized
		}
		// Un motif reconnu dans le journal (nom DNS court…) est plus précis que l'état du pod.
		if e, values, ok := cat.ByLogLines(snap.LogTail); ok {
			d.Entry, d.Confidence = e, diagnose.Recognized
			for k, v := range values {
				d.Captures[k] = v
			}
		}
	}
	if d.Entry != nil && len(snap.DeniedReads) > 0 {
		d.Confidence = diagnose.Partial
	}
	if snap.Reason == ReasonCrashLoopBackOff || snap.Reason == ReasonError {
		d.Origin = diagnose.OriginApplication
	}
	return snap, d
}

// fillContainer reprend l'image, les noms des variables et les références aux Secret et
// ConfigMap du conteneur en cause (jamais les valeurs).
func fillContainer(snap *PodSnapshot, p kuberead.Pod) {
	all := append(append([]kuberead.Container(nil), p.Spec.InitContainers...), p.Spec.Containers...)
	for _, c := range all {
		if snap.Container == "" {
			snap.Container = c.Name
		}
		if c.Name != snap.Container {
			continue
		}
		snap.Image = clean(c.Image)
		for _, e := range c.Env {
			snap.EnvNames = append(snap.EnvNames, e.Name)
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				snap.SecretRefs = append(snap.SecretRefs, e.ValueFrom.SecretKeyRef.Name)
			}
			if e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil {
				snap.ConfigRefs = append(snap.ConfigRefs, e.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
		for _, f := range c.EnvFrom {
			if f.SecretRef != nil {
				snap.SecretRefs = append(snap.SecretRefs, f.SecretRef.Name)
			}
			if f.ConfigMapRef != nil {
				snap.ConfigRefs = append(snap.ConfigRefs, f.ConfigMapRef.Name)
			}
		}
	}
}

// readEvents lit les événements du pod ; une lecture refusée est notée (FR-026).
func readEvents(r Reader, snap *PodSnapshot) {
	events, err := r.PodEvents(snap.Name)
	var denied *kuberead.ErrDenied
	switch {
	case errors.As(err, &denied):
		snap.DeniedReads = append(snap.DeniedReads, "les événements du pod")
		return
	case err != nil:
		return
	}
	sort.SliceStable(events.Items, func(i, j int) bool {
		return when(events.Items[i]).After(when(events.Items[j]))
	})
	for _, e := range events.Items {
		if e.Type != "Warning" {
			continue
		}
		snap.Events = append(snap.Events, EventSummary{Reason: e.Reason, Message: clean(e.Message), Count: e.Count})
	}
}

func when(e kuberead.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp
	}
	return e.EventTime
}

// refine précise la raison d'après les événements : sondes en échec, planification.
func refine(snap *PodSnapshot) {
	for _, e := range snap.Events {
		switch {
		case e.Reason == "Unhealthy" && strings.HasPrefix(e.Message, "Liveness probe failed"):
			snap.Reason, snap.Message = ReasonLiveness, e.Message
			return
		case e.Reason == "Unhealthy" && strings.HasPrefix(e.Message, "Readiness probe failed") && snap.Reason == ReasonNotReady:
			snap.Reason, snap.Message = ReasonReadiness, e.Message
			return
		case e.Reason == "FailedScheduling" && snap.Message == "":
			snap.Message = e.Message
		}
	}
	if snap.Reason == ReasonNotReady {
		// Pas prêt, sans sonde en échec dans les événements : probablement encore en démarrage.
		snap.State = StateStarting
	}
}

// readLogs lit la fin du journal du conteneur en cause, quand le programme a démarré : le
// journal précédent après un redémarrage, sinon le journal courant.
func readLogs(r Reader, snap *PodSnapshot) {
	switch snap.Reason {
	case ReasonCrashLoopBackOff, ReasonError, ReasonOOMKilled, ReasonLiveness, ReasonReadiness:
	default:
		return
	}
	try := func(previous bool) bool {
		lines, err := r.Logs(snap.Name, snap.Container, previous)
		var denied *kuberead.ErrDenied
		if errors.As(err, &denied) {
			snap.DeniedReads = append(snap.DeniedReads, "le journal du conteneur")
			return true
		}
		if err != nil || len(lines) == 0 || noLogs.MatchString(lines[0]) {
			return false
		}
		snap.LogTail, snap.LogPrevious = lines, previous
		return true
	}
	if snap.Restarts > 0 && try(true) {
		return
	}
	try(false)
}

// captures fournit aux gabarits du catalogue les éléments du diagnostic.
func captures(snap PodSnapshot, scope kuberead.Scope) map[string]string {
	c := map[string]string{
		"kubectl": "kubectl", "pod": snap.Name, "name": snap.Name, "container": snap.Container,
		"image": snap.Image, "reason": snap.Reason, "message": firstSentence(snap.Message),
		"restarts": strconv.Itoa(snap.Restarts), "scope": scope.Flags(),
	}
	if snap.Namespace != "" {
		c["namespace"] = snap.Namespace
		if scope.Namespace == "" {
			c["scope"] = strings.TrimSpace("-n " + snap.Namespace + " " + scope.Flags())
		}
	}
	if snap.ExitCode != 0 {
		c["exit_code"] = strconv.Itoa(snap.ExitCode)
	}
	if m := missingRef.FindStringSubmatch(snap.Message); m != nil {
		c["missing_kind"], c["missing"] = map[string]string{"secret": "Secret", "configmap": "ConfigMap"}[m[1]], m[2]
	}
	if r := insufficient.FindStringSubmatch(snap.Message); r != nil {
		c["resource"] = r[1]
	}
	if l := clueLine(snap.LogTail); l != "" {
		c["log_last"] = l
	}
	if snap.Owner != "" {
		c["owner"] = snap.Owner
	}
	// Feature 004 : cause principale de placement (taint, nœuds concernés).
	if len(snap.Placement) > 0 {
		main := MainCause(snap.Placement)
		if main.Reason == ReasonUntoleratedTaint && main.Detail != "" {
			c["taint"] = main.Detail
		}
		if len(main.NodeNames) > 0 {
			c["node_names"] = strings.Join(main.NodeNames, ", ")
			c["node"] = main.NodeNames[0]
		}
		c["nodes_total"] = strconv.Itoa(snap.PlacementTotal)
	}
	return c
}

// firstSentence raccourcit un message long à sa première phrase utile.
func firstSentence(s string) string {
	if i := strings.Index(s, ". preemption:"); i > 0 {
		s = s[:i]
	}
	if len([]rune(s)) > 220 {
		s = string([]rune(s)[:219]) + "…"
	}
	return s
}

func clean(s string) string { return redact.Redact(sanitize.Clean(s)) }

// errorWords repèrent, dans un journal, la ligne qui ressemble le plus à un message d'erreur.
var errorWords = regexp.MustCompile(`(?i)erreur|error|fatal|exception|panic|failed|échec|obligatoire|required|missing|manquant|not found|introuvable|denied|refused|refusé|cannot|impossible`)

// clueLine renvoie la dernière ligne du journal qui ressemble à une erreur, sinon la dernière
// ligne : stdout et stderr étant entrelacés, la vraie cause n'est pas toujours à la fin.
func clueLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if errorWords.MatchString(lines[i]) {
			return lines[i]
		}
	}
	if n := len(lines); n > 0 {
		return lines[n-1]
	}
	return ""
}
