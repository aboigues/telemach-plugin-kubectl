//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	binDir    string
	buildErr  error
)

// toolsDir construit kubectl-telemach une seule fois et prépare un dossier de binaires : le
// plugin et un lien « kubectl » vers le kubectl de test. C'est ce dossier qui est placé dans
// le PATH, comme après une installation réelle (D-LL-006).
func toolsDir(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binDir, buildErr = os.MkdirTemp("", "telemach-bin-")
		if buildErr != nil {
			return
		}
		cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, "kubectl-telemach"), "../../cmd/kubectl-telemach")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err: err, out: string(out)}
			return
		}
		kubectl, err := exec.LookPath(cluster.KubectlBin)
		if err != nil {
			buildErr = err
			return
		}
		buildErr = os.Symlink(kubectl, filepath.Join(binDir, "kubectl"))
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binDir
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

// shellRun évalue l'intégration shell (installée comme dans le README) puis exécute body.
// KUBECONFIG pointe vers le cluster de test : aucune commande ne peut viser un autre cluster.
func shellRun(t *testing.T, shell, body string, extraEnv ...string) (stdout, stderr string) {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s non installé", shell)
	}
	bin := toolsDir(t)
	script := filepath.Join(t.TempDir(), "body.sh")
	full := `eval "$(kubectl-telemach shell-init ` + shell + `)"` + "\n" + body + "\n"
	if err := os.WriteFile(script, []byte(full), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shell, script) //nolint:gosec // test
	cmd.Env = append([]string{
		"PATH=" + bin + ":/usr/bin:/bin",
		"HOME=" + t.TempDir(),
		"XDG_STATE_HOME=" + t.TempDir(),
		"XDG_RUNTIME_DIR=" + t.TempDir(),
		"KUBECONFIG=" + cluster.Kubeconfig,
		"NO_COLOR=1",
	}, extraEnv...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return out.String(), errb.String()
}

// snapshot renvoie l'état comparable des objets d'un namespace (champs volatils retirés),
// pour vérifier que TELEMACH n'a rien modifié (SC-008 ; feature 002 : SC-005, PVC, Ingress et
// EndpointSlices compris).
func snapshot(t *testing.T, ns string) string {
	t.Helper()
	r := cluster.Kubectl("get", "all,configmaps,serviceaccounts,pvc,ingress,endpointslices", "-n", ns, "-o", "json")
	if r.ExitCode != 0 {
		t.Fatalf("snapshot : %s", r.Stderr)
	}
	var list struct {
		Items []struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name            string `json:"name"`
				ResourceVersion string `json:"resourceVersion"`
				Generation      int    `json:"generation"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(r.Stdout), &list); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, it := range list.Items {
		switch it.Kind {
		case "EndpointSlice", "PersistentVolumeClaim":
			// Réécrits par les contrôleurs (endpoints d'un pod qui devient prêt, liaison d'un
			// volume), génération comprise : seule leur présence compte.
			lines = append(lines, it.Kind+"/"+it.Metadata.Name)
		case "Pod", "ReplicaSet", "Deployment":
			// Leur statut change seul (sondes, redémarrages) : seuls leur présence et leur
			// génération comptent.
			lines = append(lines, it.Kind+"/"+it.Metadata.Name+"#"+itoa(it.Metadata.Generation))
		default:
			lines = append(lines, it.Kind+"/"+it.Metadata.Name+"@"+it.Metadata.ResourceVersion)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func lookPath(name string) (string, error) { return exec.LookPath(name) }

// shellRunInteractive exécute body (qui évalue lui-même l'intégration) dans une session
// interactive sans fichiers de démarrage : seule une session interactive développe les alias.
func shellRunInteractive(t *testing.T, shell, body string) (stdout, stderr string) {
	t.Helper()
	return shellRunInteractiveEnv(t, shell, body)
}

// shellRunInteractiveEnv est shellRunInteractive avec des variables supplémentaires (les
// dernières l'emportent).
func shellRunInteractiveEnv(t *testing.T, shell, body string, extra ...string) (stdout, stderr string) {
	t.Helper()
	bin := toolsDir(t)
	script := filepath.Join(t.TempDir(), "body.sh")
	if err := os.WriteFile(script, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--norc", "-i"}
	if shell == "zsh" {
		flags = []string{"-f", "-i"}
	}
	cmd := exec.Command(shell, append(flags, script)...) //nolint:gosec // test
	cmd.Env = []string{
		"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir(),
		"XDG_RUNTIME_DIR=" + t.TempDir(), "KUBECONFIG=" + cluster.Kubeconfig, "NO_COLOR=1",
	}
	cmd.Env = append(cmd.Env, extra...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	return out.String(), errb.String()
}
