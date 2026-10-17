package main

import (
	"strings"
	"testing"
)

// T014 : types et noms courts acceptés par diagnose (contracts/cli.md § diagnose).
func TestParseTarget(t *testing.T) {
	tests := []struct {
		in, kind, name string
		ok             bool
	}{
		{"", "", "", true},
		{"web", "", "web", true},
		{"pod/web", "pod", "web", true},
		{"po/web", "pod", "web", true},
		{"service/web", "service", "web", true},
		{"svc/web", "service", "web", true},
		{"services/web", "service", "web", true},
		{"deployment/web", "deployment", "web", true},
		{"deploy/web", "deployment", "web", true},
		{"deployment.apps/web", "deployment", "web", true},
		{"ingress/web", "ingress", "web", true},
		{"ing/web", "ingress", "web", true},
		{"pvc/data", "pvc", "data", true},
		{"persistentvolumeclaim/data", "pvc", "data", true},
		{"configmap/web", "configmap", "web", false},
		{"service/", "service", "", false},
	}
	for _, tt := range tests {
		kind, name, ok := parseTarget(tt.in)
		if kind != tt.kind || name != tt.name || ok != tt.ok {
			t.Errorf("parseTarget(%q) = %q, %q, %v ; attendu %q, %q, %v", tt.in, kind, name, ok, tt.kind, tt.name, tt.ok)
		}
	}
}

// Un type non pris en charge liste les types acceptés, code 2.
func TestDiagnoseUnsupportedType(t *testing.T) {
	env, _, _ := fakeEnv(t, "crash")
	a, _, errb := newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never", "configmap/web", "-n", "demo"}); code != 2 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"configmap", "service/", "deployment/", "ingress/", "pvc/"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("message sans %q :\n%s", want, errb)
		}
	}
}
