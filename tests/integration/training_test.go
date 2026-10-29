//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Scénarios T1 à T13 de specs/004-training-feedback/quickstart.md (feature 004). Comme pour
// la feature 002, les commandes sont extraites du guide lui-même (LL-004) ; seuls les
// namespaces (t1, t3w…), le chemin de testdata/ et le nom des nœuds (telemach-worker…) sont
// adaptés au cluster jetable. T14 (relevé d'une formation) est rejoué par corpus.yml.

const trainingGuide = "../../specs/004-training-feedback/quickstart.md"

// trainingNamespace reconnaît les namespaces du guide : t1, t3w, t13…
var trainingNamespace = regexp.MustCompile(`\bt(\d+w?)\b`)

// trainingBlock renvoie le bloc bash du scénario id, adapté au cluster et à un préfixe de
// namespace propre au shell.
func trainingBlock(t *testing.T, id, prefix string) string {
	t.Helper()
	data, err := os.ReadFile(trainingGuide)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)### " + id + " —.*?```bash\n(.*?)```").FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("scénario %s introuvable dans le guide (D-LL-009)", id)
	}
	testdata, _ := filepath.Abs("../../testdata")
	block := strings.ReplaceAll(m[1], "testdata/", testdata+"/")
	block = strings.ReplaceAll(block, "telemach-worker2", cluster.Worker(2))
	block = strings.ReplaceAll(block, "telemach-worker", cluster.Worker(1))
	return trainingNamespace.ReplaceAllString(block, prefix+"-t$1")
}

// nodesRestored remet les nœuds en état après un scénario qui les modifie, même en cas
// d'échec (taint, cordon, label).
func nodesRestored(t *testing.T) {
	t.Cleanup(func() {
		for _, w := range []string{cluster.Worker(1), cluster.Worker(2)} {
			_ = cluster.Kubectl("uncordon", w)
			_ = cluster.Kubectl("taint", "node", w, "dedie-")
			_ = cluster.Kubectl("label", "node", w, "formation-")
		}
	})
}

// trainingRun exécute un scénario ; interactive le lance dans un shell interactif, puis
// affiche à l'invite les signaux de la vérification différée.
func trainingRun(t *testing.T, shell, id string, interactive bool) (out, prefix string) {
	t.Helper()
	prefix = strings.ToLower(id) + shell[:1]
	block := trainingBlock(t, id, prefix)
	t.Cleanup(func() {
		for _, ns := range trainingNamespace.FindAllString(trainingBlock(t, id, ""), -1) {
			// Suppression attendue : un scénario se rejoue seul (LL-008) ; des pods restants
			// fausseraient le drain de T6.
			_ = cluster.Kubectl("delete", "namespace", prefix+"-"+ns, "--timeout=120s")
		}
	})
	if !interactive {
		stdout, stderr := shellRun(t, shell, block)
		return stdout + stderr, prefix
	}
	body := `eval "$(kubectl-telemach shell-init ` + shell + `)"` + "\n__telemach_prompt 2>/dev/null\n" +
		block + "\nsleep 3\n__telemach_prompt\necho fin"
	stdout, stderr := shellRunInteractiveEnv(t, shell, body, "XDG_RUNTIME_DIR="+t.TempDir())
	if !strings.Contains(stdout, "fin") {
		t.Fatalf("%s interrompu :\n%s\n%s", id, stdout, stderr)
	}
	return stdout + stderr, prefix
}

// T1 à T4 : refus d'admission (US1).
func TestTrainingAdmission(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/T1", func(t *testing.T) {
			out, p := trainingRun(t, shell, "T1", false)
			expect(t, "T1", out, "Le quota « quota-equipe » du namespace est atteint pour limits.cpu",
				"exige que chaque conteneur déclare limits.cpu", "kubectl describe resourcequota -n "+p+"-t1")
		})
		t.Run(shell+"/T2", func(t *testing.T) {
			out, p := trainingRun(t, shell, "T2", false)
			expect(t, "T2", out, "Le LimitRange du namespace refuse ce conteneur", "kubectl describe limitrange -n "+p+"-t2")
		})
		t.Run(shell+"/T3", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T3", false)
			expect(t, "T3", out, "niveau PodSecurity « restricted »", "Version corrigée", "type: RuntimeDefault",
				"il serait refusé si ce niveau devenait obligatoire")
		})
		t.Run(shell+"/T4", func(t *testing.T) {
			out, p := trainingRun(t, shell, "T4", true)
			expect(t, "T4", out, "⚠ Le Deployment api ne peut pas créer ses pods (refus d'admission) : kubectl telemach diagnose deployment/api -n "+p+"-t4",
				"✖ admission", "Le quota « quota-equipe » du namespace est atteint pour limits.cpu")
		})
	}
}

// T5 et T6 : placement et drain (US2), en série : ils modifient les nœuds.
func TestTrainingNodes(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/T5", func(t *testing.T) {
			nodesRestored(t)
			out, _ := trainingRun(t, shell, "T5", false)
			expect(t, "T5", out, "le taint « dedie=gpu:NoSchedule » le repousse", "Nœuds par cause",
				"taint du plan de contrôle (normal)", "nœud non planifiable (cordon)", "elle ne l'y force pas")
		})
		t.Run(shell+"/T6", func(t *testing.T) {
			nodesRestored(t)
			out, _ := trainingRun(t, shell, "T6", false)
			expect(t, "T6", out, "Le drain s'arrête sur des pods de DaemonSet", "volume emptyDir serait effacé",
				"est refusée par un PodDisruptionBudget", "kubectl get pdb -A")
		})
	}
}

// T7 à T9 : Jobs, CronJobs, DaemonSets, StatefulSets, syntaxe cron (US3).
func TestTrainingWorkloads(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/T7", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T7", false)
			expect(t, "T7", out, "Le Job « calc » a abandonné après", "est suspendu (spec.suspend)", "kubectl telemach diagnose job/calc")
		})
		t.Run(shell+"/T8", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T8", false)
			expect(t, "T8", out, "exclu(s) par un taint", "C'est normal", "Le Service headless « base » du StatefulSet « base » n'existe pas")
		})
		t.Run(shell+"/T9", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T9", false)
			expect(t, "T9", out, "Le contrôleur des CronJobs refuserait ce champ", "code: 1", "est refusée par le serveur API")
		})
	}
}

// T10 et T11 : métriques et autoscaling (US4). T11 installe metrics-server dans le cluster
// jetable ; il passe donc après T10.
func TestTrainingMetrics(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/T10", func(t *testing.T) {
			if metricsInstalled() {
				t.Skip("metrics-server déjà installé par T11 : T10 suppose son absence")
			}
			out, p := trainingRun(t, shell, "T10", true)
			expect(t, "T10", out, "L'API de métriques ne répond pas",
				"⚠ Le HPA web ne reçoit aucune métrique (metrics-server absent) : kubectl telemach diagnose hpa/web -n "+p+"-t10",
				"Le HPA « web » ne reçoit aucune métrique")
		})
	}
	for _, shell := range shells {
		t.Run(shell+"/T11", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T11", false)
			expect(t, "T11", out, "Le HPA « web » est à sa borne", "Ce n'est pas une panne")
		})
	}
}

// metricsInstalled indique si l'API de métriques est déclarée dans le cluster de test.
func metricsInstalled() bool {
	return cluster.Kubectl("get", "apiservice", "v1beta1.metrics.k8s.io").ExitCode == 0
}

// T12 et T13 : rendu sur l'entrée standard (US5), wait et rollout undo (US6).
func TestTrainingStdinAndErrors(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/T12", func(t *testing.T) {
			out, _ := trainingRun(t, shell, "T12", false)
			expect(t, "T12", out, "<stdin>:36:26", "template telemarche/templates/api.yaml", "code: 1", "code: 0",
				"L'entrée standard est vide", "code: 2")
		})
		t.Run(shell+"/T13", func(t *testing.T) {
			out, p := trainingRun(t, shell, "T13", false)
			expect(t, "T13", out, "n'a pas atteint l'état attendu", "kubectl telemach diagnose pod/web -n "+p+"-t13",
				"La révision 5 n'existe pas", "kubectl rollout history deployment/api -n "+p+"-t13")
		})
	}
}
