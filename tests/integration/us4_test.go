//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const scenarios = "../../testdata/pods/scenarios"

// applyScenarios applique des scénarios de pods en échec dans un namespace de test.
func applyScenarios(t *testing.T, ns string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := cluster.Must("apply", "-n", ns, "-f", filepath.Join(scenarios, n+".yaml")); err != nil {
			t.Fatal(err)
		}
	}
}

// TestUS4Diagnose rejoue V13 : diagnostic du namespace puis de chaque pod, sans aucune
// modification du cluster (SC-008), en moins de 5 s par pod (SC-009).
func TestUS4Diagnose(t *testing.T) {
	ns := namespace(t, "us4")
	applyScenarios(t, ns, "bad-image", "crash", "missing-secret", "healthy")
	time.Sleep(45 * time.Second)
	before := snapshot(t, ns)

	stdout, stderr := shellRun(t, "bash", "kubectl telemach diagnose -n "+ns+`; echo "rc=$?"`)
	if !strings.Contains(stdout, "rc=0") || !strings.Contains(stderr, "Autres pods en difficulté") {
		t.Errorf("diagnose du namespace :\n%s\n%s", stdout, stderr)
	}
	for pod, want := range map[string]string{
		"bad-image":      "ne peut pas être téléchargée",
		"crash":          "redémarre en boucle",
		"missing-secret": "« api-absent » introuvable",
	} {
		start := time.Now()
		_, stderr := shellRun(t, "bash", "kubectl telemach diagnose pod/"+pod+" -n "+ns)
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s : diagnostic en %v > 5 s (SC-009)", pod, d)
		}
		if !strings.Contains(stderr, want) {
			t.Errorf("%s : diagnostic sans %q :\n%s", pod, want, stderr)
		}
		if strings.Contains(stderr, "s3cret-fake") {
			t.Errorf("%s : valeur d'environnement affichée", pod)
		}
	}
	_, stderr = shellRun(t, "bash", "kubectl telemach diagnose pod/healthy -n "+ns)
	if !strings.Contains(stderr, "n'est pas en difficulté") {
		t.Errorf("pod sain :\n%s", stderr)
	}
	_, stderr = shellRun(t, "bash", "kubectl telemach diagnose pod/missing-secret -n "+ns+" --as=system:serviceaccount:"+ns+":default")
	if !strings.Contains(stderr, "Le diagnostic n'a pas pu lire le cluster") && !strings.Contains(stderr, "Diagnostic partiel") {
		t.Errorf("lecture refusée : diagnostic partiel ou explication RBAC attendus :\n%s", stderr)
	}
	if after := snapshot(t, ns); after != before {
		t.Errorf("le namespace a été modifié par diagnose (SC-008) :\navant :\n%s\naprès :\n%s", before, after)
	}
}

// TestUS4Watch rejoue V14 : signal à l'invite suivante pour un pod en échec, rien pour un pod
// sain ni avec TELEMACH_WATCH=0 (SC-010).
func TestUS4Watch(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			if _, err := lookPath(shell); err != nil {
				t.Skip(err)
			}
			ns := namespace(t, "us4-"+shell)
			abs, _ := filepath.Abs(scenarios)
			runtime := t.TempDir()
			body := `eval "$(kubectl-telemach shell-init ` + shell + `)"` + "\n" +
				`kubectl apply -n ` + ns + ` -f ` + abs + `/bad-image.yaml >/dev/null` + "\n" +
				`kubectl apply -n ` + ns + ` -f ` + abs + `/healthy.yaml >/dev/null` + "\n" +
				`TELEMACH_WATCH=0 kubectl apply -n ` + ns + ` -f ` + abs + `/bad-image-2.yaml >/dev/null` + "\n" +
				`sleep 25` + "\n" + `__telemach_prompt` + "\n" + `echo fin`
			stdout, stderr := shellRunInteractiveEnv(t, shell, body, "XDG_RUNTIME_DIR="+runtime)
			if !strings.Contains(stdout, "fin") {
				t.Fatalf("scénario interrompu :\n%s\n%s", stdout, stderr)
			}
			if !strings.Contains(stderr, "⚠ Le pod bad-image est en ") || !strings.Contains(stderr, "kubectl telemach diagnose pod/bad-image -n "+ns) {
				t.Errorf("signal attendu pour bad-image :\n%s", stderr)
			}
			for _, absent := range []string{"pod healthy", "pod bad-image-2"} {
				if strings.Contains(stderr, absent) {
					t.Errorf("signal inattendu (%s) :\n%s", absent, stderr)
				}
			}
			entries, _ := os.ReadDir(filepath.Join(runtime, "telemach-kubectl"))
			if len(entries) != 0 {
				t.Errorf("le fichier de signaux doit être supprimé après affichage : %v", entries)
			}
		})
	}
}
