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

// getJSON exécute une lecture et décode sa sortie JSON dans v.
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
