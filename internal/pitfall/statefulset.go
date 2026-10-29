package pitfall

import (
	"strconv"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un StatefulSet (feature 004, research.md F8 ; catalogue : batch.yaml).
const (
	StatefulSetNoService   = "statefulset.no-service"
	StatefulSetPVCPending  = "statefulset.pvc-pending"
	StatefulSetOrderedWait = "statefulset.ordered-wait"
	StatefulSetOK          = "statefulset.ok"
)

func init() {
	knownIDs = append(knownIDs, StatefulSetNoService, StatefulSetPVCPending, StatefulSetOrderedWait, StatefulSetOK)
	diagnosers[KindStatefulSet] = DiagnoseStatefulSet
}

// DiagnoseStatefulSet vérifie, dans cet ordre, le Service headless de serviceName, les PVC des
// volumeClaimTemplates (un PVC en attente bloque son pod), puis le démarrage ordonné des pods
// (OrderedReady : le pod k+1 n'est créé que quand le pod k est prêt).
func DiagnoseStatefulSet(ctx Context, name string) (ObjectSnapshot, error) {
	sts, err := ctx.Reader.GetStatefulSet(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindStatefulSet, name)
	s.Captures["statefulset"] = name
	desired := sts.ReplicasOrDefault()
	policy := sts.PodManagementPolicyOrDefault()
	caps := map[string]string{"service": sts.Spec.ServiceName, "desired": strconv.Itoa(desired), "policy": policy}
	s.Facts = append(s.Facts, Observation{Value: strconv.Itoa(sts.Status.ReadyReplicas) + "/" + strconv.Itoa(desired) + " prêt(s)"},
		Observation{Label: "podManagementPolicy", Value: policy})

	// Service headless.
	if sts.Spec.ServiceName == "" {
		s.AddLink(StepService, StateSkipped)
	} else {
		svc, serr := ctx.Reader.GetService(sts.Spec.ServiceName)
		s.Deny(serr)
		switch {
		case kuberead.IsNotFound(serr):
			s.AddLink(StepService, StateOK, Observation{Value: sts.Spec.ServiceName + " : absent"})
			s.Add(Pitfall{ID: StatefulSetNoService, Step: StepService, Severity: Warning, Captures: caps})
		case serr != nil:
			s.AddLink(StepService, StateUnknown)
		default:
			s.AddLink(StepService, StateOK, Observation{Value: sts.Spec.ServiceName + " (" + svc.Spec.Type + ")"})
		}
	}

	sel := selectorString(sts.Spec.Selector.MatchLabels)
	pods, perr := ctx.Reader.PodsBySelector(sel)
	s.Deny(perr)

	// Volumes : PVC <modèle>-<nom>-<k>.
	pvcBlocked := map[string]bool{} // pods bloqués par un PVC en attente
	if len(sts.Spec.VolumeClaimTemplates) == 0 {
		s.AddLink(StepVolumes, StateOK, Observation{Value: "aucun volumeClaimTemplates"})
	} else {
		pvcs, verr := ctx.Reader.ListPVCs()
		s.Deny(verr)
		s.AddLink(StepVolumes, LinkStateFor(verr))
		if verr == nil {
			for _, p := range pvcs.Items {
				for _, tpl := range sts.Spec.VolumeClaimTemplates {
					prefix := tpl.Metadata.Name + "-" + name + "-"
					if !strings.HasPrefix(p.Metadata.Name, prefix) || p.Status.Phase != "Pending" {
						continue
					}
					pvcBlocked[name+"-"+strings.TrimPrefix(p.Metadata.Name, prefix)] = true
					c := map[string]string{"pvc": p.Metadata.Name}
					if p.Spec.StorageClassName != nil {
						c["class"] = *p.Spec.StorageClassName
					}
					for k, v := range caps {
						c[k] = v
					}
					s.Observe(StepVolumes, p.Metadata.Name, "Pending")
					s.Add(Pitfall{ID: StatefulSetPVCPending, Step: StepVolumes, Severity: Warning, Transient: true, Captures: c})
				}
			}
		}
	}

	// Démarrage ordonné.
	s.AddLink(StepOrdinal, LinkStateFor(perr), Observation{Value: plural(len(pods.Items), "pod") + " sur " + strconv.Itoa(desired)})
	if perr == nil {
		s.Failing = firstFailing(ctx, pods.Items)
		if policy == "OrderedReady" && len(pods.Items) < desired {
			if blocking := notReadyPod(pods.Items); blocking != "" && !pvcBlocked[blocking] {
				c := map[string]string{"blocking": blocking}
				for k, v := range caps {
					c[k] = v
				}
				if f := s.Failing; f != nil {
					c["pod"], c["reason"] = f.Pod.Name, f.Pod.Reason
				}
				s.Add(Pitfall{ID: StatefulSetOrderedWait, Step: StepOrdinal, Severity: Warning, Transient: true, Captures: c})
			}
		}
	}
	if len(s.Pitfalls) == 0 {
		s.Add(Pitfall{ID: StatefulSetOK, Step: StepService, Severity: OK, Captures: caps})
	}
	return s, nil
}

// notReadyPod renvoie le nom du premier pod (ordre des noms) qui n'est pas prêt.
func notReadyPod(pods []kuberead.Pod) string {
	best := ""
	for _, p := range pods {
		ready := false
		for _, c := range p.Status.Conditions {
			if c.Type == "Ready" && c.Status == "True" {
				ready = true
			}
		}
		if !ready && (best == "" || p.Metadata.Name < best) {
			best = p.Metadata.Name
		}
	}
	return best
}
