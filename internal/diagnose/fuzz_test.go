package diagnose

import (
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

// FuzzMatch soumet au catalogue des messages et des commandes arbitraires : jamais de
// panique, et aucune capture ne contient de valeur que le masquage aurait retirée.
func FuzzMatch(f *testing.F) {
	cat, err := catalog.Default()
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range []string{
		`error: the server doesn't have a resource type "pdos"`,
		`Error from server (Forbidden): pods is forbidden: User "u" cannot list resource "pods" in API group "" in the namespace "demo"`,
		`error: unknown flag: --token=fake-token`,
		"The Pod \"x\" is invalid: \n* metadata.name: Invalid value: \"X_1\": a lowercase RFC 1123",
		"command terminated with exit code 137",
	} {
		f.Add(seed, "get pods --token fake-token -n demo")
	}
	f.Fuzz(func(t *testing.T, stderr, line string) {
		args := redact.RedactArgs(strings.Fields(line))
		d := Diagnose(Input{Command: command.Parse(args), ExitCode: 1, Stderr: redact.Redact(stderr), InvokedAs: "kubectl"}, cat)
		for k, v := range d.Captures {
			if redact.Redact(v) != v {
				t.Fatalf("capture %s non masquée : %q", k, v)
			}
		}
		if d.Entry != nil && d.Confidence == Unknown {
			t.Fatal("une entrée reconnue ne peut pas être « unknown »")
		}
	})
}
