package manifests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Dossiers soumis au contrôle manifest-lint. Les autres dossiers de testdata/manifests/
// contiennent des manifestes volontairement faux (corpus d'erreurs) et sont exclus.
var lintedDirs = []string{
	"../../testdata/manifests/valid",
	"../../testdata/pods/scenarios",
}

// expectations est le contenu facultatif d'un fichier expected.yaml : défauts tolérés par
// fichier, chacun étant le défaut volontaire d'un scénario.
type expectations struct {
	AllowedDefects map[string][]string `yaml:"allowed_defects"`
}

// TestRepositoryManifests applique les règles de sécurité à tous les manifestes déployables
// du dépôt. Il réussit s'il n'en trouve aucun (principe IV : le contrôle existe avant les
// manifestes).
func TestRepositoryManifests(t *testing.T) {
	found := 0
	for _, dir := range lintedDirs {
		exp := loadExpectations(t, dir)
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return err
			}
			if d.IsDir() || filepath.Base(path) == "expected.yaml" || !isManifest(path) {
				return nil
			}
			found++
			rel, _ := filepath.Rel(dir, path)
			t.Run(filepath.ToSlash(filepath.Join(filepath.Base(dir), rel)), func(t *testing.T) {
				checkFile(t, path, exp.AllowedDefects[filepath.ToSlash(rel)])
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d manifeste(s) vérifié(s)", found)
}

func checkFile(t *testing.T, path string, allowed []string) {
	t.Helper()
	src, err := os.ReadFile(path) //nolint:gosec // chemin issu du parcours du dépôt
	if err != nil {
		t.Fatal(err)
	}
	violations, warnings := Check(src)
	for _, w := range warnings {
		t.Logf("⚠ avertissement (non bloquant) : %s", w)
	}
	used := map[string]bool{}
	for _, v := range violations {
		if contains(allowed, v.Rule) {
			used[v.Rule] = true
			continue
		}
		t.Errorf("%s", v)
	}
	for _, a := range allowed {
		if !used[a] {
			t.Errorf("défaut déclaré %q absent du manifeste : retirer la déclaration de expected.yaml", a)
		}
	}
	for _, sv := range SchemaCheck(src) {
		t.Errorf("règles Kubernetes : %s", sv)
	}
}

func loadExpectations(t *testing.T, dir string) expectations {
	t.Helper()
	var exp expectations
	data, err := os.ReadFile(filepath.Join(dir, "expected.yaml")) //nolint:gosec // chemin fixe du dépôt
	if os.IsNotExist(err) {
		return exp
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &exp); err != nil {
		t.Fatalf("%s/expected.yaml : %v", dir, err)
	}
	return exp
}

func isManifest(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
		return true
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestCheckRules(t *testing.T) {
	safe := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
      - name: web
        image: nginxinc/nginx-unprivileged:1.30-alpine
        securityContext:
          allowPrivilegeEscalation: false
        resources:
          requests: {cpu: 50m, memory: 64Mi}
          limits: {cpu: 200m, memory: 128Mi}
`
	if v, w := Check([]byte(safe)); len(v) != 0 || len(w) != 0 {
		t.Errorf("manifeste sûr : violations %v, avertissements %v", v, w)
	}
	unsafe := strings.NewReplacer(
		"        runAsNonRoot: true\n", "        runAsUser: 0\n",
		"          allowPrivilegeEscalation: false\n", "          privileged: true\n",
		"nginxinc/nginx-unprivileged:1.30-alpine", "nginx",
		"      containers:", "      hostNetwork: true\n      volumes:\n      - name: h\n        hostPath: {path: /}\n      containers:",
	).Replace(safe)
	unsafe = strings.Replace(unsafe, "        resources:\n          requests: {cpu: 50m, memory: 64Mi}\n          limits: {cpu: 200m, memory: 128Mi}\n", "", 1)
	v, w := Check([]byte(unsafe))
	got := map[string]bool{}
	for _, x := range v {
		got[x.Rule] = true
	}
	for _, rule := range []string{RuleRunAsNonRoot, RuleAllowPrivilegeEscalation, RulePrivileged, RuleResources, RuleHostNetwork, RuleHostPath} {
		if !got[rule] {
			t.Errorf("règle %s non détectée : %v", rule, v)
		}
	}
	if len(w) != 1 || !strings.Contains(w[0], "sans tag") {
		t.Errorf("un avertissement « sans tag » attendu, obtenu %v", w)
	}
	if v, _ := Check([]byte("a: [")); len(v) != 1 || v[0].Rule != RuleParse {
		t.Errorf("YAML invalide : violation parse attendue, obtenu %v", v)
	}
}
