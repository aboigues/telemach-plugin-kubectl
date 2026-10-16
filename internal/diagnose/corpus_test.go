package diagnose

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
)

// corpusCase est un cas de testdata/errors/ : vraie erreur relevée sur un cluster kind.
type corpusCase struct {
	Name     string
	Args     []string
	Stderr   string
	ExitCode int
	Expected struct {
		Entry                string `yaml:"entry"`
		Origin               string `yaml:"origin"`
		FixedCommandContains string `yaml:"fixed_command_contains"`
	}
}

func loadCorpus(t testing.TB) []corpusCase {
	t.Helper()
	dirs, err := filepath.Glob("../../testdata/errors/*")
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCase
	for _, dir := range dirs {
		c := corpusCase{Name: filepath.Base(dir)}
		read := func(name string) string {
			data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // corpus du dépôt
			if err != nil {
				t.Fatalf("%s : %v", c.Name, err)
			}
			return string(data)
		}
		c.Args = strings.Split(strings.TrimRight(read("args.txt"), "\n"), "\n")
		c.Stderr = read("stderr.txt")
		code, err := strconv.Atoi(strings.TrimSpace(read("exit-code.txt")))
		if err != nil {
			t.Fatalf("%s : exit-code.txt : %v", c.Name, err)
		}
		c.ExitCode = code
		if err := yaml.Unmarshal([]byte(read("expected.yaml")), &c.Expected); err != nil {
			t.Fatalf("%s : expected.yaml : %v", c.Name, err)
		}
		cases = append(cases, c)
	}
	return cases
}

// diagnoseCase rejoue la chaîne d'explain sur un cas du corpus.
func diagnoseCase(c corpusCase, cat *catalog.Catalog) Diagnosis {
	stderr := redact.Redact(sanitize.Clean(c.Stderr))
	args := redact.RedactArgs(c.Args)
	return Diagnose(Input{Command: command.Parse(args), ExitCode: c.ExitCode, Stderr: stderr, InvokedAs: "kubectl"}, cat)
}

// TestCorpus mesure SC-001 : au moins 90 % des erreurs du corpus reçoivent la bonne entrée,
// la bonne origine et la bonne commande corrigée ; aucune erreur marquée « unknown » ne
// reçoit de cause (SC-005).
func TestCorpus(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := loadCorpus(t)
	if len(cases) < 30 {
		t.Fatalf("le corpus doit contenir au moins 30 erreurs réelles, %d trouvées", len(cases))
	}
	ok := 0
	for _, c := range cases {
		d := diagnoseCase(c, cat)
		got := "unknown"
		if d.Entry != nil {
			got = d.Entry.ID
		}
		var problems []string
		if got != c.Expected.Entry {
			problems = append(problems, "entrée "+got+" au lieu de "+c.Expected.Entry)
		}
		if string(d.Origin) != c.Expected.Origin {
			problems = append(problems, "origine "+string(d.Origin)+" au lieu de "+c.Expected.Origin)
		}
		if f := c.Expected.FixedCommandContains; f != "" && !strings.Contains(d.Captures["fixed_command"], f) {
			problems = append(problems, "commande corrigée « "+d.Captures["fixed_command"]+" » sans « "+f+" »")
		}
		if c.Expected.Entry == "unknown" && d.Entry != nil {
			t.Errorf("%s : cause inventée pour une erreur non reconnue (SC-005)", c.Name)
		}
		if len(problems) == 0 {
			ok++
			continue
		}
		t.Logf("✖ %s : %s", c.Name, strings.Join(problems, " ; "))
	}
	rate := float64(ok) / float64(len(cases))
	t.Logf("SC-001 : %d/%d erreurs correctement expliquées (%.0f %%)", ok, len(cases), rate*100)
	if rate < 0.9 {
		t.Errorf("SC-001 non atteint : %.0f %% < 90 %%", rate*100)
	}
}

// TestCorpusCoversSubcommands vérifie qu'au moins un cas existe pour chaque sous-commande de
// FR-003.
func TestCorpusCoversSubcommands(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range loadCorpus(t) {
		cmd := command.Parse(c.Args)
		seen[cmd.Verb()] = true
		seen[cmd.Sub()] = true
	}
	for _, sub := range []string{
		"get", "describe", "apply", "create", "delete", "run", "expose", "logs", "exec",
		"scale", "rollout", "set image", "edit", "diff", "config use-context", "config current-context",
		"config get-contexts", "port-forward",
	} {
		if !seen[sub] {
			t.Errorf("aucun cas du corpus pour la sous-commande « %s » (FR-003)", sub)
		}
	}
}
