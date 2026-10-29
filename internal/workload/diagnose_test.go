package workload_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
	. "github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

const podsDir = "../../testdata/pods"

// fixture rejoue les lectures relevées sur kind pour un pod (testdata/pods/<cas>).
type fixture struct {
	dir    string
	denied map[string]bool
}

func (f fixture) PodEvents(string) (kuberead.EventList, error) {
	var l kuberead.EventList
	if f.denied["events"] {
		return l, &kuberead.ErrDenied{What: "les événements"}
	}
	data, err := os.ReadFile(filepath.Join(f.dir, "events.json")) //nolint:gosec // corpus du dépôt
	if err != nil || len(data) == 0 {
		return l, err
	}
	return l, json.Unmarshal(data, &l)
}

// ListNodes rejoue la lecture des nœuds (feature 004) quand le cas en contient une.
func (f fixture) ListNodes() (kuberead.NodeList, error) {
	var l kuberead.NodeList
	if f.denied["nodes"] {
		return l, &kuberead.ErrDenied{What: "les nœuds"}
	}
	data, err := os.ReadFile(filepath.Join(f.dir, "nodes.json")) //nolint:gosec // corpus du dépôt
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(data, &l)
}

func (f fixture) Logs(_, _ string, previous bool) ([]string, error) {
	if f.denied["logs"] {
		return nil, &kuberead.ErrDenied{What: "le journal"}
	}
	name := "logs.txt"
	if previous {
		name = "logs-previous.txt"
	}
	data, err := os.ReadFile(filepath.Join(f.dir, name)) //nolint:gosec // corpus du dépôt
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out, nil
}

type podCase struct {
	name string
	pod  kuberead.Pod
	exp  struct {
		State  string `yaml:"state"`
		Reason string `yaml:"reason"`
		Entry  string `yaml:"entry"`
	}
}

func loadPodCases(t *testing.T) []podCase {
	t.Helper()
	dirs, _ := filepath.Glob(filepath.Join(podsDir, "*", "pod.json"))
	var out []podCase
	for _, p := range dirs {
		dir := filepath.Dir(p)
		c := podCase{name: filepath.Base(dir)}
		data, err := os.ReadFile(p) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &c.pod); err != nil {
			t.Fatalf("%s : %v", c.name, err)
		}
		exp, err := os.ReadFile(filepath.Join(dir, "expected.yaml")) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(exp, &c.exp); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

// captureTime estime l'heure du relevé : les états ont été lus 120 s après la création.
func captureTime(p kuberead.Pod) time.Time {
	return p.Metadata.CreationTimestamp.Add(125 * time.Second)
}

// TestPodCorpus mesure SC-009 : état reconnu et bonne entrée pour au moins 90 % des pods en
// échec ; un pod sain ou en cours de création n'est jamais signalé.
func TestPodCorpus(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := loadPodCases(t)
	if len(cases) < 14 {
		t.Fatalf("corpus incomplet : %d pods", len(cases))
	}
	failing, ok := 0, 0
	for _, c := range cases {
		snap, d := Diagnose(fixture{dir: filepath.Join(podsDir, c.name)}, c.pod, kuberead.Scope{Namespace: "demo"}, cat, captureTime(c.pod))
		if c.exp.State != StateFailing {
			if snap.State == StateFailing {
				t.Errorf("%s : pod %s signalé à tort comme en difficulté (%s)", c.name, c.exp.State, snap.Reason)
			}
			continue
		}
		failing++
		entry := ""
		if d.Entry != nil {
			entry = d.Entry.ID
		}
		if snap.State == StateFailing && snap.Reason == c.exp.Reason && entry == c.exp.Entry {
			ok++
			continue
		}
		t.Logf("✖ %s : état %s, raison %s, entrée %s ; attendu %s, %s", c.name, snap.State, snap.Reason, entry, c.exp.Reason, c.exp.Entry)
	}
	rate := float64(ok) / float64(failing)
	t.Logf("SC-009 : %d/%d pods en échec correctement diagnostiqués (%.0f %%)", ok, failing, rate*100)
	if rate < 0.9 {
		t.Errorf("SC-009 non atteint : %.0f %% < 90 %%", rate*100)
	}
}

// Aucune valeur de variable d'environnement ni d'annotation n'apparaît dans le diagnostic, son
// rendu ou l'état enregistré (SC-006).
func TestPodNoSecretLeak(t *testing.T) {
	cat, _ := catalog.Default()
	for _, c := range loadPodCases(t) {
		snap, d := Diagnose(fixture{dir: filepath.Join(podsDir, c.name)}, c.pod, kuberead.Scope{Namespace: "demo"}, cat, captureTime(c.pod))
		var b strings.Builder
		_ = render.RenderPod(&b, snap, d, render.Options{Theme: render.NewTheme(false, true), Width: 80})
		saved, _ := json.Marshal(state.LastDiagnosis{Pod: mustJSON(snap)})
		for _, out := range []string{fmt.Sprintf("%#v", snap), b.String(), string(saved)} {
			if strings.Contains(out, "s3cret-fake") {
				t.Errorf("%s : valeur sensible présente", c.name)
			}
		}
	}
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

// FR-026 : une lecture refusée rend le diagnostic partiel, avec la lecture nommée.
func TestPodDeniedReads(t *testing.T) {
	cat, _ := catalog.Default()
	for _, c := range loadPodCases(t) {
		if c.name != "crash" {
			continue
		}
		snap, d := Diagnose(fixture{dir: filepath.Join(podsDir, c.name), denied: map[string]bool{"events": true, "logs": true}}, c.pod, kuberead.Scope{}, cat, captureTime(c.pod))
		if len(snap.DeniedReads) != 2 || string(d.Confidence) != "partial" || snap.Reason != ReasonCrashLoopBackOff {
			t.Errorf("diagnostic partiel attendu : %+v, %s", snap.DeniedReads, d.Confidence)
		}
	}
}

// Sélection : pods en échec seulement, du plus récent au plus ancien (FR-022).
func TestSelectFailing(t *testing.T) {
	var pods []kuberead.Pod
	var latest time.Time
	for _, c := range loadPodCases(t) {
		pods = append(pods, c.pod)
		if ct := captureTime(c.pod); ct.After(latest) {
			latest = ct
		}
	}
	sel := SelectFailing(pods, latest)
	for i, s := range sel {
		if s.Pod.Metadata.Name == "healthy" || s.Pod.Metadata.Name == "starting" {
			t.Errorf("pod %s sélectionné à tort", s.Pod.Metadata.Name)
		}
		if i > 0 && s.Pod.Metadata.CreationTimestamp.After(sel[i-1].Pod.Metadata.CreationTimestamp) {
			t.Error("les pods doivent être triés du plus récent au plus ancien")
		}
	}
	if len(sel) < 12 {
		t.Errorf("au moins 12 pods en échec attendus, %d sélectionnés", len(sel))
	}
}
