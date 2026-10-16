package command

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

func TestParse(t *testing.T) {
	type want struct {
		Sub         string
		Type        string
		Names       []string
		Namespace   string
		Context     string
		Kubeconfig  string
		Files       []string
		Recursive   bool
		DryRun      bool
		Interactive bool
	}
	tests := []struct {
		line string
		want want
	}{
		{"get pods -n demo", want{Sub: "get", Type: "pods", Namespace: "demo"}},
		{"-n demo get pods", want{Sub: "get", Type: "pods", Namespace: "demo"}},
		{"--context=kind-x get po web", want{Sub: "get", Type: "po", Names: []string{"web"}, Context: "kind-x"}},
		{"--kubeconfig /tmp/kc --namespace=demo get deploy", want{Sub: "get", Type: "deploy", Namespace: "demo", Kubeconfig: "/tmp/kc"}},
		{"-ndemo get pdos", want{Sub: "get", Type: "pdos", Namespace: "demo"}},
		{"get pod/web", want{Sub: "get", Type: "pod", Names: []string{"web"}}},
		{"get pod/web svc/api", want{Sub: "get", Type: "pod", Names: []string{"web", "api"}}},
		{"apply -f a.yaml -f dir/ -R", want{Sub: "apply", Files: []string{"a.yaml", "dir/"}, Recursive: true}},
		{"apply -f -", want{Sub: "apply", Files: []string{"-"}}},
		{"apply -f https://example.org/x.yaml", want{Sub: "apply", Files: []string{"https://example.org/x.yaml"}}},
		{"apply -k .", want{Sub: "apply"}},
		{"create deployment web --image=nginxinc/nginx-unprivileged:1.30-alpine", want{Sub: "create deployment", Type: "deployment", Names: []string{"web"}}},
		{"create -f deploy.yaml", want{Sub: "create", Files: []string{"deploy.yaml"}}},
		{"run web --image=x --dry-run=client", want{Sub: "run", Type: "pod", Names: []string{"web"}, DryRun: true}},
		{"run web --image=x --dry-run=none", want{Sub: "run", Type: "pod", Names: []string{"web"}}},
		{"config use-context kind-x", want{Sub: "config use-context"}},
		{"config current-context", want{Sub: "config current-context"}},
		{"rollout status deploy/web", want{Sub: "rollout status", Type: "deploy", Names: []string{"web"}}},
		{"set image deploy/web web=nginx:1.30", want{Sub: "set image", Type: "deploy", Names: []string{"web"}}},
		{"scale deployment web --replicas=3", want{Sub: "scale", Type: "deployment", Names: []string{"web"}}},
		{"expose deployment web --port=80", want{Sub: "expose", Type: "deployment", Names: []string{"web"}}},
		{"describe pod web", want{Sub: "describe", Type: "pod", Names: []string{"web"}}},
		{"delete -f deploy.yaml", want{Sub: "delete", Files: []string{"deploy.yaml"}}},
		{"diff -f deploy.yaml", want{Sub: "diff", Files: []string{"deploy.yaml"}}},
		{"logs web -c app --previous", want{Sub: "logs", Type: "pod", Names: []string{"web"}}},
		{"logs -f web", want{Sub: "logs", Type: "pod", Names: []string{"web"}, Interactive: true}},
		{"logs deploy/web", want{Sub: "logs", Type: "deploy", Names: []string{"web"}}},
		{"exec -it web -- sh", want{Sub: "exec", Type: "pod", Names: []string{"web"}, Interactive: true}},
		{"exec -ti web -- sh", want{Sub: "exec", Type: "pod", Names: []string{"web"}, Interactive: true}},
		{"exec -i -t web -- sh", want{Sub: "exec", Type: "pod", Names: []string{"web"}, Interactive: true}},
		{"exec web -- ls", want{Sub: "exec", Type: "pod", Names: []string{"web"}}},
		{"run -it debug --image=busybox --rm -- sh", want{Sub: "run", Type: "pod", Names: []string{"debug"}, Interactive: true}},
		{"get pods -w", want{Sub: "get", Type: "pods", Interactive: true}},
		{"get pods --watch", want{Sub: "get", Type: "pods", Interactive: true}},
		{"port-forward svc/web 8080:80", want{Sub: "port-forward", Type: "svc", Names: []string{"web"}, Interactive: true}},
		{"edit deploy web", want{Sub: "edit", Type: "deploy", Names: []string{"web"}, Interactive: true}},
		{"attach web", want{Sub: "attach", Type: "pod", Names: []string{"web"}, Interactive: true}},
		{"label pod web app=x", want{Sub: "label", Type: "pod", Names: []string{"web"}}},
		{"telemach diagnose -n demo", want{Sub: "telemach", Namespace: "demo"}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			c := Parse(strings.Fields(tt.line))
			got := want{c.Sub(), c.ResourceType, c.ResourceNames, c.Namespace, c.Context, c.Kubeconfig, c.Files, c.Recursive, c.DryRun, c.Interactive}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse(%q) (-attendu +obtenu) :\n%s", tt.line, diff)
			}
		})
	}
}

func TestParseKustomizeAndRereadable(t *testing.T) {
	c := Parse([]string{"apply", "-k", "."})
	if c.Kustomize != "." || !c.HasUnrereadableFiles() {
		t.Errorf("-k mal reconnu : %+v", c)
	}
	c = Parse([]string{"apply", "-f", "a.yaml", "-f", "-", "-f", "https://x/y.yaml"})
	if diff := cmp.Diff([]string{"a.yaml"}, c.LocalFiles()); diff != "" {
		t.Errorf("LocalFiles (-attendu +obtenu) :\n%s", diff)
	}
	if !c.HasUnrereadableFiles() {
		t.Error("-f - et URL doivent être signalés comme non relisibles")
	}
}

func TestParseUnknownFlags(t *testing.T) {
	c := Parse([]string{"get", "po", "--nmespace", "demo"})
	if diff := cmp.Diff([]string{"--nmespace"}, c.UnknownFlags); diff != "" {
		t.Errorf("UnknownFlags (-attendu +obtenu) :\n%s", diff)
	}
}

func TestParseAfterDash(t *testing.T) {
	c := Parse([]string{"exec", "web", "--", "ls", "-n", "x"})
	if c.Namespace != "" {
		t.Errorf("les options après -- appartiennent à la commande du conteneur : %q", c.Namespace)
	}
	if diff := cmp.Diff([]string{"ls", "-n", "x"}, c.AfterDash); diff != "" {
		t.Errorf("AfterDash (-attendu +obtenu) :\n%s", diff)
	}
}

func TestParseRedactedSecret(t *testing.T) {
	args := redact.RedactArgs([]string{"create", "secret", "generic", "s", "--from-literal=password=x", "--token", "abc"})
	c := Parse(args)
	if c.Sub() != "create secret generic" || len(c.ResourceNames) != 1 || c.ResourceNames[0] != "s" {
		t.Errorf("commande mal analysée : %+v", c)
	}
	for _, v := range c.Flags {
		if v == "x" || v == "abc" {
			t.Errorf("valeur sensible non masquée dans Flags : %v", c.Flags)
		}
	}
}
