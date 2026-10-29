package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// expected est le contenu de testdata/manifests/<cas>/expected.yaml.
type expected struct {
	File         string `yaml:"file"`
	Document     int    `yaml:"document"`
	Line         int    `yaml:"line"`
	Column       int    `yaml:"column"`
	Resource     string `yaml:"resource"`
	FieldPath    string `yaml:"field_path"`
	Kind         string `yaml:"kind"`
	Suggestion   string `yaml:"suggestion"`
	RelatedLines []int  `yaml:"related_lines"`
	Apply        bool   `yaml:"apply"`
}

type manifestCase struct {
	name, dir string
	exp       expected
	stderr    string
}

func loadManifestCases(t *testing.T) []manifestCase {
	t.Helper()
	dirs, err := filepath.Glob("../../testdata/manifests/*")
	if err != nil {
		t.Fatal(err)
	}
	var out []manifestCase
	for _, dir := range dirs {
		if filepath.Base(dir) == "valid" {
			continue
		}
		c := manifestCase{name: filepath.Base(dir), dir: dir}
		data, err := os.ReadFile(filepath.Join(dir, "expected.yaml")) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatalf("%s : %v", c.name, err)
		}
		if err := yaml.Unmarshal(data, &c.exp); err != nil {
			t.Fatalf("%s : %v", c.name, err)
		}
		if s, err := os.ReadFile(filepath.Join(dir, "stderr.txt")); err == nil { //nolint:gosec // corpus du dépôt
			c.stderr = string(s)
		}
		out = append(out, c)
	}
	return out
}

// match vérifie qu'un problème correspond à l'attendu ; il renvoie la liste des écarts.
func match(is Issue, e expected) []string {
	var diffs []string
	check := func(name string, got, want any) {
		if fmt.Sprint(got) != fmt.Sprint(want) {
			diffs = append(diffs, fmt.Sprintf("%s=%v (attendu %v)", name, got, want))
		}
	}
	check("fichier", filepath.Base(is.File), e.File)
	check("ligne", is.Line, e.Line)
	check("colonne", is.Column, e.Column)
	if e.Kind != KindNotUTF8 {
		check("document", is.Document, e.Document)
	}
	if e.FieldPath != "" {
		check("champ", is.FieldPath, e.FieldPath)
	}
	if e.Resource != "" {
		check("ressource", is.Resource, e.Resource)
	}
	if e.Suggestion != "" {
		check("suggestion", is.Suggestion, e.Suggestion)
	}
	if len(e.RelatedLines) > 0 {
		check("lignes liées", is.RelatedLines, e.RelatedLines)
	}
	return diffs
}

// TestManifestCorpus mesure SC-002 : bon fichier, bonne ligne et bon champ dans au moins
// 90 % des cas, via « check » (analyse locale) ; aucun extrait ne contient de secret (SC-006).
func TestManifestCorpus(t *testing.T) {
	cases := loadManifestCases(t)
	if len(cases) < 20 {
		t.Fatalf("au moins 20 manifestes erronés attendus, %d trouvés", len(cases))
	}
	ok := 0
	for _, c := range cases {
		r := Analyze([]string{filepath.Join(c.dir, c.exp.File)}, false, "")
		var best []string
		found := false
		for _, is := range r.Issues {
			if is.Kind != c.exp.Kind {
				continue
			}
			diffs := match(is, c.exp)
			if len(diffs) == 0 {
				found = true
				break
			}
			if best == nil {
				best = diffs
			}
		}
		for _, is := range r.Issues {
			for _, l := range append(is.Excerpt, is.Fixed...) {
				if strings.Contains(l.Text, "s3cret-fake") {
					t.Errorf("%s : secret en clair dans un extrait : %q", c.name, l.Text)
				}
			}
		}
		if found {
			ok++
			continue
		}
		var kinds []string
		for _, is := range r.Issues {
			kinds = append(kinds, fmt.Sprintf("%s@%d:%d %s", is.Kind, is.Line, is.Column, is.FieldPath))
		}
		t.Logf("✖ %s : attendu %s ; écarts %v ; trouvés %v", c.name, c.exp.Kind, best, kinds)
	}
	rate := float64(ok) / float64(len(cases))
	t.Logf("SC-002 (check) : %d/%d manifestes correctement localisés (%.0f %%)", ok, len(cases), rate*100)
	if rate < 0.9 {
		t.Errorf("SC-002 non atteint : %.0f %% < 90 %%", rate*100)
	}
}

// TestManifestCorpusFromKubectl mesure SC-002 côté explain : le message réel de kubectl est
// rattaché à la bonne ligne du bon fichier.
func TestManifestCorpusFromKubectl(t *testing.T) {
	total, ok := 0, 0
	for _, c := range loadManifestCases(t) {
		if c.stderr == "" {
			continue
		}
		total++
		r := Analyze([]string{filepath.Join(c.dir, c.exp.File)}, false, "")
		locs := Locate(c.stderr, r.Files)
		if locatedOK(locs, r.Issues, c.exp) {
			ok++
			continue
		}
		t.Logf("✖ %s : message rattaché à %+v, attendu ligne %d", c.name, locs, c.exp.Line)
	}
	if total == 0 {
		t.Fatal("aucun message kubectl réel dans le corpus (stderr.txt)")
	}
	rate := float64(ok) / float64(total)
	t.Logf("SC-002 (explain) : %d/%d messages kubectl rattachés à la bonne ligne (%.0f %%)", ok, total, rate*100)
	if rate < 0.9 {
		t.Errorf("SC-002 non atteint côté explain : %.0f %% < 90 %%", rate*100)
	}
}

// locatedOK : le message de kubectl est rattaché au bon endroit s'il désigne la ligne
// attendue (ou la suivante, pour une erreur YAML que le parseur signale une ligne plus bas),
// ou une position couverte par le problème attendu (ligne liée, même champ).
func locatedOK(locs []Location, issues []Issue, e expected) bool {
	if len(locs) == 0 {
		// Message non rattachable (par exemple deux documents fusionnés faute de « --- ») :
		// explain montre alors le problème trouvé par l'analyse locale.
		for _, is := range issues {
			if is.Kind == e.Kind && is.Line == e.Line {
				return true
			}
		}
	}
	for _, l := range locs {
		if l.Line == e.Line || l.Line == e.Line+1 {
			return true
		}
		for _, is := range issues {
			if is.Kind == e.Kind && is.Line == e.Line && is.Covers(l) {
				return true
			}
		}
	}
	return false
}

func TestValidManifests(t *testing.T) {
	r := Analyze([]string{"../../testdata/manifests/valid"}, false, "v1.36.5")
	if len(r.Issues) != 0 {
		t.Errorf("aucun problème attendu dans valid/ : %+v", r.Issues)
	}
	if r.RulesVersion != "1.36" || r.RulesFallback || r.Documents != 4 {
		t.Errorf("règles %s (repli %v), %d documents", r.RulesVersion, r.RulesFallback, r.Documents)
	}
}

// Issue #11 : un manifeste de placement correct (anti-affinité, tolérance, répartition par
// zone) ne contient aucun secret en clair ; topologyKey doit rester lisible.
func TestPlacementFieldsAreNotSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.yaml")
	src := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              matchLabelKeys: [pod-template-hash]
              labelSelector:
                matchLabels:
                  app: api
      tolerations:
      - key: dedie
        operator: Equal
        value: gpu
        effect: NoSchedule
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app: api
      securityContext:
        runAsNonRoot: true
      containers:
      - name: api
        image: nginxinc/nginx-unprivileged:1.30-alpine
        securityContext:
          allowPrivilegeEscalation: false
        resources:
          requests: {cpu: 50m, memory: 64Mi}
          limits: {cpu: 200m, memory: 128Mi}
`
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Analyze([]string{path}, false, "v1.37.0")
	for _, is := range r.Issues {
		t.Errorf("problème inattendu : %s ligne %d (%s)", is.Kind, is.Line, is.FieldPath)
	}
}
