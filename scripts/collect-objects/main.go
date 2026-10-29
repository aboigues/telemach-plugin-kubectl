//go:build integration

// Commande collect-objects : outil du mainteneur qui relève, sur un cluster kind JETABLE, les
// objets réels de chaque piège silencieux de la feature 002 (T005) et les range dans
// testdata/objects/<cas>/. Les lectures sont exactement celles du plugin
// (internal/kuberead/commands.go) : le corpus contient ce que le plugin lit, rien de plus.
// Chaque sortie passe par redact ; le fichier expected.yaml, rédigé à la main, n'est jamais
// écrasé. Un pod en échec relevé pour le catalogue des pods (cas DNS) est rangé au format de
// testdata/pods/<nom>/.
//
//	TELEMACH_KIND_IMAGE=kindest/node:v1.37.0@sha256:… TELEMACH_KUBECTL_BIN=kubectl-v1.37.1 \
//	  go run -tags integration ./scripts/collect-objects -out testdata/objects -pods testdata/pods
//
// Avec -variant 1.35, un cas dont un fichier diffère du relevé existant est rangé dans
// <cas>@1.35.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

const ns = "demo"

// read est une lecture du plugin et le fichier où ranger sa sortie. dyn calcule les arguments
// au moment du relevé (nom généré, par exemple celui d'un ReplicaSet).
type read struct {
	file string
	args []string
	dyn  func(c *kindcluster.Cluster, ns string) []string
}

// step prépare l'état d'un cas : kubectl (args), attente fixe (sleep) ou fichier à appliquer.
type step struct {
	apply   string        // manifeste de testdata/objects/scenarios/ à appliquer
	args    []string      // commande kubectl à exécuter
	sleep   time.Duration // attente
	soft    bool          // échec toléré (attente qui peut expirer)
	cluster bool          // objet du cluster : la commande ne reçoit pas -n
}

// objectCase est un piège à relever. L'ordre de la liste compte : certains cas réutilisent
// les objets des précédents (le Service hello de healthy pour les Ingress).
type objectCase struct {
	id    string
	ns    string // namespace du cas (feature 004) ; vide : « demo »
	steps []step
	reads []read
	after []step // remise en état après le relevé
	pod   string // pod à ranger au format de testdata/pods/ (cas DNS)
}

func apply(f string) step  { return step{apply: f} }
func run(a ...string) step { return step{args: a} }
func waitAvailable(deploy string) step {
	return step{args: []string{"wait", "--for=condition=Available", "deployment/" + deploy, "--timeout=180s"}}
}
func pause(d time.Duration) step           { return step{sleep: d} }
func r(file string, args []string) read    { return read{file: file, args: args} }
func service(name, selector string) []read { return serviceReads(name, selector) }

func serviceReads(name, selector string) []read {
	out := []read{
		r("service.json", kuberead.ServiceArgs(name)),
		r("endpointslices.json", kuberead.ServiceEndpointSlicesArgs(name)),
		r("pods.json", kuberead.PodsArgs()),
	}
	if selector != "" {
		out = append(out, r("selected-pods.json", kuberead.PodsBySelectorArgs(selector)))
	}
	return out
}

func deploymentReads(name, selector string) []read {
	return []read{
		r("deployment.json", kuberead.DeploymentArgs(name)),
		r("replicasets.json", kuberead.ReplicaSetsArgs(selector)),
		r("selected-pods.json", kuberead.PodsBySelectorArgs(selector)),
	}
}

var cases = []objectCase{
	{id: "selector-typo", steps: []step{apply("selector-typo.yaml"), waitAvailable("frontend")}, reads: service("web", "app=web")},
	{id: "not-ready", steps: []step{apply("not-ready.yaml"), pause(30 * time.Second)}, reads: service("api", "app=api")},
	{id: "targetport-name", steps: []step{apply("targetport-name.yaml"), waitAvailable("shop")}, reads: service("shop", "app=shop")},
	{id: "targetport-number", steps: []step{apply("targetport-number.yaml"), waitAvailable("cart")}, reads: service("cart", "app=cart")},
	{id: "targetport-undeclared", steps: []step{apply("targetport-undeclared.yaml"), waitAvailable("blog")}, reads: service("blog", "app=blog")},
	{id: "no-selector", steps: []step{apply("no-selector.yaml")}, reads: []read{
		r("service.json", kuberead.ServiceArgs("external")),
		r("endpointslices.json", kuberead.ServiceEndpointSlicesArgs("external")),
		r("service-docs.json", kuberead.ServiceArgs("docs")),
	}},
	{id: "healthy", steps: []step{apply("healthy.yaml"), waitAvailable("hello")}, reads: append(service("hello", "app=hello"), deploymentReads("hello", "app=hello")[:2]...)},
	{id: "rollout-stalled", steps: []step{
		apply("rollout-stalled.yaml"), waitAvailable("stalled"),
		run("set", "image", "deployment/stalled", "app=nginxinc/nginx-unprivileged:0.0.0-inexistant"),
		pause(50 * time.Second),
	}, reads: deploymentReads("stalled", "app=stalled")},
	{id: "rollout-progressing", steps: []step{apply("rollout-progressing.yaml"), pause(15 * time.Second)}, reads: deploymentReads("slow", "app=slow")},
	{id: "rollout-paused", steps: []step{
		apply("rollout-paused.yaml"), waitAvailable("paused"),
		run("rollout", "pause", "deployment/paused"),
		run("set", "image", "deployment/paused", "app=nginxinc/nginx-unprivileged:0.0.0-inexistant"),
		pause(5 * time.Second),
	}, reads: deploymentReads("paused", "app=paused")},
	{id: "mutable-tag", steps: []step{apply("mutable-tag.yaml"), waitAvailable("latest-app")}, reads: deploymentReads("latest-app", "app=latest-app")},
	{id: "config-env", steps: []step{apply("config-env.yaml"), waitAvailable("conf")}, reads: append(deploymentReads("conf", "app=conf"),
		r("configmap-dates.txt", kuberead.ConfigMapDatesArgs("conf-settings")))},
	// Même Deployment après une nouvelle valeur de la ConfigMap : les pods gardent l'ancienne.
	{id: "config-env-stale", steps: []step{pause(2 * time.Second), apply("config-env-update.yaml"), pause(2 * time.Second)}, reads: append(deploymentReads("conf", "app=conf"),
		r("configmap-dates.txt", kuberead.ConfigMapDatesArgs("conf-settings")))},
	{id: "ingress-no-class", steps: []step{apply("ingress-no-class.yaml"), pause(5 * time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("web")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-hello.json", kuberead.ServiceArgs("hello")),
	}},
	// La classe « demo » existe (non marquée par défaut) : « nginx-tpyo » est alors une classe
	// inconnue, et non l'absence de tout contrôleur.
	{id: "ingress-unknown-class", steps: []step{apply("ingressclass-not-default.yaml"), apply("ingress-unknown-class.yaml"), pause(5 * time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("typo")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-hello.json", kuberead.ServiceArgs("hello")),
	}},
	{id: "ingress-backend-missing", steps: []step{apply("ingress-backend-missing.yaml"), pause(5 * time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("shop")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-absent.json", kuberead.ServiceArgs("absent")),
	}},
	// Ingress « web » (sans classe) relu maintenant que la classe « demo » existe, non marquée par
	// défaut : aucun contrôleur ne le prend en charge.
	{id: "ingress-no-default-class", steps: []step{pause(time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("web")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-hello.json", kuberead.ServiceArgs("hello")),
	}},
	{id: "ingress-no-address", steps: []step{apply("ingress-no-address.yaml"), pause(5 * time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("hello-web")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-hello.json", kuberead.ServiceArgs("hello")),
	}},
	{id: "ingress-backend-port", steps: []step{apply("ingress-backend-port.yaml"), pause(5 * time.Second)}, reads: []read{
		r("ingress.json", kuberead.IngressArgs("wrong-port")),
		r("ingressclasses.json", kuberead.IngressClassesArgs()),
		r("service-hello.json", kuberead.ServiceArgs("hello")),
	}},
	{id: "pvc-unknown-class", steps: []step{apply("pvc-unknown-class.yaml"), pause(5 * time.Second)}, reads: []read{
		r("pvc.json", kuberead.PVCArgs("fast-data")),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
		r("pods.json", kuberead.PodsArgs()),
	}},
	{id: "pvc-waiting", steps: []step{apply("pvc-waiting.yaml"), pause(5 * time.Second)}, reads: []read{
		r("pvc.json", kuberead.PVCArgs("data")),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
		r("pods.json", kuberead.PodsArgs()),
	}},
	{id: "pvc-empty-class", steps: []step{apply("pvc-empty-class.yaml"), pause(5 * time.Second)}, reads: []read{
		r("pvc.json", kuberead.PVCArgs("static-data")),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
		r("pods.json", kuberead.PodsArgs()),
	}},
	// Pod consommateur impossible à planifier : le PVC (WaitForFirstConsumer) attend un pod qui
	// n'arrive pas ; le diagnostic passe à ce pod (feature 001).
	{id: "pvc-consumer-pending", steps: []step{apply("pvc-consumer-pending.yaml"), pause(10 * time.Second)}, reads: []read{
		r("pvc.json", kuberead.PVCArgs("cache-data")),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
		r("pods.json", kuberead.PodsArgs()),
		r("events-cache-user.json", kuberead.PodEventsArgs("cache-user")),
	}},
	// Sans classe par défaut (annotation retirée de « standard », dans ce cluster jetable
	// seulement, puis remise) : un PVC sans classe n'en reçoit aucune.
	{id: "pvc-no-default-class", steps: []step{
		{args: []string{"annotate", "storageclass", "standard", "storageclass.kubernetes.io/is-default-class=false", "--overwrite"}, cluster: true},
		apply("pvc-no-default.yaml"), pause(5 * time.Second),
	}, reads: []read{
		r("pvc.json", kuberead.PVCArgs("nodefault-data")),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
		r("pods.json", kuberead.PodsArgs()),
	}, after: []step{
		{args: []string{"annotate", "storageclass", "standard", "storageclass.kubernetes.io/is-default-class=true", "--overwrite"}, cluster: true},
	}},
	{
		id: "dns-short-name",
		steps: []step{
			apply("dns-short-name.yaml"),
			{args: []string{"wait", "--for=jsonpath={.status.phase}=Failed", "pod/dns-client", "--timeout=90s"}, soft: true},
		},
		pod: "dns-client",
	},
}

func main() {
	out := flag.String("out", "testdata/objects", "dossier du corpus d'objets")
	pods := flag.String("pods", "testdata/pods", "dossier du corpus de pods (cas DNS)")
	scenarios := flag.String("scenarios", "testdata/objects/scenarios", "dossier des manifestes des scénarios")
	variant := flag.String("variant", "", "version mineure relevée (ex. 1.35) : variante rangée dans <cas>@<version> si un fichier diffère")
	kindConfig := flag.String("kind-config", "testdata/kind/three-nodes.yaml", "configuration kind du cluster de relevé (feature 004 : trois nœuds)")
	only := flag.String("only", "", "cas à relever : un identifiant, ou f004 pour tous ceux de la feature 004")
	flag.Parse()
	if err := collect(*out, *pods, *scenarios, *variant, *kindConfig, *only); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func collect(out, podsDir, scenarios, variant, kindConfig, only string) error {
	image := os.Getenv("TELEMACH_KIND_IMAGE")
	if image == "" {
		return fmt.Errorf("TELEMACH_KIND_IMAGE est obligatoire (image kindest/node épinglée par digest)")
	}
	c, err := kindcluster.CreateWithConfig(image, kindConfig)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Delete(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	if err := c.Must("create", "namespace", ns); err != nil {
		return err
	}
	for _, oc := range append(append([]objectCase(nil), cases...), f004Cases...) {
		// -only f004 : les cas de la feature 004 (ceux qui ont leur propre namespace) ; sinon un
		// seul cas, par son identifiant.
		if only != "" && (only != "f004" || oc.ns == "") && oc.id != only {
			continue
		}
		if oc.ns != "" {
			// Un cas peut reprendre le namespace du précédent (même objet, état suivant).
			if res := c.Kubectl("create", "namespace", oc.ns); res.ExitCode != 0 && !strings.Contains(res.Stderr, "AlreadyExists") {
				return fmt.Errorf("%s : %s", oc.id, res.Stderr)
			}
		}
		if err := prepare(c, scenarios, oc.ns, oc.steps); err != nil {
			return fmt.Errorf("%s : %w", oc.id, err)
		}
		files := map[string]string{"captured-at.txt": time.Now().UTC().Format(time.RFC3339) + "\n"}
		for _, rd := range oc.reads {
			name, content := capture(c, oc.ns, rd)
			files[name] = content
		}
		dir := filepath.Join(out, oc.id)
		if oc.pod != "" {
			dir = filepath.Join(podsDir, oc.id)
			files = podFiles(c, oc.pod, files["captured-at.txt"])
		}
		if err := save(dir, variant, files); err != nil {
			return err
		}
		if err := prepare(c, scenarios, oc.ns, oc.after); err != nil {
			return fmt.Errorf("%s : remise en état : %w", oc.id, err)
		}
	}
	if only != "" {
		return nil
	}
	return checkConfigMapDates(c, out, scenarios)
}

func prepare(c *kindcluster.Cluster, scenarios, caseNS string, steps []step) error {
	namespace := ns
	if caseNS != "" {
		namespace = caseNS
	}
	nodes := strings.NewReplacer("{W1}", c.Worker(1), "{W2}", c.Worker(2))
	for _, s := range steps {
		switch {
		case s.apply != "":
			if err := c.Must("apply", "-n", namespace, "-f", filepath.Join(scenarios, s.apply)); err != nil {
				return err
			}
		case s.sleep > 0:
			time.Sleep(s.sleep)
		default:
			args := make([]string, len(s.args))
			for i, a := range s.args {
				args[i] = nodes.Replace(a)
			}
			if !s.cluster {
				args = append(args, "-n", namespace)
			}
			if r := c.Kubectl(args...); r.ExitCode != 0 && !s.soft {
				return fmt.Errorf("kubectl %s : %s", strings.Join(s.args, " "), r.Stderr)
			}
		}
	}
	return nil
}

// capture exécute une lecture ; un échec est rangé dans <fichier>.err (message masqué), comme
// le plugin le recevrait.
func capture(c *kindcluster.Cluster, caseNS string, rd read) (string, string) {
	namespace := ns
	if caseNS != "" {
		namespace = caseNS
	}
	args := rd.args
	if rd.dyn != nil {
		args = rd.dyn(c, namespace)
	}
	res := c.Kubectl(append(append([]string(nil), args...), "-n", namespace)...)
	if res.ExitCode != 0 {
		return rd.file + ".err", redact.Redact(res.Stderr)
	}
	return rd.file, redactJSON(res.Stdout)
}

// podFiles relève un pod au format de testdata/pods/<nom>/ (lectures de la feature 001).
func podFiles(c *kindcluster.Cluster, pod, capturedAt string) map[string]string {
	files := map[string]string{"captured-at.txt": capturedAt}
	if res := c.Kubectl("get", "pod", pod, "-o", "json", "-n", ns); res.ExitCode == 0 {
		files["pod.json"] = redactJSON(res.Stdout)
	}
	if res := c.Kubectl(append(kuberead.PodEventsArgs(pod), "-n", ns)...); res.ExitCode == 0 {
		files["events.json"] = redactJSON(res.Stdout)
	}
	if res := c.Kubectl("logs", pod, fmt.Sprintf("--tail=%d", kuberead.LogLines), "-n", ns); res.ExitCode == 0 {
		files["logs.txt"] = redact.Redact(res.Stdout)
	}
	return files
}

// checkConfigMapDates vérifie la lecture des dates d'une ConfigMap (T006, research.md S3) :
// au moins une date, aucune valeur, et une date plus récente après apply puis après edit.
func checkConfigMapDates(c *kindcluster.Cluster, out, scenarios string) error {
	dates := func() string {
		res := c.Kubectl(append(kuberead.ConfigMapDatesArgs("conf-settings"), "-n", ns)...)
		if res.ExitCode != 0 {
			return "ERREUR : " + redact.Redact(res.Stderr)
		}
		return strings.TrimSpace(res.Stdout)
	}
	var report strings.Builder
	initial := dates()
	fmt.Fprintf(&report, "après création :\n%s\n", initial)
	time.Sleep(2 * time.Second)
	updated := filepath.Join(c.Dir, "conf-settings.yaml")
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: conf-settings\ndata:\n  MODE: prod\n"
	if err := os.WriteFile(updated, []byte(manifest), 0o600); err != nil { //nolint:gosec // dossier temporaire du cluster de test
		return err
	}
	if err := c.Must("apply", "-n", ns, "-f", updated); err != nil {
		return err
	}
	afterApply := dates()
	fmt.Fprintf(&report, "après apply :\n%s\n", afterApply)
	time.Sleep(2 * time.Second)
	edit := c.KubectlEnv([]string{"KUBE_EDITOR=sed -i s/prod/staging/"}, "edit", "configmap", "conf-settings", "-n", ns)
	afterEdit := dates()
	fmt.Fprintf(&report, "après edit (code %d) :\n%s\n", edit.ExitCode, afterEdit)
	ok := initial != "" && !strings.HasPrefix(initial, "ERREUR") &&
		latest(afterApply) > latest(initial) && latest(afterEdit) > latest(afterApply)
	for _, value := range []string{"dev", "prod", "staging", "MODE"} {
		if strings.Contains(initial+afterApply+afterEdit, value) {
			ok = false
			fmt.Fprintf(&report, "VALEUR PRÉSENTE : %s\n", value)
		}
	}
	fmt.Fprintf(&report, "résultat : %s\n", map[bool]string{true: "OK", false: "ÉCHEC"}[ok])
	dir := filepath.Join(out, "configmap-dates")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte(report.String()), 0o600); err != nil { //nolint:gosec // corpus du dépôt
		return err
	}
	fmt.Print(report.String())
	return nil
}

// latest renvoie la date la plus récente d'une liste de dates RFC 3339 (comparaison de
// chaînes, valable pour ce format en UTC).
func latest(dates string) string {
	max := ""
	for _, d := range strings.Fields(dates) {
		if d > max {
			max = d
		}
	}
	return max
}

// redactJSON masque un document JSON en gardant sa structure : l'annotation
// last-applied-configuration (qui recopie le manifeste) devient une chaîne masquée, puis
// chaque valeur texte passe par redact. Un masquage textuel du document entier produirait un
// JSON invalide (valeur masquée sans guillemets).
func redactJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return redact.Redact(s)
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				if k == "kubectl.kubernetes.io/last-applied-configuration" {
					t[k] = redact.Mask
					continue
				}
				t[k] = walk(c)
			}
			return t
		case []any:
			for i, c := range t {
				t[i] = walk(c)
			}
			return t
		case string:
			return redact.Redact(t)
		default:
			return t
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(walk(v)); err != nil {
		return redact.Redact(s)
	}
	return buf.String()
}

// save range les fichiers d'un cas sans écraser expected.yaml ; avec variant, un cas dont un
// fichier diffère (hors date de relevé) est rangé dans <cas>@<variante>.
func save(dir, variant string, files map[string]string) error {
	if variant != "" && differs(dir, files) {
		dir += "@" + variant
	} else if variant != "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	for name, content := range files {
		if name == "expected.yaml" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil { //nolint:gosec // corpus du dépôt
			return err
		}
	}
	fmt.Printf("%-28s %d fichier(s)\n", filepath.Base(dir), len(files))
	return nil
}

// differs compare la forme des objets relevés à celle du relevé existant : les champs
// volatils (noms générés, dates, adresses) changent à chaque relevé, seules les clés comptent.
func differs(dir string, files map[string]string) bool {
	for name, content := range files {
		if name == "captured-at.txt" || !strings.HasSuffix(name, ".json") {
			continue
		}
		prev, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // corpus du dépôt
		if err != nil || shape(string(prev)) != shape(content) {
			return true
		}
	}
	return false
}

// shape résume un document JSON par l'ensemble trié de ses chemins de clés.
func shape(s string) string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return s
	}
	var paths []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				walk(prefix+"."+k, c)
			}
		case []any:
			for _, c := range t {
				walk(prefix+"[]", c)
			}
		default:
			paths = append(paths, prefix)
		}
	}
	walk("", v)
	seen := map[string]bool{}
	var uniq []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	sort.Strings(uniq)
	return strings.Join(uniq, "\n")
}
