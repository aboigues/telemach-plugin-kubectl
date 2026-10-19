package pitfall

import (
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un PersistentVolumeClaim (catalogue : internal/catalog/data/storage.yaml).
const (
	PVCUnknownClass   = "pvc.unknown-class"
	PVCEmptyClass     = "pvc.empty-class"
	PVCNoDefaultClass = "pvc.no-default-class"
	PVCWaitingFirst   = "pvc.waiting-first-consumer"
)

func init() {
	knownIDs = append(knownIDs, PVCUnknownClass, PVCEmptyClass, PVCNoDefaultClass, PVCWaitingFirst)
	diagnosers[KindPVC] = DiagnosePVC
}

// DiagnosePVC vérifie la classe de stockage d'un PVC, son mode de liaison et le pod qui
// l'utilise (research.md S7). Un PVC en attente d'un premier pod en mode WaitForFirstConsumer
// est présenté comme normal.
func DiagnosePVC(ctx Context, name string) (ObjectSnapshot, error) {
	pvc, err := ctx.Reader.GetPVC(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindPVC, name)
	s.Captures["pvc"] = name
	s.Facts = append(s.Facts, Observation{Value: pvc.Status.Phase})
	if req := pvc.Spec.Resources.Requests["storage"]; req != "" {
		s.Facts = append(s.Facts, Observation{Label: "taille", Value: req})
	}
	class, classOK := storageClassLink(ctx, &s, pvc)
	consumers := consumerLink(ctx, &s, name)
	switch {
	case !classOK:
		s.AddLink(StepBinding, StateSkipped)
	case pvc.Status.Phase == "Pending" && class.VolumeBindingMode == "WaitForFirstConsumer" && len(consumers) == 0:
		s.AddLink(StepBinding, StateOK, Observation{Value: "Pending, en attente d'un premier pod (WaitForFirstConsumer)"})
		s.Add(Pitfall{ID: PVCWaitingFirst, Step: StepBinding, Severity: Info, Transient: true})
	default:
		s.AddLink(StepBinding, StateOK, Observation{Value: pvc.Status.Phase + " (" + bindingMode(class) + ")"})
	}
	// Ordre d'affichage : classe, liaison, pod consommateur.
	s.Links = orderLinks(s.Links, StepStorageClass, StepBinding, StepConsumer)
	return s, nil
}

// storageClassLink vérifie la classe demandée (absente : classe par défaut ; vide :
// provisionnement dynamique désactivé).
func storageClassLink(ctx Context, s *ObjectSnapshot, pvc kuberead.PVC) (kuberead.StorageClass, bool) {
	classes, err := ctx.Reader.ListStorageClasses()
	if err != nil {
		s.Deny(err)
		s.AddLink(StepStorageClass, StateUnknown)
		return kuberead.StorageClass{}, false
	}
	byName := map[string]kuberead.StorageClass{}
	var list []string
	var def *kuberead.StorageClass
	for i, c := range classes.Items {
		byName[c.Metadata.Name] = c
		label := c.Metadata.Name
		if c.Metadata.Annotations.DefaultStorageClass {
			def = &classes.Items[i]
			label += " (par défaut)"
		}
		list = append(list, label)
	}
	s.Captures["classes"] = strings.Join(list, ", ")
	if len(list) == 0 {
		s.Captures["classes"] = "aucune"
	}
	switch requested := pvc.Spec.StorageClassName; {
	case requested == nil && def == nil:
		s.AddLink(StepStorageClass, StateOK, Observation{Value: "aucune classe demandée, aucune par défaut"})
		s.Add(Pitfall{ID: PVCNoDefaultClass, Step: StepStorageClass, Severity: Warning})
		return kuberead.StorageClass{}, false
	case requested == nil:
		s.AddLink(StepStorageClass, StateOK, Observation{Label: "classe par défaut", Value: def.Metadata.Name})
		return *def, true
	case *requested == "":
		s.AddLink(StepStorageClass, StateOK, Observation{Value: "storageClassName vide : provisionnement dynamique désactivé"})
		s.Add(Pitfall{ID: PVCEmptyClass, Step: StepStorageClass, Severity: Warning})
		return kuberead.StorageClass{}, false
	default:
		c, ok := byName[*requested]
		s.Captures["class"] = *requested
		if !ok {
			s.AddLink(StepStorageClass, StateOK, Observation{Value: *requested + " : inconnue"})
			s.Add(Pitfall{ID: PVCUnknownClass, Step: StepStorageClass, Severity: Warning})
			return kuberead.StorageClass{}, false
		}
		s.AddLink(StepStorageClass, StateOK, Observation{Value: *requested + " (" + c.Provisioner + ")"})
		return c, true
	}
}

// consumerLink cherche les pods du namespace qui utilisent le PVC.
func consumerLink(ctx Context, s *ObjectSnapshot, claim string) []kuberead.Pod {
	pods, err := ctx.Reader.ListPods()
	if err != nil {
		s.Deny(err)
		s.AddLink(StepConsumer, StateUnknown)
		return nil
	}
	var users []kuberead.Pod
	var names []string
	for _, p := range pods.Items {
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == claim {
				users = append(users, p)
				names = append(names, p.Metadata.Name+" ("+p.Status.Phase+")")
				break
			}
		}
	}
	if len(users) == 0 {
		s.AddLink(StepConsumer, StateOK, Observation{Value: "aucun pod"})
		return nil
	}
	s.Captures["pod"] = users[0].Metadata.Name
	s.AddLink(StepConsumer, StateOK, Observation{Value: strings.Join(names, ", ")})
	return users
}

func bindingMode(c kuberead.StorageClass) string {
	if c.VolumeBindingMode == "" {
		return "Immediate"
	}
	return c.VolumeBindingMode
}

// orderLinks remet les maillons dans l'ordre donné.
func orderLinks(links []Link, order ...Step) []Link {
	out := make([]Link, 0, len(links))
	for _, st := range order {
		for _, l := range links {
			if l.Step == st {
				out = append(out, l)
			}
		}
	}
	return out
}
