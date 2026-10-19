//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Scénarios W1 à W14 de specs/002-silent-pitfalls/quickstart.md. Les commandes sont extraites
// du guide lui-même (bloc bash de chaque scénario) : le guide et le test ne peuvent pas
// diverger (LL-004). Seuls le namespace (« demo ») et le chemin de testdata/ sont adaptés.

const pitfallsGuide = "../../specs/002-silent-pitfalls/quickstart.md"

// quickstartBlock renvoie le premier bloc bash du scénario id (« W1 ») du guide, adapté au
// namespace ns et au chemin absolu de testdata/.
func quickstartBlock(t *testing.T, id, ns string) string {
	t.Helper()
	data, err := os.ReadFile(pitfallsGuide)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)### " + id + " —.*?```bash\n(.*?)```")
	m := re.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("scénario %s introuvable dans le guide (D-LL-009)", id)
	}
	testdata, _ := filepath.Abs("../../testdata")
	block := strings.ReplaceAll(m[1], "testdata/", testdata+"/")
	block = regexp.MustCompile(`(-n |-n=)demo\b`).ReplaceAllString(block, "${1}"+ns)
	block = strings.ReplaceAll(block, ".demo", "."+ns)
	return block
}

// diagnoseLines garde les seules commandes « kubectl telemach diagnose » d'un bloc.
func diagnoseLines(block string) string {
	var out []string
	for _, l := range strings.Split(block, "\n") {
		if strings.Contains(l, "telemach diagnose") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// runScenario exécute un scénario du guide, puis rejoue ses seuls diagnostics en vérifiant
// que l'état du namespace est identique avant et après (SC-005).
func runScenario(t *testing.T, shell, id string) (stdout, stderr, ns string) {
	t.Helper()
	ns = namespace(t, strings.ToLower(id)+"-"+shell)
	block := quickstartBlock(t, id, ns)
	stdout, stderr = shellRun(t, shell, block)
	before := snapshot(t, ns)
	_, _ = shellRun(t, shell, diagnoseLines(block))
	if after := snapshot(t, ns); after != before {
		t.Errorf("%s : le namespace a été modifié par diagnose (SC-005) :\navant :\n%s\naprès :\n%s", id, before, after)
	}
	return stdout, stderr, ns
}

func expect(t *testing.T, id, out string, want ...string) {
	t.Helper()
	// On compare le texte, pas la mise en page : le rendu coupe les paragraphes à la largeur du
	// terminal et colle les guillemets à leur contenu par une espace insécable (typographie
	// française) ; strings.Fields traite les deux comme des espaces.
	out = strings.Join(strings.Fields(out), " ")
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("%s : sortie sans %q :\n%s", id, w, out)
		}
	}
}

// W1 à W4 : diagnostic d'un Service (US1).
func TestPitfallsService(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/W1", func(t *testing.T) {
			stdout, stderr, ns := runScenario(t, shell, "W1")
			expect(t, "W1", stdout+stderr, "code: 0", "✖ selector", "app=frontend", "kubectl get pods --show-labels -n "+ns)
			if strings.Contains(stderr, "get endpoints ") {
				t.Error("W1 : l'API Endpoints dépréciée ne doit pas être proposée (FR-032)")
			}
		})
		t.Run(shell+"/W2", func(t *testing.T) {
			_, stderr, ns := runScenario(t, shell, "W2")
			expect(t, "W2", stderr, "aucun n'est prêt", "kubectl telemach diagnose pod/api-", "-n "+ns)
		})
		t.Run(shell+"/W3", func(t *testing.T) {
			_, stderr, _ := runScenario(t, shell, "W3")
			expect(t, "W3", stderr, "« http », qu'aucun conteneur ne déclare", "port 8081", "La déclaration des ports est facultative", "ne déclarent aucun port")
		})
		t.Run(shell+"/W4", func(t *testing.T) {
			_, stderr, ns := runScenario(t, shell, "W4")
			expect(t, "W4", stderr, "n'a pas de selector", "✔ Le Service « hello »", "hello."+ns+".svc.cluster.local", "kubectl port-forward service/hello")
		})
	}
}

// W5 à W7 : rollout bloqué, tag mutable, configuration figée (US2).
func TestPitfallsDeployment(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/W5", func(t *testing.T) {
			_, stderr, ns := runScenario(t, shell, "W5")
			expect(t, "W5", stderr, "est bloqué", "1.30-alpine", "kubectl rollout undo deployment/stalled -n "+ns)
			// Le pod alterne entre ErrImagePull et ImagePullBackOff au fil des nouvelles tentatives.
			if !strings.Contains(stderr, "ImagePullBackOff") && !strings.Contains(stderr, "ErrImagePull") {
				t.Errorf("W5 : sortie sans l'état du pull d'image :\n%s", stderr)
			}
		})
		t.Run(shell+"/W6", func(t *testing.T) {
			_, stderr, _ := runScenario(t, shell, "W6")
			expect(t, "W6", stderr, "ℹ L'image « nginxinc/nginx-unprivileged » n'a pas de tag explicite")
		})
		t.Run(shell+"/W7", func(t *testing.T) {
			_, stderr, ns := runScenario(t, shell, "W7")
			expect(t, "W7", stderr, "La ConfigMap « conf-settings » a changé", "kubectl rollout restart deployment/conf -n "+ns, "Secret « conf-secret »")
			for _, leak := range []string{"MODE=prod", ": prod", "fake-token-value"} {
				if strings.Contains(stderr, leak) {
					t.Errorf("W7 : valeur de ConfigMap ou de Secret affichée (%q)", leak)
				}
			}
		})
	}
}

// W8 à W10 : Ingress, PVC, diagnostic sans argument et nom ambigu (US3).
func TestPitfallsIngressPVC(t *testing.T) {
	// La classe « demo » vaut pour tout le cluster : W8 la supprime, y compris en cas d'échec.
	t.Cleanup(func() { _ = cluster.Kubectl("delete", "ingressclass", "demo", "--ignore-not-found") })
	for _, shell := range shells {
		t.Run(shell+"/W8", func(t *testing.T) {
			_, stderr, ns := runScenario(t, shell, "W8")
			expect(t, "W8", stderr,
				"L'Ingress « web » existe, mais aucun contrôleur d'Ingress n'est installé",
				"La règle « / » de l'Ingress « shop » vise le Service « absent », introuvable",
				"kubectl get services -n "+ns,
				"L'Ingress « shop » n'a pas d'adresse",
				"L'Ingress « web » ne demande aucune classe, et aucune classe n'est marquée par défaut")
			for _, name := range []string{"nginx", "traefik", "haproxy"} {
				if strings.Contains(strings.ToLower(stderr), name) {
					t.Errorf("W8 : un contrôleur est nommé (%s), FR-012", name)
				}
			}
		})
		t.Run(shell+"/W9", func(t *testing.T) {
			_, stderr, _ := runScenario(t, shell, "W9")
			expect(t, "W9", stderr,
				"Le PVC « fast-data » demande la classe « rapide », qui n'existe pas",
				"standard (par défaut)",
				"Le PVC « data » attend son premier pod, c'est normal")
		})
		t.Run(shell+"/W10", func(t *testing.T) {
			ns := namespace(t, "w10-"+shell)
			testdata, _ := filepath.Abs("../../testdata")
			// Namespace des scénarios précédents, plus un pod en échec (nouvelle image introuvable).
			prep := quickstartBlock(t, "W4", ns) + quickstartBlock(t, "W8", ns) + quickstartBlock(t, "W9", ns) +
				"kubectl apply -n " + ns + " -f " + testdata + "/objects/scenarios/rollout-stalled.yaml\n" +
				"kubectl wait -n " + ns + " --for=condition=Available deployment/stalled --timeout=90s\n" +
				"kubectl set image -n " + ns + " deployment/stalled app=nginxinc/nginx-unprivileged:0.0.0-inexistant\n" +
				"sleep 20\n"
			_, _ = shellRun(t, shell, prep)
			before := snapshot(t, ns)
			stdout, stderr := shellRun(t, shell, quickstartBlock(t, "W10", ns))
			if after := snapshot(t, ns); after != before {
				t.Errorf("W10 : le namespace a été modifié par diagnose (SC-005)")
			}
			out := stdout + stderr
			expect(t, "W10", out,
				"stalled-", "Autres objets en difficulté",
				"kubectl telemach diagnose ingress/web -n "+ns,
				"kubectl telemach diagnose ingress/shop -n "+ns,
				"kubectl telemach diagnose pvc/fast-data -n "+ns,
				"service/hello", "deployment/hello", "code: 2")
			if strings.Contains(out, "diagnose pvc/data ") {
				t.Error("W10 : un PVC en attente normale ne doit pas être listé (FR-014)")
			}
		})
	}
}

// W11 : signal à l'invite suivante après une création réussie (US4) : Service sans pod,
// targetPort nommé introuvable après expose ; rien pour un Service créé avec ses pods
// (transitoire) ni avec TELEMACH_WATCH=0.
func TestPitfallsWatch(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/W11", func(t *testing.T) {
			ns := namespace(t, "w11-"+shell)
			testdata, _ := filepath.Abs("../../testdata")
			runtime := t.TempDir()
			body := `eval "$(kubectl-telemach shell-init ` + shell + `)"` + "\n" +
				// Le Deployment « hello » qu'exposera le scénario.
				"kubectl apply -n " + ns + " -f " + testdata + "/objects/scenarios/healthy.yaml >/dev/null\n" +
				"kubectl wait -n " + ns + " --for=condition=Available deployment/hello --timeout=90s >/dev/null\n" +
				"__telemach_prompt 2>/dev/null\n" +
				quickstartBlock(t, "W11", ns) + "\n" +
				"sleep 15\n__telemach_prompt\necho fin"
			stdout, stderr := shellRunInteractiveEnv(t, shell, body, "XDG_RUNTIME_DIR="+runtime)
			if !strings.Contains(stdout, "fin") {
				t.Fatalf("scénario interrompu :\n%s\n%s", stdout, stderr)
			}
			expect(t, "W11", stderr,
				"⚠ Le Service orphan ne désigne aucun pod : kubectl telemach diagnose service/orphan -n "+ns,
				"⚠ Le Service hello-typo vise le port nommé « web », qu'aucun pod ne déclare : kubectl telemach diagnose service/hello-typo -n "+ns)
			for _, absent := range []string{"together", "orphan-2"} {
				if strings.Contains(stderr, "Service "+absent+" ") {
					t.Errorf("W11 : signal inattendu pour %s :\n%s", absent, stderr)
				}
			}
		})
	}
}

// W12 et W13 : nouvelles erreurs du catalogue et DNS d'un nom court (US5).
func TestPitfallsNewErrors(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/W12", func(t *testing.T) {
			stdout, stderr, ns := runScenario(t, shell, "W12")
			out := stdout + stderr
			expect(t, "W12", out,
				"n'a pas de conteneur « helo »", "-c hello",
				"n'a pas de conteneur « trois »", "Ses conteneurs s'appellent : app, sidecar",
				"Le port 18080 de ta machine est déjà utilisé", "port-forward -n "+ns+" service/hello :80",
				"Le déploiement de « stalled » est bloqué", "kubectl telemach diagnose deployment/stalled -n "+ns,
				"Le champ spec.selector de « hello » (Deployment) ne peut pas être modifié", "Seconde voie")
			if n := strings.Count(out, "code: 1"); n < 4 {
				t.Errorf("W12 : codes de sortie d'origine non préservés (%d « code: 1 ») :\n%s", n, stdout)
			}
			// Aucune suppression n'est exécutée : le Deployment existe toujours.
			if r := cluster.Kubectl("get", "deployment", "hello", "-n", ns); r.ExitCode != 0 {
				t.Errorf("W12 : le Deployment hello a disparu : %s", r.Stderr)
			}
		})
		t.Run(shell+"/W13", func(t *testing.T) {
			_, stderr, _ := runScenario(t, shell, "W13")
			expect(t, "W13", stderr,
				"Le programme du pod « dns-client » n'a pas pu résoudre le nom « kubernetes »",
				"kubernetes.NAMESPACE.svc.cluster.local",
				"kubectl get services -A --field-selector metadata.name=kubernetes")
		})
	}
}

// W14 : lecture refusée par RBAC → diagnostic partiel, sans cause affirmée sur ce qui n'a pas
// été lu (FR-020). L'identité empruntée ne peut lire que les Services : le Service est trouvé,
// ses pods et EndpointSlices non. L'état du namespace avant et après chaque diagnose et watch
// est vérifié par les tests de W1 à W13 (runScenario, SC-005).
func TestPitfallsDenied(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell+"/W14", func(t *testing.T) {
			ns := namespace(t, "w14-"+shell)
			testdata, _ := filepath.Abs("../../testdata")
			// Préparation du test (pas une commande de l'apprenant) : droits de lecture des seuls
			// Services pour le compte de service par défaut.
			for _, args := range [][]string{
				{"apply", "-n", ns, "-f", testdata + "/objects/scenarios/healthy.yaml"},
				{"create", "role", "services-only", "--verb=get", "--resource=services", "-n", ns},
				{"create", "rolebinding", "services-only", "--role=services-only", "--serviceaccount=" + ns + ":default", "-n", ns},
				{"wait", "-n", ns, "--for=condition=Available", "deployment/hello", "--timeout=90s"},
			} {
				if err := cluster.Must(args...); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot(t, ns)
			_, stderr := shellRun(t, shell, "kubectl telemach diagnose service/hello -n "+ns+" --as=system:serviceaccount:"+ns+":default")
			expect(t, "W14", stderr, "◐ Diagnostic partiel", "Lecture refusée par RBAC")
			for _, claim := range []string{"ne désigne aucun pod", "aucun n'est prêt"} {
				if strings.Contains(stderr, claim) {
					t.Errorf("W14 : cause affirmée sur une information non lue (%q) :\n%s", claim, stderr)
				}
			}
			if after := snapshot(t, ns); after != before {
				t.Errorf("W14 : le namespace a été modifié par diagnose (SC-005)")
			}
		})
	}
}
