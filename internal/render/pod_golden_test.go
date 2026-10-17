package render

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

const podsCorpus = "../../testdata/pods"

// podFixture rejoue les lectures relevées sur kind pour un pod (testdata/pods/<cas>).
type podFixture string

func (f podFixture) PodEvents(string) (kuberead.EventList, error) {
	var l kuberead.EventList
	data, err := os.ReadFile(filepath.Join(string(f), "events.json")) //nolint:gosec // corpus du dépôt
	if err != nil || len(data) == 0 {
		return l, err
	}
	return l, json.Unmarshal(data, &l)
}

func (f podFixture) Logs(_, _ string, previous bool) ([]string, error) {
	name := "logs.txt"
	if previous {
		name = "logs-previous.txt"
	}
	data, err := os.ReadFile(filepath.Join(string(f), name)) //nolint:gosec // corpus du dépôt
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

// TestPodGolden fige le rendu de diagnose (US4, V13) pour chaque pod du corpus relevé sur
// kind : il sert de référence aux contrôles de langue et de masquage (SC-005, SC-006).
func TestPodGolden(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(podsCorpus, "*", "pod.json"))
	if len(files) == 0 {
		t.Fatal("corpus de pods vide (D-LL-009)")
	}
	for _, f := range files {
		dir := filepath.Dir(f)
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(f) //nolint:gosec // corpus du dépôt
			if err != nil {
				t.Fatal(err)
			}
			var p kuberead.Pod
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			// Les états ont été relevés 120 s après la création (voir testdata/pods).
			now := p.Metadata.CreationTimestamp.Add(125e9)
			snap, d := workload.Diagnose(podFixture(dir), p, kuberead.Scope{Namespace: "demo"}, cat, now)
			var b bytes.Buffer
			if err := RenderPod(&b, snap, d, Options{Theme: NewTheme(false, true), Width: 80}); err != nil {
				t.Fatal(err)
			}
			checkGolden(t, filepath.Join("us4", name+".nocolor.txt"), b.String())
		})
	}
}
