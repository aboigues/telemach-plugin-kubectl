package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

var update = flag.Bool("update", false, "réécrit les fichiers de référence (golden files)")

// goldenCase rejoue une erreur du quickstart (US1) et compare le rendu à sa référence.
type goldenCase struct {
	name, line, stderr string
	exitCode           int
	repeated           bool
}

var us1Cases = []goldenCase{
	{name: "v1-type-mal-orthographie", line: "get deploymnet -n demo", stderr: `error: the server doesn't have a resource type "deploymnet"` + "\n", exitCode: 1},
	{name: "v2-mauvais-namespace", line: "get pod web", stderr: `Error from server (NotFound): pods "web" not found` + "\n", exitCode: 1},
	{name: "v3-cluster-injoignable", line: "get pods", stderr: "The connection to the server localhost:8080 was refused - did you specify the right host or port?\n", exitCode: 1},
	{name: "v4-acces-refuse", line: "get pods -n demo --as=system:serviceaccount:demo:default", stderr: `Error from server (Forbidden): pods is forbidden: User "system:serviceaccount:demo:default" cannot list resource "pods" in API group "" in the namespace "demo"` + "\n", exitCode: 1},
	{name: "v5-option-avant-plugin", line: "-n demo telemach diagnose", stderr: "Error: flags cannot be placed before plugin name: -n\n", exitCode: 1},
	{name: "v6-non-reconnue", line: "get pods", stderr: "error: something totally new\n", exitCode: 1},
	{name: "v17-repetition", line: "get pdos", stderr: `error: the server doesn't have a resource type "pdos"` + "\n", exitCode: 1, repeated: true},
	{name: "exec-application", line: "exec web -n demo -- sh -c exit_3", stderr: "command terminated with exit code 3\n", exitCode: 3},
}

func TestExplanationGolden(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, gc := range us1Cases {
		for _, color := range []bool{false, true} {
			suffix := "nocolor"
			if color {
				suffix = "color"
			}
			t.Run(gc.name+"/"+suffix, func(t *testing.T) {
				args := strings.Fields(strings.ReplaceAll(gc.line, "exit_3", "exit-3"))
				cmd := command.Parse(redact.RedactArgs(args))
				d := diagnose.Diagnose(diagnose.Input{Command: cmd, ExitCode: gc.exitCode, Stderr: gc.stderr, InvokedAs: "kubectl"}, cat)
				var b bytes.Buffer
				opts := Options{Theme: NewTheme(color, true), Width: 80, KubectlVersion: "v1.37.1", Repeated: gc.repeated, ExitCode: gc.exitCode}
				if err := RenderExplanation(&b, d, gc.stderr, opts); err != nil {
					t.Fatal(err)
				}
				checkGolden(t, filepath.Join("us1", gc.name+"."+suffix+".txt"), b.String())
				if !color && strings.Contains(b.String(), "\x1b") {
					t.Error("aucune séquence ESC attendue sans couleur")
				}
			})
		}
	}
}

// checkGolden compare got au fichier de référence testdata/golden/<name> (ou le réécrit
// avec -update).
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // référence du dépôt
	if err != nil {
		t.Fatalf("référence absente (lancer go test -update) : %v", err)
	}
	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("rendu différent de %s (-attendu +obtenu) :\n%s", name, diff)
	}
}
