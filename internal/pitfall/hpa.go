package pitfall

import (
	"strconv"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un HorizontalPodAutoscaler (feature 004, research.md F10 ; catalogue : metrics.yaml).
const (
	HPAMetricsUnavailable = "hpa.metrics-unavailable"
	HPATargetMissing      = "hpa.target-missing"
	HPAMissingRequest     = "hpa.missing-request"
	HPAInactive           = "hpa.inactive"
	HPALimited            = "hpa.limited"
	HPAOtherMetrics       = "hpa.other-metrics"
	HPAOK                 = "hpa.ok"
)

func init() {
	knownIDs = append(knownIDs, HPAMetricsUnavailable, HPATargetMissing, HPAMissingRequest, HPAInactive, HPALimited, HPAOtherMetrics, HPAOK)
	diagnosers[KindHPA] = DiagnoseHPA
}

// DiagnoseHPA suit la chaîne d'un autoscaler, dans l'ordre : API de métriques disponible,
// cible existante, request déclarée par chaque conteneur pour chaque ressource visée, puis
// conditions (AbleToScale, ScalingActive, ScalingLimited). Le premier maillon cassé porte la
// leçon (FR-016).
func DiagnoseHPA(ctx Context, name string) (ObjectSnapshot, error) {
	h, err := ctx.Reader.GetHPA(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindHPA, name)
	s.Captures["hpa"] = name
	ref := h.Spec.ScaleTargetRef
	caps := map[string]string{
		"target_kind": ref.Kind, "target": ref.Name, "min": strconv.Itoa(h.MinReplicasOrDefault()),
		"max": strconv.Itoa(h.Spec.MaxReplicas), "current": strconv.Itoa(h.Status.CurrentReplicas),
	}
	s.Facts = append(s.Facts, Observation{Label: "cible", Value: ref.Kind + "/" + ref.Name},
		Observation{Label: "répliques", Value: caps["current"] + " (de " + caps["min"] + " à " + caps["max"] + ")"})

	var resources []string // ressources visées par une métrique Resource en pourcentage
	other := false
	for _, m := range h.Spec.Metrics {
		switch {
		case m.Type == "Resource" && m.Resource != nil && m.Resource.Target.Type == "Utilization":
			resources = append(resources, m.Resource.Name)
		case m.Type == "Resource":
		default:
			other = true
		}
	}
	caps["resources"] = strings.Join(resources, ", ")
	add := func(p Pitfall) {
		c := map[string]string{}
		for k, v := range caps {
			c[k] = v
		}
		for k, v := range p.Captures {
			c[k] = v
		}
		p.Captures = c
		s.Add(p)
	}

	// API de métriques (metrics-server).
	api, aerr := ctx.Reader.GetMetricsAPIService()
	s.Deny(aerr)
	switch {
	case kuberead.IsNotFound(aerr):
		s.AddLink(StepMetricsAPI, StateOK, Observation{Value: kuberead.MetricsAPIServiceName + " : absente"})
		add(Pitfall{ID: HPAMetricsUnavailable, Step: StepMetricsAPI, Severity: Warning})
	case aerr != nil:
		s.AddLink(StepMetricsAPI, StateUnknown)
	default:
		c := kuberead.FindCondition(api.Status.Conditions, "Available")
		if c != nil && c.Status == "True" {
			s.AddLink(StepMetricsAPI, StateOK, Observation{Value: kuberead.MetricsAPIServiceName + " : disponible"})
		} else {
			reason := "indisponible"
			if c != nil {
				reason = clean(c.Reason)
			}
			s.AddLink(StepMetricsAPI, StateOK, Observation{Value: kuberead.MetricsAPIServiceName + " : " + reason})
			add(Pitfall{ID: HPAMetricsUnavailable, Step: StepMetricsAPI, Severity: Warning, Captures: map[string]string{"api_reason": reason}})
		}
	}

	// Cible et requests.
	tpl, terr := hpaTarget(ctx, ref.Kind, ref.Name)
	s.Deny(terr)
	switch {
	case tpl == nil && terr == nil:
		s.AddLink(StepTarget, StateSkipped, Observation{Value: ref.Kind + " : non vérifié"})
		s.AddLink(StepRequests, StateSkipped)
	case kuberead.IsNotFound(terr):
		s.AddLink(StepTarget, StateOK, Observation{Value: ref.Kind + "/" + ref.Name + " : introuvable"})
		add(Pitfall{ID: HPATargetMissing, Step: StepTarget, Severity: Warning})
		s.AddLink(StepRequests, StateSkipped)
	case terr != nil:
		s.AddLink(StepTarget, StateUnknown)
		s.AddLink(StepRequests, StateUnknown)
	default:
		s.AddLink(StepTarget, StateOK, Observation{Value: ref.Kind + "/" + ref.Name})
		s.AddLink(StepRequests, StateOK)
		for _, c := range tpl.Spec.Containers {
			for _, res := range resources {
				if _, ok := c.Resources.Requests[res]; !ok {
					s.Observe(StepRequests, c.Name, "aucune request "+res)
					add(Pitfall{ID: HPAMissingRequest, Step: StepRequests, Severity: Warning, Captures: map[string]string{"container": c.Name, "resource": res}})
				}
			}
		}
	}

	// Conditions de l'autoscaler.
	s.AddLink(StepConditions, StateOK)
	for _, c := range h.Status.Conditions {
		s.Observe(StepConditions, c.Type, c.Status+" ("+clean(c.Reason)+")")
	}
	if limited := kuberead.FindCondition(h.Status.Conditions, "ScalingLimited"); limited != nil && limited.Status == "True" {
		add(Pitfall{ID: HPALimited, Step: StepConditions, Severity: Info, Captures: map[string]string{"limit_reason": limited.Reason}})
	}
	if s.Healthy() {
		// Une condition en échec sans cause trouvée plus haut est citée telle quelle.
		for _, t := range []string{"AbleToScale", "ScalingActive"} {
			if c := kuberead.FindCondition(h.Status.Conditions, t); c != nil && c.Status == "False" {
				add(Pitfall{ID: HPAInactive, Step: StepConditions, Severity: Warning, Captures: map[string]string{"condition": t, "condition_message": clean(c.Message)}})
				break
			}
		}
	}
	if other {
		add(Pitfall{ID: HPAOtherMetrics, Step: StepConditions, Severity: Info})
	}
	if len(s.Pitfalls) == 0 {
		add(Pitfall{ID: HPAOK, Step: StepConditions, Severity: OK})
	}
	return s, nil
}

// hpaTarget lit le modèle de pod de la cible (Deployment ou StatefulSet) ; un autre type de
// cible n'est pas vérifié (nil, nil).
func hpaTarget(ctx Context, kind, name string) (*kuberead.PodTemplate, error) {
	switch kind {
	case "Deployment":
		d, err := ctx.Reader.GetDeployment(name)
		if err != nil {
			return nil, err
		}
		return &d.Spec.Template, nil
	case "StatefulSet":
		st, err := ctx.Reader.GetStatefulSet(name)
		if err != nil {
			return nil, err
		}
		return &st.Spec.Template, nil
	}
	return nil, nil
}
