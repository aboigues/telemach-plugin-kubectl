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

// readCase lit un fichier JSON d'un cas du corpus.
func readCase[T any](t *testing.T, dir, file string) T {
	t.Helper()
	var v T
	data, err := os.ReadFile(filepath.Join(objectsCorpus, dir, file)) //nolint:gosec // corpus du dépôt
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// FR-015 : sans argument, les Ingress sans classe prise en charge et les PVC dont aucune classe
// ne fournira le volume sont listés ; un PVC qui attend normalement son premier pod ne l'est pas.
func TestIngressPVCSummaries(t *testing.T) {
	ingCases := []string{"ingress-no-address", "ingress-unknown-class", "ingress-no-default-class"}
	pvcCases := []string{"pvc-waiting", "pvc-unknown-class", "pvc-empty-class"}
	var ings kuberead.IngressList
	for _, c := range ingCases {
		ings.Items = append(ings.Items, readCase[kuberead.Ingress](t, c, "ingress.json"))
	}
	var pvcs kuberead.PVCList
	for _, c := range pvcCases {
		pvcs.Items = append(pvcs.Items, readCase[kuberead.PVC](t, c, "pvc.json"))
	}
	f := &summaryFixture{
		fixture: &fixture{Dir: filepath.Join(objectsCorpus, "healthy")},
		ings:    ings, pvcs: pvcs,
		ingClasses: readCase[kuberead.IngressClassList](t, "ingress-unknown-class", "ingressclasses.json"),
		scClasses:  readCase[kuberead.StorageClassList](t, "pvc-waiting", "storageclasses.json"),
	}
	ctx := testContext(t, f.fixture, timeOf(t, f.Dir))
	ctx.Reader = f
	got, _ := Summaries(ctx, nil, MaxSummaries)
	want := []ObjectSummary{
		{Kind: KindIngress, Name: "typo", Pitfall: IngressUnknownClass},
		{Kind: KindIngress, Name: "web", Pitfall: IngressNoClass},
		{Kind: KindPVC, Name: "fast-data", Pitfall: PVCUnknownClass},
		{Kind: KindPVC, Name: "static-data", Pitfall: PVCEmptyClass},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("objets en difficulté (-attendu +obtenu) :\n%s", diff)
	}
}

// summaryFixture sert des listes d'Ingress, de PVC et de classes construites par le test.
type summaryFixture struct {
	*fixture
	ings       kuberead.IngressList
	pvcs       kuberead.PVCList
	ingClasses kuberead.IngressClassList
	scClasses  kuberead.StorageClassList
}

func (s *summaryFixture) ListIngresses() (kuberead.IngressList, error) { return s.ings, nil }
func (s *summaryFixture) ListIngressClasses() (kuberead.IngressClassList, error) {
	return s.ingClasses, nil
}
func (s *summaryFixture) ListPVCs() (kuberead.PVCList, error) { return s.pvcs, nil }
func (s *summaryFixture) ListStorageClasses() (kuberead.StorageClassList, error) {
	return s.scClasses, nil
}
