package kubeterms

import "testing"

func TestResolveType(t *testing.T) {
	tests := []struct {
		in       string
		singular string
	}{
		{"po", "pod"},
		{"pods", "pod"},
		{"Pod", "pod"},
		{"deploy", "deployment"},
		{"deployments.apps", "deployment"},
		{"svc", "service"},
		{"cm", "configmap"},
		{"ns", "namespace"},
		{"ing", "ingress"},
		{"pvc", "persistentvolumeclaim"},
		{"sts", "statefulset"},
		{"ds", "daemonset"},
		{"cj", "cronjob"},
		{"job", "job"},
		{"secret", "secret"},
	}
	for _, tt := range tests {
		rt, ok := ResolveType(tt.in)
		if !ok || rt.Singular != tt.singular {
			t.Errorf("ResolveType(%q) = %q, %v ; attendu %q", tt.in, rt.Singular, ok, tt.singular)
		}
	}
	for _, bad := range []string{"pdos", "deploymnet", "", "foo"} {
		if _, ok := ResolveType(bad); ok {
			t.Errorf("ResolveType(%q) ne devrait pas réussir", bad)
		}
	}
}

func TestClosestType(t *testing.T) {
	tests := map[string]string{
		"deploymnet": "deployment",
		"pdos":       "pods",
		"pdo":        "pod",
		"servcie":    "service",
		"configmpas": "configmaps",
		"namespcae":  "namespace",
		"ingres":     "ingress",
	}
	for in, want := range tests {
		got, ok := ClosestType(in)
		if !ok || got != want {
			t.Errorf("ClosestType(%q) = %q, %v ; attendu %q", in, got, ok, want)
		}
	}
	if got, ok := ClosestType("zzzzzzzz"); ok {
		t.Errorf("aucune suggestion attendue, obtenu %q", got)
	}
}

func TestClosestSubcommand(t *testing.T) {
	tests := map[string]string{"gt": "get", "aply": "apply", "decsribe": "describe", "lgos": "logs"}
	for in, want := range tests {
		got, ok := ClosestSubcommand(in)
		if !ok || got != want {
			t.Errorf("ClosestSubcommand(%q) = %q, %v ; attendu %q", in, got, ok, want)
		}
	}
}

func TestFlags(t *testing.T) {
	for _, name := range []string{"-n", "--namespace", "--context", "--kubeconfig", "--token", "-s", "--as", "--request-timeout"} {
		f, ok := LookupFlag(name)
		if !ok || !f.Global || !f.TakesArg {
			t.Errorf("%s devrait être une option globale avec valeur : %+v, %v", name, f, ok)
		}
	}
	if f, _ := LookupFlag("--token"); !f.Sensitive {
		t.Error("--token doit être marquée sensible")
	}
	if !ShortTakesArg('n') || !ShortTakesArg('f') || ShortTakesArg('i') || ShortTakesArg('t') {
		t.Error("ShortTakesArg incorrect pour -n, -f, -i ou -t")
	}
	if got, ok := ClosestFlag("--nmespace"); !ok || got != "--namespace" {
		t.Errorf("ClosestFlag(--nmespace) = %q, %v", got, ok)
	}
	if got, ok := ClosestFlag("--replcias"); !ok || got != "--replicas" {
		t.Errorf("ClosestFlag(--replcias) = %q, %v", got, ok)
	}
}

func TestSubSubcommands(t *testing.T) {
	if !contains(SubSubcommands("config"), "use-context") || !contains(SubSubcommands("rollout"), "status") || !contains(SubSubcommands("set"), "image") {
		t.Error("sous-commandes de second niveau manquantes")
	}
	if !IsSubcommand("port-forward") || IsSubcommand("telemach") {
		t.Error("IsSubcommand incorrect")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
