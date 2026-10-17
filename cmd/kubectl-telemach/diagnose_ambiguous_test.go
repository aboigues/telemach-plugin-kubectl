package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeObjects est un faux kubectl qui ne connaît que les objets de FAKE_EXISTS
// (« pod/web service/hello deployment/hello ») et journalise ses appels.
const fakeObjects = `#!/bin/sh
echo "$*" >> "$CALL_LOG"
kind="$2"; name="$3"
case " $FAKE_EXISTS " in
*" $kind/$name "*)
  case "$kind" in
  pod) printf '{"metadata":{"name":"%s","creationTimestamp":"2026-10-17T10:00:00Z"},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}' "$name" ;;
  service) printf '{"metadata":{"name":"%s"},"spec":{"type":"ClusterIP","selector":{"app":"%s"},"ports":[{"port":80,"targetPort":8080}]}}' "$name" "$name" ;;
  deployment) printf '{"metadata":{"name":"%s"},"spec":{"selector":{"matchLabels":{"app":"%s"}}}}' "$name" "$name" ;;
  esac ;;
*)
  if [ "$kind" = "pods" ] || [ "$kind" = "endpointslices" ]; then printf '{"items":[]}'; exit 0; fi
  echo "Error from server (NotFound): $kind \"$name\" not found" >&2; exit 1 ;;
esac
`

func objectsEnv(t *testing.T, exists string) (map[string]string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte(fakeObjects), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	log := filepath.Join(dir, "calls.log")
	t.Setenv("CALL_LOG", log)
	t.Setenv("FAKE_EXISTS", exists)
	return map[string]string{"TELEMACH_KUBECTL": bin, "TELEMACH_KUBECTL_SHELL": "zsh:9.9.9"}, log
}

// T025, FR-016 : un nom sans type qui désigne plusieurs objets donne la liste des objets et
// des commandes à copier, sans choix arbitraire.
func TestDiagnoseAmbiguousName(t *testing.T) {
	env, _ := objectsEnv(t, "service/hello deployment/hello")
	a, _, errb := newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never", "hello", "-n", "demo"}); code != 2 {
		t.Fatalf("code %d\n%s", code, errb)
	}
	for _, want := range []string{"plusieurs objets", "kubectl telemach diagnose service/hello -n demo", "kubectl telemach diagnose deployment/hello -n demo"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("sortie sans %q :\n%s", want, errb)
		}
	}
}

// Un seul objet trouvé : diagnostic direct (un pod reste diagnostiqué comme dans la feature
// 001) ; aucun : message d'objet introuvable, code 2.
func TestDiagnoseSingleOrNone(t *testing.T) {
	env, log := objectsEnv(t, "service/hello")
	a, _, errb := newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never", "hello", "-n", "demo"}); code != 0 {
		t.Fatalf("Service seul : code %d\n%s", code, errb)
	}
	if !strings.Contains(errb.String(), "Service « hello »") {
		t.Errorf("diagnostic du Service attendu :\n%s", errb)
	}
	data, _ := os.ReadFile(log) //nolint:gosec // fichier de test
	for _, c := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !strings.HasPrefix(c, "get ") {
			t.Errorf("appel interdit : %q", c)
		}
	}
	env, _ = objectsEnv(t, "")
	a, _, errb = newTestApp(t, env)
	if code := run(a, []string{"diagnose", "--color=never", "absent", "-n", "demo"}); code != 2 || !strings.Contains(errb.String(), "introuvable") {
		t.Errorf("aucun objet : code %d\n%s", code, errb)
	}
}
