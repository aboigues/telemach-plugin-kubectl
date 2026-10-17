package pitfall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// namespaceFixture réunit les Services et EndpointSlices de plusieurs cas du corpus, comme un
// namespace où ils coexistent.
type namespaceFixture struct {
	*fixture
	cases []string
}

func (n namespaceFixture) ListServices() (kuberead.ServiceList, error) {
	var out kuberead.ServiceList
	for _, c := range n.cases {
		var s kuberead.Service
		data, _ := os.ReadFile(filepath.Join(objectsCorpus, c, "service.json")) //nolint:gosec // corpus du dépôt
		if err := json.Unmarshal(data, &s); err != nil {
			return out, err
		}
		out.Items = append(out.Items, s)
	}
	return out, nil
}

func (n namespaceFixture) ListEndpointSlices() (kuberead.EndpointSliceList, error) {
	var out kuberead.EndpointSliceList
	for _, c := range n.cases {
		var l kuberead.EndpointSliceList
		data, _ := os.ReadFile(filepath.Join(objectsCorpus, c, "endpointslices.json")) //nolint:gosec // corpus du dépôt
		if err := json.Unmarshal(data, &l); err != nil {
			return out, err
		}
		out.Items = append(out.Items, l.Items...)
	}
	return out, nil
}

// FR-015 : sans argument, les Services sans endpoint prêt sont listés, avec la bonne leçon ; un
// Service sain ou sans selector ne l'est pas.
func TestServiceSummaries(t *testing.T) {
	f := &fixture{Dir: filepath.Join(objectsCorpus, "healthy")}
	ns := namespaceFixture{fixture: f, cases: []string{"selector-typo", "not-ready", "healthy", "no-selector"}}
	ctx := testContext(t, f, timeOf(t, f.Dir))
	ctx.Reader = ns
	pods, err := f.ListPods()
	if err != nil {
		t.Fatal(err)
	}
	got, denied := Summaries(ctx, pods.Items, MaxSummaries)
	want := []ObjectSummary{
		{Kind: KindService, Name: "api", Pitfall: ServicePodsNotReady},
		{Kind: KindService, Name: "web", Pitfall: ServiceNoMatchingPods},
	}
	if diff := cmp.Diff(want, got); diff != "" || len(denied) > 0 {
		t.Errorf("objets en difficulté (-attendu +obtenu) :\n%s %v", diff, denied)
	}
	if got, _ := Summaries(ctx, pods.Items, 1); len(got) != 1 {
		t.Errorf("limite non respectée : %v", got)
	}
	f.Denied = map[string]bool{"services.json": true}
	ctx.Reader = f
	if _, denied := Summaries(ctx, pods.Items, MaxSummaries); len(denied) != 1 {
		t.Errorf("lecture refusée non signalée : %v", denied)
	}
}
