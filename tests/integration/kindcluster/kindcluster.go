//go:build integration

// Package kindcluster crée et pilote un cluster kind JETABLE pour les tests d'intégration et
// le relevé du corpus. Il n'utilise jamais le kubeconfig ni le contexte de l'utilisateur : le
// cluster a son propre fichier kubeconfig temporaire, et toute commande refuse de s'exécuter
// sur un autre contexte que « kind-telemach-* » (D-LL-015, principe III).
package kindcluster

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Prefix est le préfixe obligatoire des clusters et contextes de test.
const Prefix = "telemach-"

// Cluster est un cluster kind jetable.
type Cluster struct {
	Name       string // telemach-xxxxxx
	Context    string // kind-telemach-xxxxxx
	Kubeconfig string // fichier kubeconfig propre au cluster
	Dir        string // dossier temporaire du cluster
	Kind       string // binaire kind
	KubectlBin string // binaire kubectl

	workDir string // dossier de travail de la prochaine commande (KubectlIn)
}

// Result est le résultat d'une commande kubectl.
type Result struct {
	Stdout, Stderr string
	ExitCode       int
}

// Create crée un cluster kind à partir de l'image donnée (épinglée par digest).
func Create(image string) (*Cluster, error) {
	if !strings.Contains(image, "@sha256:") {
		return nil, fmt.Errorf("l'image kind doit être épinglée par digest : %q", image)
	}
	dir, err := os.MkdirTemp("", "telemach-kind-")
	if err != nil {
		return nil, err
	}
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	c := &Cluster{
		Name:       Prefix + hex.EncodeToString(suffix),
		Dir:        dir,
		Kubeconfig: filepath.Join(dir, "kubeconfig"),
		Kind:       envOr("TELEMACH_KIND", "kind"),
		KubectlBin: envOr("TELEMACH_KUBECTL_BIN", "kubectl"),
	}
	c.Context = "kind-" + c.Name
	cmd := exec.Command(c.Kind, "create", "cluster", "--name", c.Name, "--image", image, //nolint:gosec // binaire de test
		"--kubeconfig", c.Kubeconfig, "--wait", "180s")
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("kind create cluster : %v\n%s", err, out)
	}
	return c, nil
}

// Delete supprime le cluster et son dossier temporaire.
func (c *Cluster) Delete() error {
	cmd := exec.Command(c.Kind, "delete", "cluster", "--name", c.Name, "--kubeconfig", c.Kubeconfig) //nolint:gosec // binaire de test
	out, err := cmd.CombinedOutput()
	_ = os.RemoveAll(c.Dir)
	if err != nil {
		return fmt.Errorf("kind delete cluster : %v\n%s", err, out)
	}
	return nil
}

// Env renvoie l'environnement qui isole kubectl sur ce cluster : KUBECONFIG pointe vers le
// fichier du cluster, jamais vers celui de l'utilisateur.
func (c *Cluster) Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "KUBECONFIG=") {
			env = append(env, kv)
		}
	}
	return append(env, "KUBECONFIG="+c.Kubeconfig)
}

// Kubectl exécute kubectl sur ce cluster, avec --context imposé. Elle refuse tout contexte qui
// ne serait pas celui d'un cluster de test.
func (c *Cluster) Kubectl(args ...string) Result {
	return c.KubectlEnv(nil, args...)
}

// KubectlIn exécute kubectl dans le dossier dir (chemins de -f relatifs à ce dossier).
func (c *Cluster) KubectlIn(dir string, args ...string) Result {
	c.workDir = dir
	defer func() { c.workDir = "" }()
	return c.Kubectl(args...)
}

// KubectlEnv est Kubectl avec des variables d'environnement supplémentaires.
func (c *Cluster) KubectlEnv(extra []string, args ...string) Result {
	if !strings.HasPrefix(c.Context, "kind-"+Prefix) {
		panic("contexte hors cluster de test : " + c.Context)
	}
	// --context est placé après la sous-commande (avant « -- » s'il existe) : placé en tête, il
	// changerait le message de kubectl pour une sous-commande inconnue ou un plugin
	// (« flags cannot be placed before plugin name »).
	full := make([]string, 0, len(args)+2)
	inserted := false
	for _, a := range args {
		if a == "--" && !inserted {
			full = append(full, "--context", c.Context)
			inserted = true
		}
		full = append(full, a)
	}
	if !inserted {
		full = append(full, "--context", c.Context)
	}
	return c.run(extra, full...)
}

// KubectlRaw exécute kubectl sans imposer --context (pour relever les erreurs de contexte).
// Le kubeconfig reste celui du cluster de test.
func (c *Cluster) KubectlRaw(args ...string) Result {
	return c.run(nil, args...)
}

func (c *Cluster) run(extra []string, args ...string) Result {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.KubectlBin, args...) //nolint:gosec // binaire de test
	cmd.Env = append(c.Env(), extra...)
	cmd.Dir = c.workDir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		code = -1
		errb.WriteString(err.Error())
	}
	return Result{Stdout: out.String(), Stderr: errb.String(), ExitCode: code}
}

// Must exécute kubectl et renvoie une erreur si la commande échoue.
func (c *Cluster) Must(args ...string) error {
	if r := c.Kubectl(args...); r.ExitCode != 0 {
		return fmt.Errorf("kubectl %s : code %d\n%s", strings.Join(args, " "), r.ExitCode, r.Stderr)
	}
	return nil
}

// CreateNamespace crée un namespace propre au test et renvoie son nom.
func (c *Cluster) CreateNamespace(base string) (string, error) {
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	ns := base + "-" + hex.EncodeToString(suffix)
	return ns, c.Must("create", "namespace", ns)
}

// ServerURL renvoie l'adresse du serveur API du cluster.
func (c *Cluster) ServerURL() string {
	r := c.Kubectl("config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	return strings.TrimSpace(r.Stdout)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
