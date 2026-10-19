package pitfall

import (
	"strconv"
	"strings"

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

// classLink détermine la classe effective : ingressClassName, sinon l'ancienne annotation,
// sinon la classe par défaut. Elle renvoie faux si aucun contrôleur ne prendra l'Ingress.
func classLink(ctx Context, s *ObjectSnapshot, ing kuberead.Ingress) bool {
	classes, err := ctx.Reader.ListIngressClasses()
	if err != nil {
		s.Deny(err)
		s.AddLink(StepClass, StateUnknown)
		return false
	}
	names := map[string]kuberead.IngressClass{}
	var list []string
	defaultClass := ""
	for _, c := range classes.Items {
		names[c.Metadata.Name] = c
		label := c.Metadata.Name
		if c.Metadata.Annotations.DefaultIngressClass {
			defaultClass = c.Metadata.Name
			label += " (par défaut)"
		}
		list = append(list, label)
	}
	s.Captures["classes"] = strings.Join(list, ", ")
	requested, source := "", ""
	switch {
	case ing.Spec.IngressClassName != nil:
		requested, source = *ing.Spec.IngressClassName, "ingressClassName"
	case ing.Metadata.Annotations.IngressClass != "":
		requested, source = ing.Metadata.Annotations.IngressClass, "annotation kubernetes.io/ingress.class (dépréciée)"
	}
	s.Captures["class"] = requested
	switch {
	case len(classes.Items) == 0:
		s.AddLink(StepClass, StateOK, Observation{Value: "aucune classe d'Ingress dans le cluster"})
		s.Add(Pitfall{ID: IngressNoController, Step: StepClass, Severity: Warning})
		return false
	case requested != "":
		c, ok := names[requested]
		if !ok {
			s.AddLink(StepClass, StateOK, Observation{Label: source, Value: requested + " : inconnue"})
			s.Add(Pitfall{ID: IngressUnknownClass, Step: StepClass, Severity: Warning})
			return false
		}
		s.AddLink(StepClass, StateOK, Observation{Label: source, Value: requested + " (contrôleur " + c.Spec.Controller + ")"})
		s.Captures["controller"] = c.Spec.Controller
		return true
	case defaultClass != "":
		s.AddLink(StepClass, StateOK, Observation{Label: "classe par défaut", Value: defaultClass})
		s.Captures["class"], s.Captures["controller"] = defaultClass, names[defaultClass].Spec.Controller
		return true
	default:
		s.AddLink(StepClass, StateOK, Observation{Value: "aucune classe demandée, aucune par défaut"})
		s.Add(Pitfall{ID: IngressNoClass, Step: StepClass, Severity: Warning})
		return false
	}
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
