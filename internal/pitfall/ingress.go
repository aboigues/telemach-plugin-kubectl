package pitfall

import (
	"strconv"
	"strings"
	"sync"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un Ingress (catalogue : internal/catalog/data/ingress.yaml).
const (
	IngressNoController  = "ingress.no-controller"
	IngressUnknownClass  = "ingress.unknown-class"
	IngressNoClass       = "ingress.no-class"
	IngressBackendAbsent = "ingress.backend-missing"
	IngressBackendPort   = "ingress.backend-port"
	IngressNoAddress     = "ingress.no-address"
)

// maxBackends limite les Services lus pour un Ingress (research.md S6).
const maxBackends = 10

func init() {
	knownIDs = append(knownIDs, IngressNoController, IngressUnknownClass, IngressNoClass,
		IngressBackendAbsent, IngressBackendPort, IngressNoAddress)
	diagnosers[KindIngress] = DiagnoseIngress
	summarizers = append(summarizers, ingressSummaries)
}

// backend est un Service visé par une règle d'un Ingress.
type backend struct {
	service, path string
	number        int
	portName      string
}

// DiagnoseIngress vérifie la classe d'un Ingress, les Services qu'il vise puis l'adresse
// attribuée par son contrôleur (research.md S6).
func DiagnoseIngress(ctx Context, name string) (ObjectSnapshot, error) {
	ing, err := ctx.Reader.GetIngress(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindIngress, name)
	s.Captures["ingress"] = name
	classOK := classLink(ctx, &s, ing)
	backendLink(ctx, &s, ing)
	if !classOK {
		s.AddLink(StepAddress, StateSkipped)
		return s, nil
	}
	var addrs []string
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		addrs = append(addrs, lb.IP+lb.Hostname)
	}
	if len(addrs) == 0 {
		s.AddLink(StepAddress, StateOK, Observation{Value: "aucune adresse attribuée"})
		s.Add(Pitfall{ID: IngressNoAddress, Step: StepAddress, Severity: Warning})
		return s, nil
	}
	s.AddLink(StepAddress, StateOK, Observation{Value: strings.Join(addrs, ", ")})
	return s, nil
}

// ingressClass détermine la classe effective d'un Ingress : ingressClassName, sinon l'ancienne
// annotation, sinon la classe par défaut. id est le piège de classe ("" si une classe existante
// le prend en charge) ; requested et source disent quelle classe a été demandée et comment.
func ingressClass(ing kuberead.Ingress, classes []kuberead.IngressClass) (id, requested, source string, class *kuberead.IngressClass) {
	switch {
	case ing.Spec.IngressClassName != nil:
		requested, source = *ing.Spec.IngressClassName, "ingressClassName"
	case ing.Metadata.Annotations.IngressClass != "":
		requested, source = ing.Metadata.Annotations.IngressClass, "annotation kubernetes.io/ingress.class (dépréciée)"
	}
	if len(classes) == 0 {
		return IngressNoController, requested, source, nil
	}
	for i, c := range classes {
		if (requested != "" && c.Metadata.Name == requested) || (requested == "" && c.Metadata.Annotations.DefaultIngressClass) {
			return "", requested, source, &classes[i]
		}
	}
	if requested != "" {
		return IngressUnknownClass, requested, source, nil
	}
	return IngressNoClass, "", "", nil
}

// classLink vérifie la classe effective. Elle renvoie faux si aucun contrôleur ne prendra
// l'Ingress.
func classLink(ctx Context, s *ObjectSnapshot, ing kuberead.Ingress) bool {
	classes, err := ctx.Reader.ListIngressClasses()
	if err != nil {
		s.Deny(err)
		s.AddLink(StepClass, StateUnknown)
		return false
	}
	var list []string
	for _, c := range classes.Items {
		label := c.Metadata.Name
		if c.Metadata.Annotations.DefaultIngressClass {
			label += " (par défaut)"
		}
		list = append(list, label)
	}
	s.Captures["classes"] = strings.Join(list, ", ")
	id, requested, source, class := ingressClass(ing, classes.Items)
	s.Captures["class"] = requested
	switch {
	case id == IngressNoController:
		s.AddLink(StepClass, StateOK, Observation{Value: "aucune classe d'Ingress dans le cluster"})
	case id == IngressUnknownClass:
		s.AddLink(StepClass, StateOK, Observation{Label: source, Value: requested + " : inconnue"})
	case id == IngressNoClass:
		s.AddLink(StepClass, StateOK, Observation{Value: "aucune classe demandée, aucune par défaut"})
	case requested == "":
		s.AddLink(StepClass, StateOK, Observation{Label: "classe par défaut", Value: class.Metadata.Name})
		s.Captures["class"] = class.Metadata.Name
	default:
		s.AddLink(StepClass, StateOK, Observation{Label: source, Value: requested + " (contrôleur " + class.Spec.Controller + ")"})
	}
	if id != "" {
		s.Add(Pitfall{ID: id, Step: StepClass, Severity: Warning})
		return false
	}
	s.Captures["controller"] = class.Spec.Controller
	return true
}

// ingressSummaries repère les Ingress qu'aucune classe existante ne prend en charge (FR-015).
func ingressSummaries(ctx Context, _ []kuberead.Pod) ([]ObjectSummary, error) {
	var ings kuberead.IngressList
	var classes kuberead.IngressClassList
	var errIng, errCls error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ings, errIng = ctx.Reader.ListIngresses() }()
	go func() { defer wg.Done(); classes, errCls = ctx.Reader.ListIngressClasses() }()
	wg.Wait()
	if errIng != nil {
		return nil, errIng
	}
	if errCls != nil {
		return nil, errCls
	}
	var out []ObjectSummary
	for _, ing := range ings.Items {
		if id, requested, _, _ := ingressClass(ing, classes.Items); id != "" {
			out = append(out, ObjectSummary{Kind: KindIngress, Name: ing.Metadata.Name, Pitfall: id,
				Captures: map[string]string{"class": requested}})
		}
	}
	return out, nil
}

// backendLink vérifie que chaque Service visé existe et expose le port demandé.
func backendLink(ctx Context, s *ObjectSnapshot, ing kuberead.Ingress) {
	var backends []backend
	seen := map[string]bool{}
	addBackend := func(b kuberead.IngressBackend, path string) {
		if b.Service == nil || len(backends) == maxBackends {
			return
		}
		key := b.Service.Name + ":" + strconv.Itoa(b.Service.Port.Number) + b.Service.Port.Name
		if !seen[key] {
			seen[key] = true
			backends = append(backends, backend{service: b.Service.Name, path: path, number: b.Service.Port.Number, portName: b.Service.Port.Name})
		}
	}
	if ing.Spec.DefaultBackend != nil {
		addBackend(*ing.Spec.DefaultBackend, "défaut")
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			addBackend(p.Backend, rule.Host+p.Path)
		}
	}
	if len(backends) == 0 {
		s.AddLink(StepBackend, StateOK, Observation{Value: "aucun backend"})
		return
	}
	// Premier backend : la leçon propose de tester l'application par son Service.
	s.Captures["service"], s.Captures["backendPort"] = backends[0].service, backends[0].portName
	if backends[0].portName == "" {
		s.Captures["backendPort"] = strconv.Itoa(backends[0].number)
	}
	state := StateOK
	var obs []Observation
	var pitfalls []Pitfall
	for _, b := range backends {
		port := b.portName
		if port == "" {
			port = strconv.Itoa(b.number)
		}
		caps := map[string]string{"service": b.service, "path": b.path, "backendPort": port}
		svc, err := ctx.Reader.GetService(b.service)
		switch {
		case kuberead.IsNotFound(err):
			obs = append(obs, Observation{Label: b.path, Value: "service/" + b.service + " : introuvable"})
			pitfalls = append(pitfalls, Pitfall{ID: IngressBackendAbsent, Step: StepBackend, Severity: Warning, Captures: caps})
		case err != nil:
			s.Deny(err)
			state = StateUnknown
			obs = append(obs, Observation{Label: b.path, Value: "service/" + b.service + " : non lu"})
		case !exposes(svc, b):
			caps["servicePorts"] = servicePorts(svc)
			obs = append(obs, Observation{Label: b.path, Value: "service/" + b.service + " : port " + port + " non exposé"})
			pitfalls = append(pitfalls, Pitfall{ID: IngressBackendPort, Step: StepBackend, Severity: Warning, Captures: caps})
		default:
			obs = append(obs, Observation{Label: b.path, Value: "service/" + b.service + ":" + port})
		}
	}
	s.AddLink(StepBackend, state, obs...)
	for _, p := range pitfalls {
		s.Add(p)
	}
}

func exposes(svc kuberead.Service, b backend) bool {
	for _, p := range svc.Spec.Ports {
		if (b.portName != "" && p.Name == b.portName) || (b.portName == "" && p.Port == b.number) {
			return true
		}
	}
	return false
}

func servicePorts(svc kuberead.Service) string {
	var parts []string
	for _, p := range svc.Spec.Ports {
		part := strconv.Itoa(p.Port)
		if p.Name != "" {
			part += " (" + p.Name + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}
