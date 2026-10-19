package pitfall

import (
	"strings"
	"sync"

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
	summarizers = append(summarizers, pvcSummaries)
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
	if pvc.Status.Phase == "Pending" {
		consumerFailing(ctx, &s, consumers)
	}
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

// storageClass détermine la classe effective d'un PVC : la classe demandée, sinon la classe par
// défaut. id est le piège de classe ("" si une classe existante fournira le volume).
func storageClass(pvc kuberead.PVC, classes []kuberead.StorageClass) (id string, class *kuberead.StorageClass) {
	requested := pvc.Spec.StorageClassName
	if requested != nil && *requested == "" {
		return PVCEmptyClass, nil
	}
	for i, c := range classes {
		if (requested != nil && c.Metadata.Name == *requested) || (requested == nil && c.Metadata.Annotations.DefaultStorageClass) {
			return "", &classes[i]
		}
	}
	if requested == nil {
		return PVCNoDefaultClass, nil
	}
	return PVCUnknownClass, nil
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
	var list []string
	for _, c := range classes.Items {
		label := c.Metadata.Name
		if c.Metadata.Annotations.DefaultStorageClass {
			label += " (par défaut)"
		}
		list = append(list, label)
	}
	s.Captures["classes"] = strings.Join(list, ", ")
	if len(list) == 0 {
		s.Captures["classes"] = "aucune"
	}
	if pvc.Spec.StorageClassName != nil {
		s.Captures["class"] = *pvc.Spec.StorageClassName
	}
	id, class := storageClass(pvc, classes.Items)
	switch {
	case id == PVCNoDefaultClass:
		s.AddLink(StepStorageClass, StateOK, Observation{Value: "aucune classe demandée, aucune par défaut"})
	case id == PVCEmptyClass:
		s.AddLink(StepStorageClass, StateOK, Observation{Value: "storageClassName vide : provisionnement dynamique désactivé"})
	case id == PVCUnknownClass:
		s.AddLink(StepStorageClass, StateOK, Observation{Value: s.Captures["class"] + " : inconnue"})
	case pvc.Spec.StorageClassName == nil:
		s.AddLink(StepStorageClass, StateOK, Observation{Label: "classe par défaut", Value: class.Metadata.Name})
		s.Captures["class"] = class.Metadata.Name
	default:
		s.AddLink(StepStorageClass, StateOK, Observation{Value: class.Metadata.Name + " (" + class.Provisioner + ")"})
	}
	if id != "" {
		s.Add(Pitfall{ID: id, Step: StepStorageClass, Severity: Warning})
		return kuberead.StorageClass{}, false
	}
	return *class, true
}

// pvcSummaries repère les PVC Pending dont aucune classe ne fournira le volume (FR-015). Une
// attente WaitForFirstConsumer est normale et n'est pas listée ; un pod consommateur en échec
// est déjà listé parmi les pods.
func pvcSummaries(ctx Context, _ []kuberead.Pod) ([]ObjectSummary, error) {
	var pvcs kuberead.PVCList
	var classes kuberead.StorageClassList
	var errPVC, errCls error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pvcs, errPVC = ctx.Reader.ListPVCs() }()
	go func() { defer wg.Done(); classes, errCls = ctx.Reader.ListStorageClasses() }()
	wg.Wait()
	if errPVC != nil {
		return nil, errPVC
	}
	if errCls != nil {
		return nil, errCls
	}
	var out []ObjectSummary
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != "Pending" {
			continue
		}
		if id, _ := storageClass(pvc, classes.Items); id != "" {
			caps := map[string]string{}
			if pvc.Spec.StorageClassName != nil {
				caps["class"] = *pvc.Spec.StorageClassName
			}
			out = append(out, ObjectSummary{Kind: KindPVC, Name: pvc.Metadata.Name, Pitfall: id, Captures: caps})
		}
	}
	return out, nil
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

// consumerFailing diagnostique, comme dans la feature 001, le pod en attente qui utilise un PVC
// non lié (research.md S7) : c'est lui qui retient le volume en mode WaitForFirstConsumer.
func consumerFailing(ctx Context, s *ObjectSnapshot, consumers []kuberead.Pod) {
	var pending []kuberead.Pod
	for _, p := range consumers {
		if p.Status.Phase == "Pending" {
			pending = append(pending, p)
		}
	}
	if s.Failing = firstFailing(ctx, pending); s.Failing == nil {
		return
	}
	s.Captures["pod"] = s.Failing.Pod.Name
	if l := s.link(StepConsumer); l != nil {
		l.State = StateBroken
	}
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
