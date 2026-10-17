package pitfall

import (
	"sort"
	"sync"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// MaxSummaries est le nombre maximal d'objets de la liste courte de diagnose sans argument
// (FR-015).
const MaxSummaries = 10

// summarizers repèrent, pour un type d'objet, les objets en difficulté du namespace ; chaque
// fichier de diagnostic y ajoute le sien.
var summarizers []func(ctx Context, pods []kuberead.Pod) ([]ObjectSummary, error)

// Summaries lance en parallèle les repérages de chaque type d'objet (research.md S8) et renvoie
// au plus limit objets en difficulté, triés par type puis par nom, ainsi que les lectures
// refusées. pods sont les pods du namespace, déjà lus pour le diagnostic des pods.
func Summaries(ctx Context, pods []kuberead.Pod, limit int) ([]ObjectSummary, []string) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var out []ObjectSummary
	var denied ObjectSnapshot
	for _, f := range summarizers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := f(ctx, pods)
			mu.Lock()
			defer mu.Unlock()
			out = append(out, list...)
			denied.Deny(err)
		}()
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:max(limit, 0)]
	}
	return out, denied.DeniedReads
}

func init() {
	summarizers = append(summarizers, serviceSummaries, deploymentSummaries)
}

// deploymentSummaries repère les Deployments dont le rollout est bloqué.
func deploymentSummaries(ctx Context, _ []kuberead.Pod) ([]ObjectSummary, error) {
	deps, err := ctx.Reader.ListDeployments()
	if err != nil {
		return nil, err
	}
	var out []ObjectSummary
	for _, d := range deps.Items {
		desired := 1
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		if rolloutPhase(d, desired) == PhaseStalled {
			out = append(out, ObjectSummary{Kind: KindDeployment, Name: d.Metadata.Name, Pitfall: RolloutStalled})
		}
	}
	return out, nil
}

// serviceSummaries repère les Services à selector dont aucun endpoint n'est prêt : aucun pod
// désigné, ou des pods désignés mais pas prêts.
func serviceSummaries(ctx Context, pods []kuberead.Pod) ([]ObjectSummary, error) {
	var services kuberead.ServiceList
	var slices kuberead.EndpointSliceList
	var errSvc, errEps error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); services, errSvc = ctx.Reader.ListServices() }()
	go func() { defer wg.Done(); slices, errEps = ctx.Reader.ListEndpointSlices() }()
	wg.Wait()
	if errSvc != nil {
		return nil, errSvc
	}
	if errEps != nil {
		return nil, errEps
	}
	ready := map[string]int{}
	for _, e := range slices.Items {
		ready[e.ServiceName()] += e.ReadyEndpoints()
	}
	var out []ObjectSummary
	for _, svc := range services.Items {
		if len(svc.Spec.Selector) == 0 || svc.Spec.Type == "ExternalName" || ready[svc.Metadata.Name] > 0 {
			continue
		}
		id := ServiceNoMatchingPods
		for _, p := range pods {
			if matches(svc.Spec.Selector, p.Metadata.Labels) {
				id = ServicePodsNotReady
				break
			}
		}
		out = append(out, ObjectSummary{Kind: KindService, Name: svc.Metadata.Name, Pitfall: id})
	}
	return out, nil
}

// matches indique si des labels satisfont un selector d'égalités.
func matches(selector, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
