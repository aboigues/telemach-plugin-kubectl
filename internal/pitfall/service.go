package pitfall

import (
	"sort"
	"strconv"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/textdist"
)

// Pièges d'un Service (catalogue : internal/catalog/data/service.yaml).
const (
	ServiceNoMatchingPods  = "service.no-matching-pods"
	ServicePodsNotReady    = "service.pods-not-ready"
	ServiceTargetPortName  = "service.targetport-name"
	ServiceTargetPortNum   = "service.targetport-number"
	ServicePortsUndeclared = "service.ports-undeclared"
	ServiceNoSelector      = "service.no-selector"
	ServiceExternalName    = "service.external-name"
	ServiceHealthy         = "service.healthy"
)

// maxClosest est le nombre maximal de groupes de labels proches proposés (FR-002).
const maxClosest = 3

func init() {
	knownIDs = append(knownIDs, ServiceNoMatchingPods, ServicePodsNotReady, ServiceTargetPortName,
		ServiceTargetPortNum, ServicePortsUndeclared, ServiceNoSelector, ServiceExternalName, ServiceHealthy)
	diagnosers[KindService] = DiagnoseService
}

// DiagnoseService suit le chemin du trafic d'un Service (research.md S4) : type, selector,
// pods prêts, endpoints, ports. Le premier maillon cassé porte la leçon.
func DiagnoseService(ctx Context, name string) (ObjectSnapshot, error) {
	svc, err := ctx.Reader.GetService(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindService, name)
	typ := svc.Spec.Type
	if typ == "" {
		typ = "ClusterIP"
	}
	s.Facts = append(s.Facts, Observation{Value: typ})
	if ports := portSummary(svc); ports != "" {
		s.Facts = append(s.Facts, Observation{Label: "ports", Value: ports})
	}
	s.Captures["service"] = name
	if len(svc.Spec.Ports) > 0 {
		s.Captures["port"] = strconv.Itoa(svc.Spec.Ports[0].Port)
	}

	s.AddLink(StepType, StateOK, Observation{Value: typ})
	if typ == "ExternalName" {
		for _, st := range []Step{StepSelector, StepReadiness, StepEndpoints, StepPorts} {
			s.AddLink(st, StateSkipped)
		}
		s.Add(Pitfall{
			ID: ServiceExternalName, Step: StepType, Severity: Info,
			Captures: map[string]string{"external": svc.Spec.ExternalName},
		})
		return s, nil
	}
	if len(svc.Spec.Selector) == 0 {
		s.AddLink(StepSelector, StateSkipped, Observation{Value: "aucun selector"})
		s.AddLink(StepReadiness, StateSkipped)
		ready := endpointsLink(ctx, &s, name)
		s.AddLink(StepPorts, StateSkipped)
		s.Add(Pitfall{
			ID: ServiceNoSelector, Step: StepType, Severity: Info,
			Captures: map[string]string{"ready": strconv.Itoa(ready)},
		})
		return s, nil
	}

	sel := selectorString(svc.Spec.Selector)
	s.Captures["selector"] = sel
	pods, err := ctx.Reader.PodsBySelector(sel)
	if err != nil {
		s.Deny(err)
		s.AddLink(StepSelector, StateUnknown, Observation{Label: sel, Value: "non lu"})
		s.AddLink(StepReadiness, StateSkipped)
		endpointsLink(ctx, &s, name)
		s.AddLink(StepPorts, StateSkipped)
		return s, nil
	}
	if len(pods.Items) == 0 {
		s.AddLink(StepSelector, StateOK, Observation{Label: sel, Value: "aucun pod"})
		s.AddLink(StepReadiness, StateSkipped)
		endpointsLink(ctx, &s, name)
		s.AddLink(StepPorts, StateSkipped)
		s.Add(Pitfall{
			ID: ServiceNoMatchingPods, Step: StepSelector, Severity: Warning,
			Captures: map[string]string{"closest": closestLabels(ctx, &s, sel)},
		})
		return s, nil
	}
	s.AddLink(StepSelector, StateOK, Observation{Label: sel, Value: plural(len(pods.Items), "pod")})

	var notReady, readyPods []string
	for _, p := range pods.Items {
		if podReady(p) {
			readyPods = append(readyPods, p.Metadata.Name)
		} else {
			notReady = append(notReady, p.Metadata.Name)
		}
	}
	s.AddLink(StepReadiness, StateOK, Observation{Value: strconv.Itoa(len(readyPods)) + "/" + strconv.Itoa(len(pods.Items)) + " prêt(s)"})
	if len(readyPods) == 0 {
		s.Add(Pitfall{ID: ServicePodsNotReady, Step: StepReadiness, Severity: Warning, Captures: map[string]string{
			"pods": strconv.Itoa(len(pods.Items)), "notready": firstNames(notReady), "pod": notReady[0],
		}})
	}

	ready := endpointsLink(ctx, &s, name)
	portsLink(&s, svc, pods.Items)
	if len(s.Pitfalls) == 0 {
		s.Add(Pitfall{
			ID: ServiceHealthy, Step: StepType, Severity: OK,
			Captures: map[string]string{"ready": strconv.Itoa(ready)},
		})
	}
	return s, nil
}

// endpointsLink lit les EndpointSlices du Service et renvoie le nombre d'endpoints prêts. Un
// écart avec les pods prêts est montré tel quel, sans cause supposée.
func endpointsLink(ctx Context, s *ObjectSnapshot, service string) int {
	slices, err := ctx.Reader.ServiceEndpointSlices(service)
	if err != nil {
		s.Deny(err)
		s.AddLink(StepEndpoints, StateUnknown)
		return 0
	}
	ready := 0
	for _, e := range slices.Items {
		ready += e.ReadyEndpoints()
	}
	s.AddLink(StepEndpoints, StateOK, Observation{Value: strconv.Itoa(ready) + " prêt(s)"})
	return ready
}

// portsLink compare chaque targetPort aux ports déclarés par les conteneurs des pods désignés
// (FR-003). Un port non déclaré n'est jamais présenté comme une erreur.
func portsLink(s *ObjectSnapshot, svc kuberead.Service, pods []kuberead.Pod) {
	names := map[string]int{}
	numbers := map[int]bool{}
	for _, p := range pods {
		for _, c := range p.Spec.Containers {
			for _, cp := range c.Ports {
				numbers[cp.ContainerPort] = true
				if cp.Name != "" {
					names[cp.Name] = cp.ContainerPort
				}
			}
		}
	}
	declared := declaredPorts(names, numbers)
	state := StateOK
	var obs []Observation
	var pitfalls []Pitfall
	for _, sp := range svc.Spec.Ports {
		target := sp.TargetPort
		if !target.IsNamed() && target.Int == 0 {
			target.Int = sp.Port // targetPort absent : même numéro que port
		}
		caps := map[string]string{"port": strconv.Itoa(sp.Port), "targetPort": target.String(), "declared": declared, "pod": pods[0].Metadata.Name}
		switch {
		case target.IsNamed():
			if n, ok := names[target.Str]; ok {
				obs = append(obs, Observation{Value: strconv.Itoa(sp.Port) + " → " + target.Str + " (" + strconv.Itoa(n) + ")"})
				continue
			}
			state = StateBroken
			obs = append(obs, Observation{Value: strconv.Itoa(sp.Port) + " → " + target.Str + " : introuvable"})
			pitfalls = append(pitfalls, Pitfall{ID: ServiceTargetPortName, Step: StepPorts, Severity: Warning, Captures: caps})
		case len(numbers) == 0:
			if state == StateOK {
				state = StateUnknown
			}
			obs = append(obs, Observation{Value: strconv.Itoa(sp.Port) + " → " + target.String() + " : aucun port déclaré"})
			pitfalls = append(pitfalls, Pitfall{ID: ServicePortsUndeclared, Step: StepPorts, Severity: Info, Captures: caps})
		case numbers[target.Int]:
			obs = append(obs, Observation{Value: strconv.Itoa(sp.Port) + " → " + target.String()})
		default:
			state = StateBroken
			caps["declaredPort"] = strconv.Itoa(firstNumber(numbers))
			obs = append(obs, Observation{Value: strconv.Itoa(sp.Port) + " → " + target.String() + " : non déclaré"})
			pitfalls = append(pitfalls, Pitfall{ID: ServiceTargetPortNum, Step: StepPorts, Severity: Warning, Captures: caps})
		}
	}
	if state == StateBroken {
		state = StateOK // Add() casse le maillon pour chaque avertissement
	}
	s.AddLink(StepPorts, state, obs...)
	for _, p := range pitfalls {
		s.Add(p)
	}
}

// closestLabels propose au plus maxClosest groupes de labels de pods du namespace, du plus
// proche au plus éloigné du selector (FR-002). Une lecture refusée laisse la liste vide.
func closestLabels(ctx Context, s *ObjectSnapshot, sel string) string {
	all, err := ctx.Reader.ListPods()
	if err != nil {
		s.Deny(err)
		return ""
	}
	seen := map[string]bool{}
	var sets []string
	for _, p := range all.Items {
		labels := map[string]string{}
		for k, v := range p.Metadata.Labels {
			if k != "pod-template-hash" && !strings.HasPrefix(k, "controller-revision-hash") {
				labels[k] = v
			}
		}
		if set := selectorString(labels); set != "" && !seen[set] {
			seen[set] = true
			sets = append(sets, set)
		}
	}
	sort.SliceStable(sets, func(i, j int) bool {
		return textdist.Distance(sel, sets[i]) < textdist.Distance(sel, sets[j])
	})
	if len(sets) > maxClosest {
		sets = sets[:maxClosest]
	}
	return strings.Join(sets, " ; ")
}

func podReady(p kuberead.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

// selectorString écrit des labels sous la forme « clé=valeur,… », clés triées.
func selectorString(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ",")
}

func portSummary(svc kuberead.Service) string {
	var parts []string
	for _, p := range svc.Spec.Ports {
		t := p.TargetPort.String()
		if !p.TargetPort.IsNamed() && p.TargetPort.Int == 0 {
			t = strconv.Itoa(p.Port)
		}
		part := strconv.Itoa(p.Port) + " → " + t
		if p.NodePort != 0 {
			part += " (nodePort " + strconv.Itoa(p.NodePort) + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func declaredPorts(names map[string]int, numbers map[int]bool) string {
	if len(numbers) == 0 {
		return "aucun"
	}
	byNumber := map[int]string{}
	for n, p := range names {
		byNumber[p] = n
	}
	var nums []int
	for n := range numbers {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
		if name := byNumber[n]; name != "" {
			parts[i] += " (" + name + ")"
		}
	}
	return strings.Join(parts, ", ")
}

func firstNumber(numbers map[int]bool) int {
	first := 0
	for n := range numbers {
		if first == 0 || n < first {
			first = n
		}
	}
	return first
}

func firstNames(names []string) string {
	if len(names) > maxClosest {
		return strings.Join(names[:maxClosest], ", ") + "…"
	}
	return strings.Join(names, ", ")
}

func plural(n int, word string) string {
	if n > 1 {
		return strconv.Itoa(n) + " " + word + "s"
	}
	return strconv.Itoa(n) + " " + word
}
