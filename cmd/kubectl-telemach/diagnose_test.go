package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// fakeCluster est un faux kubectl qui rejoue les pods relevés sur kind (testdata/pods) et
// journalise ses appels : diagnose et watch ne doivent rien faire d'autre que ces lectures.
const fakeCluster = `#!/bin/sh
echo "$*" >> "$CALL_LOG"
case "$FAKE_MODE" in
refused) echo 'The connection to the server localhost:8080 was refused - did you specify the right host or port?' >&2; exit 1 ;;
esac
case "$1 $2" in
"get pods")
  if [ "$3" = "-l" ]; then cat "$FAKE_DIR/selected.json"; else cat "$FAKE_DIR/list.json"; fi ;;
"get pod")
  if [ -f "$FAKE_PODS/$3/pod.json" ]; then cat "$FAKE_PODS/$3/pod.json"; else echo "Error from server (NotFound): pods \"$3\" not found" >&2; exit 1; fi ;;
"get deployment") printf '{"metadata":{"name":"%s"},"spec":{"selector":{"matchLabels":{"app":"web"}}}}' "$3" ;;
"get events")
  name=$(echo "$4" | sed 's/.*involvedObject.name=//')
  if [ "$FAKE_MODE" = "denied" ]; then echo 'Error from server (Forbidden): events is forbidden: User "u" cannot list resource "events"' >&2; exit 1; fi
  cat "$FAKE_PODS/$name/events.json" ;;
"logs "*)
  case "$*" in *--previous*) f=logs-previous.txt ;; *) f=logs.txt ;; esac
  cat "$FAKE_PODS/$2/$f" ;;
*) echo "appel inattendu : $*" >&2; exit 99 ;;
esac
`

const podsCorpus = "../../testdata/pods"

// fakeEnv installe le faux kubectl et prépare la liste des pods ; il renvoie l'heure du relevé.
func fakeEnv(t *testing.T, names ...string) (map[string]string, string, time.Time) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte(fakeCluster), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(podsCorpus)
	var items []json.RawMessage
	var latest time.Time
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(podsCorpus, n, "pod.json"))
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, data)
		var p kuberead.Pod
		_ = json.Unmarshal(data, &p)
		if ct := p.Metadata.CreationTimestamp.Add(125 * time.Second); ct.After(latest) {
			latest = ct
		}
	}
	list, _ := json.Marshal(map[string]any{"items": items})
	for _, f := range []string{"list.json", "selected.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), list, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(dir, "calls.log")
	t.Setenv("CALL_LOG", log)
	t.Setenv("FAKE_DIR", dir)
	t.Setenv("FAKE_PODS", abs)
	return map[string]string{"TELEMACH_KUBECTL": bin, "TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"}, log, latest
}

func readCalls(t *testing.T, log string) []string {
	t.Helper()
	data, _ := os.ReadFile(log) //nolint:gosec // fichier de test
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// Quickstart V13 : sans argument, explication du pod le plus récent puis liste courte des
// autres ; seules les lectures prévues sont faites.
func TestDiagnoseNamespace(t *testing.T) {
	env, log, at := fakeEnv(t, "crash", "oom", "bad-image", "healthy", "starting")
	a, _, errb := newTestApp(t, env)
	a.now = func() time.Time { return at }
	if code := run(a, []string{"diagnose", "--color=never", "-n", "demo"}); code != 0 {
		t.Fatalf("code %d\n%s", code, errb)
	}
	got := errb.String()
	for _, want := range []string{"Autres pods en difficulté", "kubectl telemach diagnose pod/", "-n demo", "📘 Concept"} {
		if !strings.Contains(got, want) {
			t.Errorf("sortie sans %q :\n%s", want, got)
		}
	}
	for _, healthy := range []string{"• healthy", "• starting"} {
		if strings.Contains(got, healthy) {
			t.Errorf("pod sain listé : %q", healthy)
		}
	}
	for _, c := range readCalls(t, log) {
		if !strings.HasPrefix(c, "get ") && !strings.HasPrefix(c, "logs ") {
			t.Errorf("appel interdit : %q", c)
		}
		if !strings.Contains(c, "--request-timeout=10s") || !strings.Contains(c, "-n demo") {
			t.Errorf("portée ou délai manquants : %q", c)
		}
	}
}

// US4 scénarios 1 et 2 : un pod précis, avec son journal (≤ 20 lignes) et sans secret.
func TestDiagnosePod(t *testing.T) {
	env, _, at := fakeEnv(t, "crash")
	a, _, errb := newTestApp(t, env)
	a.now = func() time.Time { return at }
	if code := run(a, []string{"diagnose", "--color=never", "pod/crash", "-n", "demo"}); code != 0 {
		t.Fatalf("code %d\n%s", code, errb)
	}
	got := errb.String()
	if os.Getenv("TELEMACH_SHOW") != "" {
		t.Log("\n" + got)
	}
	for _, want := range []string{"redémarre en boucle", "code 1", "la variable DB_URL est obligatoire", "📜 Fin du journal", "kubectl logs crash -c app -n demo --previous"} {
		if !strings.Contains(got, want) {
			t.Errorf("sortie sans %q :\n%s", want, got)
		}
	}
	if strings.Contains(got, "s3cret-fake") {
		t.Error("valeur d'environnement affichée (SC-006)")
	}
	// last réaffiche le diagnostic sans relire le cluster (FR-027).
	t.Setenv("FAKE_MODE", "refused")
	errb.Reset()
	if code := run(a, []string{"last", "--color=never"}); code != 0 || !strings.Contains(errb.String(), "redémarre en boucle") {
		t.Errorf("last : code %d\n%s", code, errb)
	}
}

// FR-026 : lecture des événements refusée → diagnostic partiel nommant la lecture.
func TestDiagnoseDenied(t *testing.T) {
	env, _, at := fakeEnv(t, "missing-secret")
	t.Setenv("FAKE_MODE", "denied")
	a, _, errb := newTestApp(t, env)
	a.now = func() time.Time { return at }
	run(a, []string{"diagnose", "--color=never", "pod/missing-secret", "-n", "demo"})
	if got := errb.String(); !strings.Contains(got, "Diagnostic partiel") || !strings.Contains(got, "les événements du pod") || !strings.Contains(got, "api-absent") {
		t.Errorf("diagnostic partiel attendu :\n%s", got)
	}
}

func TestDiagnoseNoFailingPod(t *testing.T) {
	env, _, at := fakeEnv(t, "healthy", "starting")
	a, _, errb := newTestApp(t, env)
	a.now = func() time.Time { return at }
	if code := run(a, []string{"diagnose", "--color=never", "-n", "demo"}); code != 0 || !strings.Contains(errb.String(), "Aucun pod en difficulté dans le namespace « demo »") {
		t.Errorf("code %d\n%s", code, errb)
	}
}

func TestDiagnoseUnreachableAndNotFound(t *testing.T) {
	env, _, _ := fakeEnv(t, "crash")
	a, _, errb := newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never", "pod/crsh"}); code != 2 || !strings.Contains(errb.String(), "Nom proche : crash") {
		t.Errorf("introuvable : code %d\n%s", code, errb)
	}
	t.Setenv("FAKE_MODE", "refused")
	a, _, errb = newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never"}); code != 1 || !strings.Contains(errb.String(), "localhost:8080") || !strings.Contains(errb.String(), "kubeconfig") {
		t.Errorf("injoignable : code %d\n%s", code, errb)
	}
}

// FR-028, FR-029 : watch ne lit que le statut des pods ciblés et dépose un signal par pod en
// échec ; rien pour un pod sain.
func TestWatchSignals(t *testing.T) {
	env, log, at := fakeEnv(t, "bad-image", "healthy")
	runtime := t.TempDir()
	env["XDG_RUNTIME_DIR"] = runtime
	a, out, errb := newTestApp(t, env)
	a.now = func() time.Time { return at }
	pid := os.Getpid()
	for _, pod := range []string{"bad-image", "healthy"} {
		run(a, []string{"watch", "--delay", "0", "--shell-pid", itoa(pid), "--", "run", pod, "--image=x:1.0", "-n", "demo"})
	}
	if out.Len() != 0 || errb.Len() != 0 {
		t.Errorf("watch ne doit rien écrire sur le terminal : %q %q", out, errb)
	}
	data, err := os.ReadFile(state.PendingFile(filepath.Join(runtime, "telemach-kubectl"), pid))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "⚠ Le pod bad-image est en ImagePullBackOff : kubectl telemach diagnose pod/bad-image -n demo\n" {
		t.Errorf("signal : %q", got)
	}
	for _, c := range readCalls(t, log) {
		if !strings.HasPrefix(c, "get pod ") {
			t.Errorf("watch ne lit que le statut des pods (FR-029) : %q", c)
		}
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
