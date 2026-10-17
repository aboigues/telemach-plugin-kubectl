package diagnose

import (
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

// firstFix rend la première correction de l'entrée reconnue, comme l'affichera explain.
func firstFix(t *testing.T, args []string, stderr, invokedAs string) (Diagnosis, string) {
	t.Helper()
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	clean := redact.RedactArgs(args)
	d := Diagnose(Input{Command: command.Parse(clean), ExitCode: 1, Stderr: redact.Redact(stderr), InvokedAs: invokedAs}, cat)
	if d.Entry == nil {
		t.Fatalf("erreur non reconnue : %q", stderr)
	}
	return d, catalog.Render(d.Entry.Fixes[0].Command, d.Captures)
}

// FR-011, FR-012 : la commande corrigée ne change que l'élément fautif et garde contexte,
// namespace et autres options.
func TestFixedCommands(t *testing.T) {
	tests := []struct {
		name, line, stderr, invokedAs, want string
	}{
		{"type mal orthographié", "get deploymnet -n demo --context kind-x", `error: the server doesn't have a resource type "deploymnet"`, "kubectl", "kubectl get deployment -n demo --context kind-x"},
		{"type avec nom", "describe pdo/web -n demo", `error: the server doesn't have a resource type "pdo"`, "kubectl", "kubectl describe pod/web -n demo"},
		{"sous-commande", "gt pods", `error: unknown command "gt" for "kubectl"`, "kubectl", "kubectl get pods"},
		{"option", "get po --nmespace demo", `error: unknown flag: --nmespace`, "kubectl", "kubectl get po --namespace demo"},
		{"option avec =", "get po --nmespace=demo -o wide", `error: unknown flag: --nmespace`, "kubectl", "kubectl get po --namespace=demo -o wide"},
		{"alias k", "get pdos -n demo", `error: the server doesn't have a resource type "pdos"`, "k", "k get pods -n demo"},
		{"option avant le plugin", "-n demo --context kind-x telemach diagnose", `Error: flags cannot be placed before plugin name: -n`, "kubectl", "kubectl telemach diagnose -n demo --context kind-x"},
		{"option obligatoire", "create deployment web -n demo", `error: required flag(s) "image" not set`, "kubectl", "kubectl create deployment web -n demo --image=IMAGE"},
		{"nom invalide", "run Web_1 --image=x -n demo", "The Pod \"Web_1\" is invalid: \n* metadata.name: Invalid value: \"Web_1\": a lowercase RFC 1123 subdomain", "kubectl", "kubectl run web-1 --image=x -n demo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := firstFix(t, strings.Fields(tt.line), tt.stderr, tt.invokedAs)
			if got != tt.want {
				t.Errorf("correction = %q, attendu %q", got, tt.want)
			}
		})
	}
}

func TestFixedCommandNoSuggestion(t *testing.T) {
	d, got := firstFix(t, []string{"get", "zzzzzzzzzz"}, `error: the server doesn't have a resource type "zzzzzzzzzz"`, "kubectl")
	if got != "kubectl get TYPE" || d.Captures["suggested_type"] != "" {
		t.Errorf("sans suggestion fiable, un repère TYPE est attendu : %q, %v", got, d.Captures)
	}
}

// Les valeurs sensibles de la commande ne réapparaissent jamais dans une correction.
func TestFixedCommandRedacted(t *testing.T) {
	_, got := firstFix(t, []string{"get", "pdos", "--token", "fake-token-123"}, `error: the server doesn't have a resource type "pdos"`, "kubectl")
	if strings.Contains(got, "fake-token-123") || !strings.Contains(got, redact.Mask) {
		t.Errorf("le jeton doit être masqué dans la correction : %q", got)
	}
}

func TestQuote(t *testing.T) {
	if got := joinArgs([]string{"exec", "web", "--", "sh", "-c", "exit 3"}); got != "exec web -- sh -c 'exit 3'" {
		t.Errorf("joinArgs = %q", got)
	}
}
