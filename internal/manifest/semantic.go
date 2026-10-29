package manifest

import (
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/cron"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/schema"
	"github.com/aboigues/telemach-plugin-kubectl/internal/textdist"
)

// Règles hors schéma (research.md R7) : apiVersion et kind, apiVersion retirées, noms
// d'objets, cohérence selector/labels, valeurs autorisées connues, secrets en clair.

// removedAPIs associe une apiVersion retirée à l'apiVersion actuelle, par kind.
var removedAPIs = map[string]map[string]string{
	"extensions/v1beta1": {
		"Deployment": "apps/v1", "DaemonSet": "apps/v1", "ReplicaSet": "apps/v1",
		"Ingress": "networking.k8s.io/v1", "NetworkPolicy": "networking.k8s.io/v1",
	},
	"apps/v1beta1":              {"Deployment": "apps/v1", "StatefulSet": "apps/v1"},
	"apps/v1beta2":              {"Deployment": "apps/v1", "StatefulSet": "apps/v1", "DaemonSet": "apps/v1", "ReplicaSet": "apps/v1"},
	"batch/v1beta1":             {"CronJob": "batch/v1"},
	"batch/v2alpha1":            {"CronJob": "batch/v1"},
	"networking.k8s.io/v1beta1": {"Ingress": "networking.k8s.io/v1", "IngressClass": "networking.k8s.io/v1"},
	"policy/v1beta1":            {"PodDisruptionBudget": "policy/v1"},
	"autoscaling/v2beta1":       {"HorizontalPodAutoscaler": "autoscaling/v2"},
	"autoscaling/v2beta2":       {"HorizontalPodAutoscaler": "autoscaling/v2"},
	"rbac.authorization.k8s.io/v1beta1": {
		"Role": "rbac.authorization.k8s.io/v1", "RoleBinding": "rbac.authorization.k8s.io/v1",
		"ClusterRole": "rbac.authorization.k8s.io/v1", "ClusterRoleBinding": "rbac.authorization.k8s.io/v1",
	},
}

// knownKinds sont des kinds Kubernetes de base, couverts ou non par les règles embarquées :
// un kind absent de cette liste dans un groupe de base est probablement mal orthographié.
var knownKinds = map[string]string{
	"Pod": "v1", "Service": "v1", "ConfigMap": "v1", "Secret": "v1", "Namespace": "v1",
	"PersistentVolumeClaim": "v1", "PersistentVolume": "v1", "ServiceAccount": "v1", "Node": "v1",
	"Endpoints": "v1", "LimitRange": "v1", "ResourceQuota": "v1", "ReplicationController": "v1",
	"Event": "v1", "List": "v1",
	"Deployment": "apps/v1", "StatefulSet": "apps/v1", "DaemonSet": "apps/v1", "ReplicaSet": "apps/v1",
	"ControllerRevision": "apps/v1",
	"Job":                "batch/v1", "CronJob": "batch/v1",
	"Ingress": "networking.k8s.io/v1", "IngressClass": "networking.k8s.io/v1", "NetworkPolicy": "networking.k8s.io/v1",
	"Role": "rbac.authorization.k8s.io/v1", "RoleBinding": "rbac.authorization.k8s.io/v1",
	"ClusterRole": "rbac.authorization.k8s.io/v1", "ClusterRoleBinding": "rbac.authorization.k8s.io/v1",
	"HorizontalPodAutoscaler": "autoscaling/v2", "PodDisruptionBudget": "policy/v1",
	"StorageClass": "storage.k8s.io/v1", "CustomResourceDefinition": "apiextensions.k8s.io/v1",
	"PriorityClass": "scheduling.k8s.io/v1", "Lease": "coordination.k8s.io/v1",
}

// coreGroups sont les groupes d'API intégrés à Kubernetes ; un autre groupe désigne une
// ressource personnalisée (CRD).
var coreGroups = map[string]bool{
	"": true, "apps": true, "batch": true, "networking.k8s.io": true, "rbac.authorization.k8s.io": true,
	"autoscaling": true, "policy": true, "storage.k8s.io": true, "apiextensions.k8s.io": true,
	"scheduling.k8s.io": true, "coordination.k8s.io": true, "extensions": true,
	"admissionregistration.k8s.io": true, "certificates.k8s.io": true, "discovery.k8s.io": true,
	"node.k8s.io": true, "events.k8s.io": true, "flowcontrol.apiserver.k8s.io": true,
	"resource.k8s.io": true,
}

// toolConfigGroups sont les groupes des fichiers de configuration des composants de
// Kubernetes : ils ont apiVersion et kind, mais se passent au composant (--config), pas à
// kubectl apply (issue #12). La valeur est le programme qui lit le fichier.
var toolConfigGroups = map[string]string{
	"kubeadm.k8s.io": "kubeadm", "kubelet.config.k8s.io": "kubelet",
	"kubeproxy.config.k8s.io": "kube-proxy", "kubescheduler.config.k8s.io": "kube-scheduler",
}

// enums liste les valeurs autorisées de champs courants, absentes des documents OpenAPI.
var enums = map[string][]string{
	"restartPolicy":   {"Always", "OnFailure", "Never"},
	"imagePullPolicy": {"Always", "IfNotPresent", "Never"},
	"protocol":        {"TCP", "UDP", "SCTP"},
	"dnsPolicy":       {"ClusterFirst", "ClusterFirstWithHostNet", "Default", "None"},
}

// serviceTypes sont les valeurs de spec.type d'un Service.
var serviceTypes = []string{"ClusterIP", "NodePort", "LoadBalancer", "ExternalName"}

var dns1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// SemanticIssues applique les règles hors schéma et le schéma de la version choisie.
func SemanticIssues(f *File, d *Document, rules *schema.SchemaSet) []Issue {
	if d.Root == nil {
		return nil
	}
	var out []Issue
	add := func(kind string, n *yaml.Node, field, suggestion, detail string) {
		is := Issue{Kind: kind, File: f.Path, Document: d.Index, Resource: d.Resource(), FieldPath: field, Suggestion: suggestion, Detail: detail}
		if n != nil {
			is.Line, is.Column = n.Line, n.Column
		} else {
			is.Line, is.Column = d.StartLine, 1
		}
		out = append(out, is)
	}
	root := d.Root
	apiKey, kindKey := getKey(root, "apiVersion"), getKey(root, "kind")
	switch {
	case apiKey == nil:
		add(KindMissingAPIVersion, root.Content[0], "apiVersion", knownKinds[d.Kind], "")
		return out
	case kindKey == nil:
		add(KindMissingKind, root.Content[0], "kind", "", "")
		return out
	}
	apiVal, kindVal := get(root, "apiVersion"), get(root, "kind")
	group := ""
	if i := strings.LastIndex(d.APIVersion, "/"); i > 0 {
		group = d.APIVersion[:i]
	}
	if tool, ok := toolConfigGroups[group]; ok {
		add(KindToolConfig, apiKey, "apiVersion", tool, d.APIVersion)
		return out
	}
	if !coreGroups[group] {
		add(KindUnknownCRD, apiKey, "apiVersion", "", d.APIVersion)
		return out
	}
	if repl, ok := removedAPIs[d.APIVersion][d.Kind]; ok {
		add(KindRemovedAPIVersion, apiVal, "apiVersion", repl, d.APIVersion)
		return out
	}
	want, known := knownKinds[d.Kind]
	if !known {
		add(KindUnknownKind, kindVal, "kind", closestKind(d.Kind), "")
		return out
	}
	if want != d.APIVersion {
		add(KindRemovedAPIVersion, apiVal, "apiVersion", want, d.APIVersion)
		return out
	}
	if meta := get(root, "metadata"); meta != nil {
		if n := get(meta, "name"); n != nil && n.Kind == yaml.ScalarNode && !dns1123.MatchString(n.Value) {
			add(KindInvalidName, n, "metadata.name", dnsName(n.Value), "")
		}
	}
	if rules != nil {
		if findings, ok := rules.Validate(root, d.APIVersion, d.Kind); ok {
			for _, fd := range findings {
				add(fd.Kind, fd.Node, fd.Path, fd.Suggestion, fd.Expected)
			}
		}
	}
	out = append(out, selectorIssues(f, d)...)
	out = append(out, enumIssues(f, d)...)
	out = append(out, cronIssues(f, d)...)
	out = append(out, limitsIssues(f, d)...)
	for _, p := range d.Masked {
		is := Issue{Kind: KindPlaintextSecret, File: f.Path, Document: d.Index, Line: p.Line, Column: p.Column, Resource: d.Resource(), Suggestion: "secretKeyRef"}
		is.FieldPath = pathAt(d.Root, p)
		out = append(out, is)
	}
	return out
}

// selectorIssues vérifie que chaque label du selector figure, avec la même valeur, dans les
// labels du modèle de pod (Deployment, StatefulSet, DaemonSet, ReplicaSet).
func selectorIssues(f *File, d *Document) []Issue {
	switch d.Kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet":
	default:
		return nil
	}
	spec := get(d.Root, "spec")
	match := get(get(spec, "selector"), "matchLabels")
	labels := get(get(get(spec, "template"), "metadata"), "labels")
	if match == nil || match.Kind != yaml.MappingNode {
		return nil
	}
	var related []int
	var expected []string
	for i := 0; i+1 < len(match.Content); i += 2 {
		k, v := match.Content[i].Value, match.Content[i+1].Value
		tv := get(labels, k)
		if tv != nil && tv.Value == v {
			continue
		}
		expected = append(expected, k+": "+v)
		if tk := getKey(labels, k); tk != nil {
			related = append(related, tk.Line)
		} else if labels != nil {
			related = append(related, labels.Line)
		}
	}
	if len(expected) == 0 {
		return nil
	}
	return []Issue{{
		Kind: KindSelectorMismatch, File: f.Path, Document: d.Index, Line: match.Line, Column: match.Column,
		Resource: d.Resource(), FieldPath: "spec.selector.matchLabels", Suggestion: strings.Join(expected, ", "),
		RelatedLines: related, RelatedPaths: []string{"spec.template.metadata.labels"},
	}}
}

// enumIssues vérifie les valeurs autorisées connues (restartPolicy, imagePullPolicy,
// protocol, dnsPolicy, type d'un Service).
func enumIssues(f *File, d *Document) []Issue {
	var out []Issue
	for path, n := range Paths(d.Root) {
		if n.Kind != yaml.ScalarNode || strings.HasSuffix(path, "#key") {
			continue
		}
		field := path[strings.LastIndex(path, ".")+1:]
		allowed := enums[field]
		if d.Kind == "Service" && path == "spec.type" {
			allowed = serviceTypes
		}
		if len(allowed) == 0 || contains(allowed, n.Value) {
			continue
		}
		sugg, _ := textdist.Closest(n.Value, allowed, 3)
		out = append(out, Issue{
			Kind: KindInvalidEnum, File: f.Path, Document: d.Index, Line: n.Line, Column: n.Column,
			Resource: d.Resource(), FieldPath: path, Suggestion: sugg, Detail: strings.Join(allowed, ", "),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// limitsIssues signale, par une information, chaque conteneur sans resources.limits
// (feature 004, FR-004) : c'est la cause n° 1 du refus « must specify limits » d'un quota.
func limitsIssues(f *File, d *Document) []Issue {
	path, ok := podTemplates[d.Kind]
	if !ok {
		return nil
	}
	tpl := d.Root
	prefix := ""
	for _, k := range path {
		tpl = get(tpl, k)
		prefix += k + "."
	}
	var out []Issue
	spec := get(tpl, "spec")
	for _, list := range []string{"initContainers", "containers"} {
		for i, c := range seq(get(spec, list)) {
			if lim := get(get(c, "resources"), "limits"); lim != nil && len(lim.Content) > 0 {
				continue
			}
			key := getKey(c, "name")
			if key == nil {
				continue
			}
			out = append(out, Issue{
				Kind: KindMissingLimits, File: f.Path, Document: d.Index, Line: key.Line, Column: key.Column,
				Resource: d.Resource(), FieldPath: prefix + "spec." + list + "[" + itoa(i) + "].resources.limits",
				Detail: scalar(c, "name"),
			})
		}
	}
	return out
}

// cronIssues vérifie hors ligne l'expression spec.schedule d'un CronJob (feature 004,
// research.md F9). La version corrigée n'est proposée que si elle est certaine : quatre champs
// dont les trois derniers valent « * » (il manque un « * »).
func cronIssues(f *File, d *Document) []Issue {
	if d.Kind != "CronJob" {
		return nil
	}
	n := get(get(d.Root, "spec"), "schedule")
	if n == nil || n.Kind != yaml.ScalarNode {
		return nil
	}
	_, err := cron.Parse(n.Value)
	if err == nil {
		return nil
	}
	is := Issue{
		Kind: KindInvalidCron, File: f.Path, Document: d.Index, Line: n.Line, Column: n.Column,
		Resource: d.Resource(), FieldPath: "spec.schedule", Detail: err.Error(),
	}
	if fs := strings.Fields(n.Value); len(fs) == 4 && fs[1] == "*" && fs[2] == "*" && fs[3] == "*" {
		quote := ""
		switch n.Style {
		case yaml.DoubleQuotedStyle:
			quote = `"`
		case yaml.SingleQuotedStyle:
			quote = "'"
		}
		is.Suggestion = quote + strings.Join(fs, " ") + " *" + quote
	}
	return []Issue{is}
}

// pathAt retrouve le chemin du champ dont la valeur est à la position p.
func pathAt(root *yaml.Node, p Position) string {
	for path, n := range Paths(root) {
		if !strings.HasSuffix(path, "#key") && n.Kind == yaml.ScalarNode && n.Line == p.Line && n.Column == p.Column {
			return path
		}
	}
	return ""
}

// closestKind propose le kind connu le plus proche.
func closestKind(kind string) string {
	names := make([]string, 0, len(knownKinds))
	for k := range knownKinds {
		names = append(names, k)
	}
	sort.Strings(names)
	best, bestDist := "", 4
	for _, n := range names {
		if d := textdist.TypoDistance(strings.ToLower(kind), strings.ToLower(n)); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

// dnsName transforme un nom en nom d'objet valide.
func dnsName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// maskDocument masque les valeurs sensibles du document et retient leurs positions.
func maskDocument(d *Document) {
	if d.Root == nil {
		return
	}
	for _, p := range redact.RedactNode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{d.Root}}) {
		d.Masked = append(d.Masked, Position{p.Line, p.Column})
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
