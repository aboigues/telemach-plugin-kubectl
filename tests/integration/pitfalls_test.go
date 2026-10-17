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
