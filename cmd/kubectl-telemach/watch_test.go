package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// objectCluster est un faux kubectl qui rejoue un cas de testdata/objects (FAKE_CASE) et
// journalise ses appels. FAKE_MAIN est le nom de l'objet principal du cas (fichier
// <type>.json, « * » pour tous les noms) ; FAKE_DENY refuse les lectures d'un type ; FAKE_MODE=refused simule un
// cluster injoignable.
const objectCluster = `#!/bin/sh
echo "$*" >> "$CALL_LOG"
if [ "$FAKE_MODE" = refused ]; then echo 'The connection to the server localhost:8080 was refused' >&2; exit 1; fi
if [ -n "$FAKE_DENY" ] && [ "$2" = "$FAKE_DENY" ]; then echo "Error from server (Forbidden): $2 is forbidden" >&2; exit 1; fi
serve() { if [ -f "$FAKE_CASE/$1" ]; then cat "$FAKE_CASE/$1"; else echo "Error from server (NotFound): $1 not found" >&2; exit 1; fi; }
case "$1 $2" in
"get service")
  if [ "$3" = "$FAKE_MAIN" ] || [ "$FAKE_MAIN" = "*" ]; then serve service.json; else serve "service-$3.json"; fi ;;
"get endpointslices") serve endpointslices.json ;;
"get pods")
  if [ "$3" = "-l" ]; then serve selected-pods.json; else serve pods.json; fi ;;
"get pod") serve "pod-$3.json" ;;
"get ingress") serve ingress.json ;;
"get ingressclasses") serve ingressclasses.json ;;
"get pvc") serve pvc.json ;;
"get storageclasses") serve storageclasses.json ;;
"get events") echo '{"items":[]}' ;;
"logs "*) ;;
*) echo "appel inattendu : $*" >&2; exit 99 ;;
esac
`

const objectsCorpus = "../../testdata/objects"

// watchEnv installe le faux kubectl sur un cas du corpus d'objets ; il renvoie l'application,
// le journal des appels et une fonction qui lit les signaux déposés.
func watchEnv(t *testing.T, corpus, main string) (*app, string, func() string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte(objectCluster), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(filepath.Join(objectsCorpus, corpus))
	log := filepath.Join(dir, "calls.log")
	t.Setenv("CALL_LOG", log)
	t.Setenv("FAKE_CASE", abs)
	t.Setenv("FAKE_MAIN", main)
	runtime := t.TempDir()
	env := map[string]string{"TELEMACH_KUBECTL": bin, "XDG_RUNTIME_DIR": runtime}
	a, out, errb := newTestApp(t, env)
	at, _ := os.ReadFile(filepath.Join(abs, "captured-at.txt")) //nolint:gosec // corpus du dépôt
	when, _ := time.Parse(time.RFC3339, strings.TrimSpace(string(at)))
	a.now = func() time.Time { return when }
	t.Cleanup(func() {
		if out.Len() != 0 || errb.Len() != 0 {
			t.Errorf("watch ne doit rien écrire sur le terminal : %q %q", out, errb)
		}
	})
	signals := func() string {
		data, _ := os.ReadFile(state.PendingFile(filepath.Join(runtime, "telemach-kubectl"), os.Getpid())) //nolint:gosec // fichier de test
		return string(data)
	}
	return a, log, signals
}

// manifestFile écrit un manifeste local dans un dossier temporaire.
func manifestFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runWatch(a *app, args ...string) {
	run(a, append([]string{"watch", "--delay", "0", "--shell-pid", itoa(os.Getpid()), "--"}, args...))
}

const (
	serviceWeb  = "apiVersion: v1\nkind: Service\nmetadata:\n  name: web\nspec:\n  selector:\n    app: web\n  ports:\n  - port: 80\n"
	deployWeb   = "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  selector:\n    matchLabels:\n      app: web\n  template:\n    metadata:\n      labels:\n        app: web\n    spec:\n      containers:\n      - name: web\n        image: nginx:1.29\n"
	ingressWeb  = "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: web\n"
	pvcTemplate = "apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: %s\n"
)

// T048 : une ligne par piège non transitoire au format de contracts/cli.md § watch ; rien pour
// un objet sain (SC-002), un piège transitoire (FR-024) ou une lecture échouée.
func TestWatchObjects(t *testing.T) {
	tests := []struct {
		name, corpus, main string
		args               []string
		want               string
	}{
		{
			"Service sans pod", "selector-typo", "web",
			[]string{"apply", "-f", serviceWeb, "-n", "demo"},
			"⚠ Le Service web ne désigne aucun pod : kubectl telemach diagnose service/web -n demo\n",
		},
		{"Service et Deployment ensemble", "selector-typo", "web", []string{"apply", "-f", serviceWeb + "---\n" + deployWeb, "-n", "demo"}, ""},
		{
			"targetPort nommé", "targetport-name", "shop",
			[]string{"expose", "deployment", "shop", "--port=80", "-n", "demo"},
			"⚠ Le Service shop vise le port nommé « http », qu'aucun pod ne déclare : kubectl telemach diagnose service/shop -n demo\n",
		},
		{"Service sain", "healthy", "hello", []string{"expose", "deployment", "hello", "--port=80", "-n", "demo"}, ""},
		{
			"Ingress sans classe", "ingress-no-class", "",
			[]string{"apply", "-f", ingressWeb, "-n", "demo"},
			"⚠ L'Ingress web n'a aucune classe prise en charge : kubectl telemach diagnose ingress/web -n demo\n",
		},
		{
			"PVC à classe inconnue", "pvc-unknown-class", "",
			[]string{"apply", "-f", fmt.Sprintf(pvcTemplate, "fast-data"), "-n", "demo"},
			"⚠ Le PVC fast-data demande la classe « rapide », qui n'existe pas : kubectl telemach diagnose pvc/fast-data -n demo\n",
		},
		{"PVC en attente normale", "pvc-waiting", "", []string{"apply", "-f", fmt.Sprintf(pvcTemplate, "data"), "-n", "demo"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _, signals := watchEnv(t, tt.corpus, tt.main)
			args := append([]string(nil), tt.args...)
			if args[0] == "apply" {
				args[2] = manifestFile(t, args[2])
			}
			runWatch(a, args...)
			if got := signals(); got != tt.want {
				t.Errorf("signaux :\n%q\nattendu :\n%q", got, tt.want)
			}
		})
	}
}

// FR-020 et research.md S9 : rien n'est signalé pour un objet dont une lecture a échoué.
func TestWatchFailedRead(t *testing.T) {
	for _, mode := range []string{"deny", "refused"} {
		a, _, signals := watchEnv(t, "selector-typo", "web")
		if mode == "deny" {
			t.Setenv("FAKE_DENY", "endpointslices")
		} else {
			t.Setenv("FAKE_MODE", "refused")
		}
		runWatch(a, "apply", "-f", manifestFile(t, serviceWeb), "-n", "demo")
		if got := signals(); got != "" {
			t.Errorf("%s : aucun signal attendu sur une information incomplète : %q", mode, got)
		}
	}
}

// Au plus 12 lectures par vérification ; les objets hors budget ne sont pas signalés.
func TestWatchReadBudget(t *testing.T) {
	a, log, signals := watchEnv(t, "selector-typo", "*")
	var docs []string
	for i := range 10 {
		docs = append(docs, strings.Replace(serviceWeb, "name: web", fmt.Sprintf("name: web%d", i), 1))
	}
	runWatch(a, "apply", "-f", manifestFile(t, strings.Join(docs, "---\n")), "-n", "demo")
	if calls := readCalls(t, log); len(calls) != WatchReads {
		t.Errorf("%d lectures, %d attendues (budget épuisé) : %v", len(calls), WatchReads, calls)
	}
	// Chaque Service coûte 4 lectures : les 3 premiers sont vérifiés, les autres non.
	if got := strings.Count(signals(), "⚠"); got != WatchReads/4 {
		t.Errorf("%d signaux, %d attendus :\n%s", got, WatchReads/4, signals())
	}
}

// FR-023 : l'information sur un tag mutable est décidée sans lecture, au plus une fois par
// image et par 24 h.
func TestWatchImageHint(t *testing.T) {
	a, log, signals := watchEnv(t, "healthy", "")
	t.Setenv("FAKE_MODE", "refused")
	m := manifestFile(t, strings.Replace(deployWeb, "nginx:1.29", "nginx", 1))
	runWatch(a, "apply", "-f", m, "-n", "demo")
	runWatch(a, "apply", "-f", m, "-n", "demo")
	want := "ℹ L'image nginx n'a pas de tag explicite : un nouvel apply ne la mettra pas à jour (kubectl telemach deepen)\n"
	if got := signals(); got != want {
		t.Errorf("signaux :\n%q\nattendu une seule fois :\n%q", got, want)
	}
	// Les seules lectures sont celles des pods du Deployment (cluster injoignable ici) : la
	// ligne ℹ n'en dépend pas.
	for _, c := range readCalls(t, log) {
		if !strings.HasPrefix(c, "get pods -l ") {
			t.Errorf("lecture inattendue : %q", c)
		}
	}
	a.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	runWatch(a, "apply", "-f", m, "-n", "demo")
	if got := strings.Count(signals(), "ℹ"); got != 2 {
		t.Errorf("après 24 h, l'information doit revenir : %d ligne(s)", got)
	}
}
