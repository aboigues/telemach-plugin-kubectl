//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Scénarios du quickstart que us1_test.go et us4_test.go ne couvrent pas (T087) : rejoués
// sur chaque image kind de la matrice CI, sous bash et zsh, avec l'intégration shell.

const manifests = "../../testdata/manifests"

// shells renvoie les shells à tester.
var shells = []string{"bash", "zsh"}

// expectAll signale chaque fragment attendu absent de got.
func expectAll(t *testing.T, label, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s : sortie sans %q :\n%s", label, w, got)
		}
	}
}

// kubeconfigSecrets renvoie les valeurs de certificats et de clés du kubeconfig de test, qui
// ne doivent jamais apparaître dans une explication (V3).
func kubeconfigSecrets(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(cluster.Kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*(?:client-key-data|client-certificate-data|certificate-authority-data|token):\s*(\S{16,})`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		// Extrait pris au milieu : tous les blocs PEM encodés commencent de la même façon.
		mid := len(m[1]) / 2
		out = append(out, m[1][mid-8:mid+8])
	}
	if len(out) == 0 {
		t.Fatal("kubeconfig de test sans certificat ni clé : rien à comparer (D-LL-009)")
	}
	return out
}

// V3 : contexte inexistant et serveur API injoignable ; aucun contenu du kubeconfig affiché.
func TestQuickstartV3(t *testing.T) {
	secrets := kubeconfigSecrets(t)
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			stdout, stderr := shellRun(t, shell, `kubectl --context inexistant get pods; echo "rc=$?"`)
			expectAll(t, "contexte", stdout+stderr, "rc=1", "Le contexte « inexistant » n'existe pas", "kubectl config get-contexts")
			// Port fermé sur la machine : connexion refusée, sans toucher au cluster de test.
			stdout, stderr2 := shellRun(t, shell, `kubectl --server=https://127.0.0.1:1 get pods; echo "rc=$?"`)
			expectAll(t, "injoignable", stdout+stderr2, "rc=1", "Le serveur API 127.0.0.1:1 refuse la connexion", "kubectl config get-contexts")
			for _, s := range secrets {
				if strings.Contains(stderr+stderr2, s) {
					t.Error("contenu du kubeconfig affiché (FR-018)")
				}
			}
		})
	}
}

// V6 : erreur inventée, binaire appelé directement → aucune cause supposée.
func TestQuickstartV6(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			_, stderr := shellRun(t, shell, `kubectl-telemach explain --exit-code 1 --stderr-file <(echo "error: something totally new") -- get pods`)
			expectAll(t, "V6", stderr, "✖ Erreur non reconnue", "préfère ne pas deviner")
			if strings.Contains(stderr, "🔎 Cause probable") {
				t.Errorf("cause inventée pour une erreur inconnue (FR-008) :\n%s", stderr)
			}
		})
	}
}

// V7, V8, V9 : manifestes refusés par le vrai kubectl ou le vrai serveur API, localisés à la
// ligne près dans le fichier de l'apprenant.
func TestQuickstartManifests(t *testing.T) {
	cases := []struct {
		name, dir, file string
		want            []string
	}{
		{"V7", "tab-indent", "deploy.yaml", []string{"deploy.yaml:18:1", "tabulation"}},
		{"V8", "unknown-field-multidoc", "all.yaml", []string{"all.yaml:24:3 (document 3, Deployment/web)", "replicas"}},
		{"V9", "selector-mismatch", "deploy.yaml", []string{"deploy.yaml:9:7", "spec.template.metadata.labels"}},
	}
	for _, shell := range shells {
		for _, tc := range cases {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				ns := namespace(t, "qs-"+strings.ToLower(tc.name)+"-"+shell)
				dir, _ := filepath.Abs(filepath.Join(manifests, tc.dir))
				stdout, stderr := shellRun(t, shell, `cd `+dir+` && kubectl apply -f `+tc.file+` -n `+ns+`; echo "rc=$?"`)
				expectAll(t, tc.name, stdout+stderr, append([]string{"rc=1", "💡 Version corrigée"}, tc.want...)...)
			})
		}
	}
}

// V10, V11 : check sans cluster (KUBECONFIG invalide), secret masqué partout, sans -f.
func TestQuickstartCheck(t *testing.T) {
	secret, _ := filepath.Abs(filepath.Join(manifests, "plaintext-secret", "secret.yaml"))
	valid, _ := filepath.Abs(filepath.Join(manifests, "valid"))
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			state := t.TempDir()
			env := []string{"XDG_STATE_HOME=" + state, "KUBECONFIG=/nonexistent"}
			stdout, stderr := shellRun(t, shell, `kubectl telemach check -f `+secret+`; echo "rc=$?"`, env...)
			expectAll(t, "V10", stdout+stderr, "rc=1", "password: ••••••", "secretKeyRef")
			saved, err := os.ReadFile(filepath.Join(state, "telemach-kubectl", "last.json"))
			if err != nil {
				t.Fatalf("V10 : last.json absent : %v", err)
			}
			if strings.Contains(stderr+string(saved), "s3cret-fake") {
				t.Error("V10 : secret en clair dans la sortie ou last.json (SC-006)")
			}
			stdout, stderr = shellRun(t, shell, `kubectl telemach check -f `+valid+`; echo "rc=$?"`+"\n"+`kubectl telemach check; echo "rc2=$?"`, env...)
			expectAll(t, "V11", stdout+stderr, "rc=0", "Règles Kubernetes 1.3", "✔ Aucun problème détecté", "rc2=2", "Indique les manifestes à vérifier avec -f")
		})
	}
}

// V12 et V17 : répétition d'une erreur (rendu court), last (rendu complet), deepen et exercice.
func TestQuickstartRepeatLastDeepen(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			body := `kubectl get pdos 2>/dev/null; kubectl get pdos; echo "--- last"; kubectl telemach last` +
				"\n" + `echo "--- deepen"; kubectl telemach deepen; kubectl telemach deepen --exercise`
			stdout, stderr := shellRun(t, shell, body)
			expectAll(t, "V17", stderr, "↺ Même erreur que précédemment", "kubectl telemach last")
			expectAll(t, "V17 last", stderr, "📘 Concept — resource type", "kubectl get pods")
			expectAll(t, "V12", stderr, "🧪 Exercice", "kubectl create namespace")
			if !strings.Contains(stdout, "--- last") || !strings.Contains(stdout, "--- deepen") {
				t.Errorf("scénario interrompu :\n%s", stdout)
			}
		})
	}
}

// V15 (seconde moitié) : sans alias « k » préalable, l'intégration ne le crée pas (FR-001a).
func TestQuickstartNoKAlias(t *testing.T) {
	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			if _, err := lookPath(shell); err != nil {
				t.Skip(err)
			}
			body := `unalias k 2>/dev/null; eval "$(kubectl-telemach shell-init ` + shell + `)"; alias k 2>/dev/null || echo "k non créé"`
			stdout, stderr := shellRunInteractive(t, shell, body)
			if !strings.Contains(stdout, "k non créé") {
				t.Errorf("alias k créé :\n%s\n%s", stdout, stderr)
			}
		})
	}
}
