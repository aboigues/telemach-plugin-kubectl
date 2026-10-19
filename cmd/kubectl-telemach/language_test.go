package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/testcorpus"
)

// checkOutputs vérifie la sortie de la dernière commande, le fichier last.json et le rendu de
// « last » : aucun secret du corpus en clair (SC-006) ; textes rédigés en français (SC-005).
func checkOutputs(t *testing.T, a *app, label, output string, secrets []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.store.Dir, "last.json"))
	if err != nil {
		t.Fatalf("%s : last.json absent (D-LL-009) : %v", label, err)
	}
	errb := a.stderr.(interface {
		String() string
		Reset()
	})
	errb.Reset()
	if code := run(a, []string{"last", "--color=never"}); code != 0 {
		t.Fatalf("%s : last, code %d", label, code)
	}
	again := errb.String()
	for name, text := range map[string]string{"sortie": output, "last.json": string(data), "last": again} {
		if leaks := testcorpus.Leaks(text, secrets); len(leaks) > 0 {
			t.Errorf("%s : secret en clair dans %s : %q", label, name, leaks)
		}
	}
	for name, text := range map[string]string{"sortie": output, "last": again} {
		if words := testcorpus.EnglishWords(text); len(words) > 0 {
			t.Errorf("%s : texte anglais dans %s : %q\n%s", label, name, words, text)
		}
	}
}

// T088 : chaque cas du corpus de masquage passé comme message d'erreur et comme ligne de
// commande, puis check sur un manifeste à secret en clair et diagnose sur chaque pod relevé.
func TestCorpusLanguageAndSecrets(t *testing.T) {
	secrets, err := testcorpus.SecretValues("../../testdata")
	if err != nil || len(secrets) == 0 {
		t.Fatalf("corpus de secrets illisible ou vide (D-LL-009) : %v", err)
	}
	inputs, _ := filepath.Glob("../../testdata/secrets/*.txt")
	n := 0
	for _, in := range inputs {
		if strings.HasSuffix(in, ".expected.txt") {
			continue
		}
		n++
		raw, err := os.ReadFile(in) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatal(err)
		}
		args := []string{"get", "pods"}
		if line := strings.TrimSpace(string(raw)); strings.HasPrefix(line, "kubectl ") && !strings.Contains(line, "\n") {
			args = strings.Fields(strings.TrimPrefix(line, "kubectl "))
		}
		a, _, errb := newTestApp(t, nil)
		f := writeStderr(t, "Error from server (BadRequest): "+string(raw))
		if code := run(a, append([]string{"explain", "--color=never", "--exit-code", "1", "--stderr-file", f, "--"}, args...)); code != 0 {
			t.Fatalf("%s : code %d", in, code)
		}
		checkOutputs(t, a, filepath.Base(in), errb.String(), secrets)
	}
	if n == 0 {
		t.Fatal("aucun cas de masquage (D-LL-009)")
	}

	a, _, errb := newTestApp(t, nil)
	if code := run(a, []string{"check", "--color=never", "-f", "../../testdata/manifests/plaintext-secret/"}); code != 1 {
		t.Fatalf("check : code %d\n%s", code, errb)
	}
	checkOutputs(t, a, "check plaintext-secret", errb.String(), secrets)

	pods, _ := filepath.Glob(filepath.Join(podsCorpus, "*", "pod.json"))
	for _, p := range pods {
		name := filepath.Base(filepath.Dir(p))
		env, _, at := fakeEnv(t, name)
		a, _, errb := newTestApp(t, env)
		a.now = func() time.Time { return at }
		if code := run(a, []string{"diagnose", "--color=never", "pod/" + name, "-n", "demo"}); code != 0 {
			t.Fatalf("diagnose %s : code %d\n%s", name, code, errb)
		}
		if _, err := os.Stat(filepath.Join(a.store.Dir, "last.json")); err != nil {
			continue // pod sain ou en démarrage : rien d'enregistré
		}
		checkOutputs(t, a, "diagnose "+name, errb.String(), secrets)
	}

	// T065 : chaque objet du corpus relevé sur kind (feature 002) passé par diagnose puis last,
	// hints.json compris. La valeur du Secret de config-env.yaml s'ajoute aux secrets cherchés.
	secrets = append(secrets, "fake-token-value")
	n = 0
	for _, c := range objectCases(t) {
		a, _, _ := objectEnv(t, c.dir, c.name)
		errb := a.stderr.(interface{ String() string })
		if code := run(a, []string{"diagnose", "--color=never", c.kind + "/" + c.name, "-n", "demo"}); code != 0 {
			t.Fatalf("diagnose %s/%s (%s) : code %d\n%s", c.kind, c.name, c.dir, code, errb)
		}
		n++
		label := "diagnose " + c.kind + "/" + c.name + " (" + c.dir + ")"
		checkOutputs(t, a, label, errb.String(), secrets)
		if data, err := os.ReadFile(filepath.Join(a.store.Dir, "hints.json")); err == nil {
			if leaks := testcorpus.Leaks(string(data), secrets); len(leaks) > 0 {
				t.Errorf("%s : secret en clair dans hints.json : %q", label, leaks)
			}
		}
	}
	if n == 0 {
		t.Fatal("aucun cas d'objet (D-LL-009)")
	}
}

// objectCase est un objet attendu d'un cas de testdata/objects/ (expected*.yaml).
type objectCase struct{ dir, kind, name string }

// objectCases liste les objets du corpus d'objets, variantes de version comprises.
func objectCases(t *testing.T) []objectCase {
	t.Helper()
	dirs, _ := filepath.Glob(filepath.Join(objectsCorpus, "*"))
	var out []objectCase
	for _, dir := range dirs {
		base := filepath.Base(dir)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() || base == "scenarios" || base == "configmap-dates" {
			continue
		}
		expDir := dir
		if i := strings.Index(base, "@"); i >= 0 {
			expDir = filepath.Join(objectsCorpus, base[:i])
		}
		files, _ := filepath.Glob(filepath.Join(expDir, "expected*.yaml"))
		for _, f := range files {
			var exp struct {
				Kind, Name string
			}
			data, _ := os.ReadFile(f) //nolint:gosec // corpus du dépôt
			if err := yaml.Unmarshal(data, &exp); err != nil || exp.Kind == "" {
				t.Fatalf("%s : %v", f, err)
			}
			out = append(out, objectCase{dir: base, kind: exp.Kind, name: exp.Name})
		}
	}
	return out
}
