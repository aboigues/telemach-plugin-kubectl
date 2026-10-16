//go:build integration

// Commande collect-errors : outil du mainteneur qui relève de VRAIES erreurs kubectl sur un
// cluster kind jetable et les range dans testdata/errors/ (T038). Chaque cas produit
// args.txt (un argument par ligne), stderr.txt (masqué) et exit-code.txt ; le fichier
// expected.yaml, rédigé à la main, n'est jamais écrasé.
//
//	TELEMACH_KIND_IMAGE=kindest/node:v1.37.0@sha256:… TELEMACH_KUBECTL_BIN=kubectl-v1.37.1 \
//	  go run -tags integration ./scripts/collect-errors -out testdata/errors
//
// Avec -variant 1.35, un cas dont le message diffère de celui déjà relevé est rangé dans
// <id>@1.35 (variante de version).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

// Images utilisées par les scénarios (vérifiées sur Docker Hub le 2026-10-16, D-LL-004).
const (
	webImage  = "nginxinc/nginx-unprivileged:1.30-alpine"
	toolImage = "busybox:1.38"
)

// collectCase est une commande fautive. Dans Args, {NS} est le namespace de démonstration,
// {EMPTY} un kubeconfig vide, {SERVER} l'adresse du serveur API, {FILE} un manifeste périmé.
type collectCase struct {
	ID   string
	Args []string
	Raw  bool // sans --context imposé (erreurs de contexte et de connexion)
}

var cases = []collectCase{
	// Ligne de commande.
	{ID: "get-unknown-type", Args: words("get deploymnet -n {NS}")},
	{ID: "get-unknown-type-plural", Args: words("get pdos")},
	{ID: "describe-unknown-type", Args: words("describe pdo web")},
	{ID: "unknown-command", Args: words("gt pods")},
	{ID: "unknown-flag", Args: words("get pods --nmespace {NS}")},
	{ID: "unknown-shorthand", Args: words("get pods -z")},
	{ID: "get-no-type", Args: words("get")},
	{ID: "plugin-flags-before-name", Args: words("-n {NS} telemach diagnose")},
	{ID: "create-deployment-no-image", Args: words("create deployment web2 -n {NS}")},
	{ID: "run-invalid-name", Args: words("run Web_1 --image=" + webImage + " -n {NS}")},
	{ID: "scale-no-replicas", Args: words("scale deployment web -n {NS}")},
	// Ressources introuvables.
	{ID: "get-pod-notfound-default", Args: words("get pod web")},
	{ID: "describe-deploy-notfound", Args: words("describe deployment absent -n {NS}")},
	{ID: "delete-pod-notfound", Args: words("delete pod absent -n {NS}")},
	{ID: "logs-pod-notfound", Args: words("logs absent -n {NS}")},
	{ID: "exec-pod-notfound", Args: words("exec absent -n {NS} -- ls")},
	{ID: "scale-deploy-notfound", Args: words("scale deployment absent --replicas=2 -n {NS}")},
	{ID: "rollout-status-notfound", Args: words("rollout status deployment/absent -n {NS}")},
	{ID: "edit-deploy-notfound", Args: words("edit deployment absent -n {NS}")},
	{ID: "port-forward-notfound", Args: words("port-forward pod/absent 8080:8080 -n {NS}")},
	{ID: "expose-notfound", Args: words("expose deployment absent --port=8080 -n {NS}")},
	{ID: "set-image-unknown-container", Args: words("set image deployment/web nope=" + webImage + " -n {NS}")},
	{ID: "logs-waiting", Args: words("logs badimage -n {NS}")},
	// Namespaces.
	{ID: "create-in-missing-namespace", Args: words("create configmap c --from-literal=mode=demo -n inexistant")},
	{ID: "describe-namespace-notfound", Args: words("describe namespace inexistant")},
	// Fichiers.
	{ID: "apply-missing-file", Args: words("apply -f absent.yaml -n {NS}")},
	{ID: "diff-missing-file", Args: words("diff -f absent.yaml -n {NS}")},
	// Contexte, kubeconfig, connexion.
	{ID: "context-not-exist", Args: words("get pods --context inexistant"), Raw: true},
	{ID: "use-context-missing", Args: words("config use-context inexistant"), Raw: true},
	{ID: "get-contexts-missing", Args: words("config get-contexts inexistant"), Raw: true},
	{ID: "current-context-not-set", Args: words("config current-context --kubeconfig {EMPTY}"), Raw: true},
	{ID: "connection-refused-localhost", Args: words("get pods --kubeconfig {EMPTY}"), Raw: true},
	{ID: "x509-unknown-authority", Args: words("get pods --kubeconfig {EMPTY} --server {SERVER} --token fake-token"), Raw: true},
	{ID: "unauthorized", Args: words("get pods --kubeconfig {EMPTY} --server {SERVER} --insecure-skip-tls-verify --token fake-token"), Raw: true},
	// Droits.
	{ID: "forbidden-list-pods", Args: words("get pods -n {NS} --as=system:serviceaccount:{NS}:default")},
	{ID: "forbidden-create-deployment", Args: words("create deployment x --image=" + webImage + " -n {NS} --as=system:serviceaccount:{NS}:default")},
	// Conflits.
	{ID: "create-already-exists", Args: words("create deployment web --image=" + webImage + " -n {NS}")},
	{ID: "replace-conflict", Args: words("replace -f {FILE}")},
	// Programme du conteneur.
	{ID: "exec-application-error", Args: words("exec deploy/web -n {NS} -- sh -c exit_3")},
	{ID: "exec-command-not-found", Args: words("exec deploy/web -n {NS} -- doesnotexist")},
}

func words(s string) []string { return strings.Fields(s) }

func main() {
	out := flag.String("out", "testdata/errors", "dossier du corpus")
	variant := flag.String("variant", "", "version mineure relevée (ex. 1.35) : variante rangée dans <id>@<version> si le message diffère")
	only := flag.String("only", "", "identifiant d'un seul cas à relever")
	manifests := flag.String("manifests", "", "dossier du corpus de manifestes : relève le message de « kubectl apply -f » de chaque cas")
	flag.Parse()
	if *manifests != "" {
		if err := collectManifests(*manifests); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := collect(*out, *variant, *only); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func collect(out, variant, only string) error {
	image := os.Getenv("TELEMACH_KIND_IMAGE")
	if image == "" {
		return fmt.Errorf("TELEMACH_KIND_IMAGE est obligatoire (image kindest/node épinglée par digest)")
	}
	c, err := kindcluster.Create(image)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Delete(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	ns := "demo"
	if err := setup(c, ns); err != nil {
		return err
	}
	empty := filepath.Join(c.Dir, "empty-kubeconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		return err
	}
	stale, err := staleManifest(c, ns)
	if err != nil {
		return err
	}
	repl := strings.NewReplacer("{NS}", ns, "{EMPTY}", empty, "{SERVER}", c.ServerURL(), "{FILE}", stale)
	for _, tc := range cases {
		if only != "" && tc.ID != only {
			continue
		}
		args := make([]string, len(tc.Args))
		for i, a := range tc.Args {
			args[i] = strings.ReplaceAll(repl.Replace(a), "exit_3", "exit 3")
		}
		var r kindcluster.Result
		if tc.Raw {
			r = c.KubectlRaw(args...)
		} else {
			r = c.Kubectl(args...)
		}
		if r.ExitCode == 0 {
			fmt.Fprintf(os.Stderr, "⚠ %s : la commande a réussi, cas ignoré\n", tc.ID)
			continue
		}
		if err := save(out, variant, tc.ID, c, args, r); err != nil {
			return err
		}
	}
	return nil
}

// setup prépare le namespace de démonstration : un deployment sain, un pod à deux conteneurs
// et un pod dont l'image est introuvable.
func setup(c *kindcluster.Cluster, ns string) error {
	steps := [][]string{
		{"create", "namespace", ns},
		{"create", "deployment", "web", "--image=" + webImage, "-n", ns},
	}
	for _, s := range steps {
		if err := c.Must(s...); err != nil {
			return err
		}
	}
	pods := fmt.Sprintf(podsYAML, toolImage, toolImage, webImage)
	file := filepath.Join(c.Dir, "pods.yaml")
	if err := os.WriteFile(file, []byte(pods), 0o600); err != nil { //nolint:gosec // dossier temporaire du cluster de test
		return err
	}
	if err := c.Must("apply", "-n", ns, "-f", file); err != nil {
		return err
	}
	if err := c.Must("rollout", "status", "deployment/web", "-n", ns, "--timeout=180s"); err != nil {
		return err
	}
	if err := c.Must("wait", "--for=condition=Ready", "pod/duo", "-n", ns, "--timeout=180s"); err != nil {
		return err
	}
	// Laisser le temps au téléchargement de l'image introuvable d'échouer.
	time.Sleep(15 * time.Second)
	return nil
}

const podsYAML = `apiVersion: v1
kind: Pod
metadata:
  name: duo
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65534
  containers:
  - name: un
    image: %s
    command: ["sleep", "3600"]
    securityContext: {allowPrivilegeEscalation: false}
    resources: {requests: {cpu: 10m, memory: 16Mi}, limits: {cpu: 50m, memory: 32Mi}}
  - name: deux
    image: %s
    command: ["sleep", "3600"]
    securityContext: {allowPrivilegeEscalation: false}
    resources: {requests: {cpu: 10m, memory: 16Mi}, limits: {cpu: 50m, memory: 32Mi}}
---
apiVersion: v1
kind: Pod
metadata:
  name: badimage
spec:
  securityContext:
    runAsNonRoot: true
  containers:
  - name: web
    image: %s-0.0.0-inexistant
    securityContext: {allowPrivilegeEscalation: false}
    resources: {requests: {cpu: 10m, memory: 16Mi}, limits: {cpu: 50m, memory: 32Mi}}
`

// staleManifest enregistre le deployment web, puis le modifie : remplacer le fichier
// enregistré provoque un conflit de version (Conflict).
func staleManifest(c *kindcluster.Cluster, ns string) (string, error) {
	r := c.Kubectl("get", "deployment", "web", "-n", ns, "-o", "yaml")
	if r.ExitCode != 0 {
		return "", fmt.Errorf("get deployment web : %s", r.Stderr)
	}
	file := filepath.Join(c.Dir, "web-stale.yaml")
	if err := os.WriteFile(file, []byte(r.Stdout), 0o600); err != nil { //nolint:gosec // dossier temporaire du cluster de test
		return "", err
	}
	if err := c.Must("annotate", "deployment", "web", "-n", ns, "telemach/modifie=oui"); err != nil {
		return "", err
	}
	return file, nil
}

// normalize rend le message stable d'un relevé à l'autre : nom du cluster, port du serveur et
// chemins temporaires neutralisés, secrets masqués.
func normalize(s string, c *kindcluster.Cluster) string {
	s = strings.ReplaceAll(s, c.Dir, "/tmp/telemach")
	s = strings.ReplaceAll(s, c.Name, "telemach-cluster")
	if u := c.ServerURL(); u != "" {
		s = strings.ReplaceAll(s, strings.TrimPrefix(u, "https://"), "127.0.0.1:6443")
	}
	s = execID.ReplaceAllString(s, `exec "<id>"`)
	return redact.Redact(s)
}

// execID est l'identifiant aléatoire d'une exécution dans un conteneur.
var execID = regexp.MustCompile(`exec "[0-9a-f]{64}"`)

func save(out, variant, id string, c *kindcluster.Cluster, args []string, r kindcluster.Result) error {
	stderr := normalize(r.Stderr, c)
	dir := filepath.Join(out, id)
	if variant != "" {
		if prev, err := os.ReadFile(filepath.Join(dir, "stderr.txt")); err == nil { //nolint:gosec // corpus du dépôt
			if string(prev) == stderr {
				return nil
			}
			dir = filepath.Join(out, id+"@"+variant)
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	files := map[string]string{
		"args.txt":      normalize(strings.Join(args, "\n"), c) + "\n",
		"stderr.txt":    stderr,
		"exit-code.txt": fmt.Sprintf("%d\n", r.ExitCode),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			return err
		}
	}
	fmt.Printf("%-32s code %3d  %s", filepath.Base(dir), r.ExitCode, firstLine(stderr))
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i+1]
	}
	return s + "\n"
}

// collectManifests applique chaque manifeste erroné du corpus (champ apply: true de
// expected.yaml) sur un cluster jetable et range le message de kubectl dans le dossier du cas.
func collectManifests(root string) error {
	image := os.Getenv("TELEMACH_KIND_IMAGE")
	if image == "" {
		return fmt.Errorf("TELEMACH_KIND_IMAGE est obligatoire (image kindest/node épinglée par digest)")
	}
	dirs, err := filepath.Glob(filepath.Join(root, "*"))
	if err != nil {
		return err
	}
	c, err := kindcluster.Create(image)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Delete(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	if err := c.Must("create", "namespace", "demo"); err != nil {
		return err
	}
	for _, dir := range dirs {
		exp, err := os.ReadFile(filepath.Join(dir, "expected.yaml")) //nolint:gosec // corpus du dépôt
		if err != nil || !strings.Contains(string(exp), "apply: true") {
			continue
		}
		file := ""
		for _, line := range strings.Split(string(exp), "\n") {
			if v, ok := strings.CutPrefix(line, "file: "); ok {
				file = strings.Trim(v, "'")
			}
		}
		args := []string{"apply", "-f", file, "-n", "demo"}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		r := c.KubectlIn(abs, args...)
		if r.ExitCode == 0 {
			fmt.Fprintf(os.Stderr, "⚠ %s : apply a réussi\n", filepath.Base(dir))
			_ = c.KubectlIn(abs, "delete", "-f", file, "-n", "demo", "--wait=false")
			continue
		}
		files := map[string]string{
			"args.txt":      strings.Join(args, "\n") + "\n",
			"stderr.txt":    normalize(r.Stderr, c),
			"exit-code.txt": fmt.Sprintf("%d\n", r.ExitCode),
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil { //nolint:gosec // corpus du dépôt
				return err
			}
		}
		fmt.Printf("%-28s %s", filepath.Base(dir), firstLine(files["stderr.txt"]))
	}
	return nil
}
