package diagnose_test

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
)

type timedCase struct {
	args   []string
	stderr string
	code   int
}

func loadTimedCases(t testing.TB) []timedCase {
	t.Helper()
	dirs, _ := filepath.Glob("../../testdata/errors/*")
	var out []timedCase
	for _, dir := range dirs {
		args, err1 := os.ReadFile(filepath.Join(dir, "args.txt"))     //nolint:gosec // corpus du dépôt
		stderr, err2 := os.ReadFile(filepath.Join(dir, "stderr.txt")) //nolint:gosec // corpus du dépôt
		code, err3 := os.ReadFile(filepath.Join(dir, "exit-code.txt"))
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("%s : cas incomplet", dir)
		}
		n, _ := strconv.Atoi(strings.TrimSpace(string(code)))
		out = append(out, timedCase{strings.Fields(string(args)), string(stderr), n})
	}
	return out
}

// explainOnce rejoue tout le travail d'explain pour un cas : chargement du catalogue,
// nettoyage, masquage, analyse, diagnostic et rendu en couleur.
func explainOnce(tc timedCase) error {
	cat, err := catalog.Default()
	if err != nil {
		return err
	}
	stderr := redact.Redact(sanitize.Clean(tc.stderr))
	cmd := command.Parse(redact.RedactArgs(tc.args))
	d := diagnose.Diagnose(diagnose.Input{Command: cmd, ExitCode: tc.code, Stderr: stderr, InvokedAs: "kubectl"}, cat)
	var b bytes.Buffer
	return render.RenderExplanation(&b, d, stderr, render.Options{Theme: render.NewTheme(true, true), Width: 80, ExitCode: tc.code})
}

// TestExplainLatency mesure l'objectif interne de SC-003 (moins de 200 ms au p95, l'exigence
// étant de 2 s) sur tout le corpus (D-LL-012). La mesure se fait sur le binaire normal : sous
// -race, elle est ignorée, et la CI la lance dans une étape sans -race.
func TestExplainLatency(t *testing.T) {
	if raceEnabled {
		t.Skip("latence mesurée sans le détecteur de courses (étape dédiée de la CI)")
	}
	cases := loadTimedCases(t)
	var durations []time.Duration
	for round := 0; round < 3; round++ {
		for _, tc := range cases {
			start := time.Now()
			if err := explainOnce(tc); err != nil {
				t.Fatal(err)
			}
			durations = append(durations, time.Since(start))
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)*95/100]
	t.Logf("explain : p95 = %v sur %d exécutions", p95, len(durations))
	if p95 > 200*time.Millisecond {
		t.Errorf("p95 = %v > 200 ms (objectif interne de SC-003)", p95)
	}
}

func BenchmarkExplain(b *testing.B) {
	cases := loadTimedCases(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := explainOnce(cases[i%len(cases)]); err != nil {
			b.Fatal(err)
		}
	}
}
