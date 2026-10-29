package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const corpus = "../../testdata/manifests/"

// FR-017 : codes de sortie 0 (aucun problème), 1 (problème), 2 (fichier introuvable).
func TestCheckExitCodes(t *testing.T) {
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"-f", corpus + "valid"}, 0, "✔ Aucun problème détecté"},
		{[]string{"-f", corpus + "unknown-field/deploy.yaml"}, 1, "imagePullPolicy"},
		{[]string{"-f", corpus + "absent.yaml"}, 2, "absent.yaml"},
		{[]string{"-f", corpus + "valid/deploy.yaml", "-f", corpus + "wrong-level/deploy.yaml"}, 1, "Problème 1/1"},
		// Issue #12 : une ressource personnalisée, un template ou des valeurs Helm, une
		// configuration de kubeadm sont des informations, pas des problèmes.
		{[]string{"-f", corpus + "unknown-crd/widget.yaml"}, 0, "ℹ Information"},
		{[]string{"-f", corpus + "helm-template/deployment.yaml"}, 0, "helm template"},
		{[]string{"-f", corpus + "helm-values/traefik-values.yaml"}, 0, "Fichier values de Helm"},
		{[]string{"-f", corpus + "tool-config/kubeadm-config.yaml"}, 0, "kubeadm avec --config"},
		{[]string{"-f", corpus + "unknown-crd/widget.yaml", "-f", corpus + "wrong-level/deploy.yaml"}, 1, "Problème 1/1"},
	}
	for _, tt := range tests {
		a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_VERSION": "v1.37.1"})
		code := run(a, append([]string{"check", "--color=never"}, tt.args...))
		if code != tt.code || !strings.Contains(errb.String(), tt.want) {
			t.Errorf("check %v : code %d (attendu %d)\n%s", tt.args, code, tt.code, errb)
		}
		if !strings.Contains(errb.String(), "Règles Kubernetes 1.37") {
			t.Errorf("la version de règles doit être indiquée (FR-014a) :\n%s", errb)
		}
	}
}

// FR-017a : sans -f, aucun fichier n'est lu, même si le dossier courant contient un
// manifeste ; le message explique l'option et le code vaut 2.
func TestCheckWithoutFile(t *testing.T) {
	dir := t.TempDir()
	trap := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(trap, []byte("piège: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd) //nolint:errcheck
	before, _ := os.Stat(trap)
	a, _, errb := newTestApp(t, nil)
	code := run(a, []string{"check", "--color=never"})
	if code != 2 || !strings.Contains(errb.String(), "Kubernetes n'a pas de fichier de manifeste par défaut") || !strings.Contains(errb.String(), "Rien n'a été lu") {
		t.Errorf("code %d :\n%s", code, errb)
	}
	if strings.Contains(errb.String(), "piège") {
		t.Error("le manifeste du dossier courant ne doit pas être lu")
	}
	after, _ := os.Stat(trap)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("le fichier ne doit pas être modifié")
	}
}

// -R parcourt les sous-dossiers ; sans -R, ils sont ignorés.
func TestCheckRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sous-dossier")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(corpus + "unknown-field/deploy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deploy.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	valid, _ := os.ReadFile(corpus + "valid/service.yaml")
	if err := os.WriteFile(filepath.Join(dir, "service.yaml"), valid, 0o600); err != nil {
		t.Fatal(err)
	}
	a, _, _ := newTestApp(t, nil)
	if code := run(a, []string{"check", "-f", dir}); code != 0 {
		t.Errorf("sans -R, le sous-dossier est ignoré : code %d", code)
	}
	a, _, _ = newTestApp(t, nil)
	if code := run(a, []string{"check", "-f", dir, "-R"}); code != 1 {
		t.Errorf("avec -R, le manifeste du sous-dossier est vérifié : code %d", code)
	}
}

// Lecture seule et aucun accès au cluster : check n'exécute jamais kubectl (un faux kubectl
// placé en tête du PATH échouerait bruyamment s'il était appelé).
func TestCheckReadOnlyNoCluster(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "appelé")
	fake := "#!/bin/sh\ntouch " + marker + "\nexit 99\n"
	if err := os.WriteFile(filepath.Join(bin, "kubectl"), []byte(fake), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	target := corpus + "unknown-field/deploy.yaml"
	before, _ := os.Stat(target)
	a, _, _ := newTestApp(t, nil)
	run(a, []string{"check", "-f", target})
	if _, err := os.Stat(marker); err == nil {
		t.Error("check ne doit jamais appeler kubectl")
	}
	after, _ := os.Stat(target)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("le manifeste ne doit pas être modifié")
	}
}

// Après une vérification, last réaffiche les problèmes (manifestes réanalysés).
func TestCheckThenLast(t *testing.T) {
	a, _, errb := newTestApp(t, map[string]string{"TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"})
	run(a, []string{"check", "--color=never", "-f", corpus + "invalid-enum/pod.yaml"})
	errb.Reset()
	a.now = func() time.Time { return time.Date(2026, 10, 16, 12, 5, 0, 0, time.UTC) }
	if code := run(a, []string{"last", "--color=never"}); code != 0 || !strings.Contains(errb.String(), "Always") {
		t.Errorf("last après check : code %d\n%s", code, errb)
	}
}
