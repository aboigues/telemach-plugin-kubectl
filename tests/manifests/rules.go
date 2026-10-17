package manifests

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Identifiants des règles de sécurité appliquées aux manifestes d'exemple (constitution,
// principe III ; contrôle CI manifest-lint, principe IV). Un scénario de test ne peut s'en
// écarter que pour un défaut qu'il déclare dans « allowed_defects ».
const (
	RuleParse                    = "parse"
	RuleRunAsNonRoot             = "run-as-non-root"
	RuleAllowPrivilegeEscalation = "allow-privilege-escalation"
	RuleResources                = "resources"
	RulePrivileged               = "privileged"
	RuleHostPath                 = "host-path"
	RuleHostNetwork              = "host-network"
	RuleHostPID                  = "host-pid"
)

// Violation est un écart à une règle bloquante.
type Violation struct {
	Rule     string
	Document int    // numéro du document dans le fichier (1-based)
	Resource string // Kind/nom
	Detail   string
}

func (v Violation) String() string {
	return fmt.Sprintf("document %d (%s) : %s — %s", v.Document, v.Resource, v.Rule, v.Detail)
}

// workloadKinds associe chaque type de charge de travail au chemin de son modèle de pod.
var workloadKinds = map[string][]string{
	"Pod":         {},
	"Deployment":  {"spec", "template"},
	"ReplicaSet":  {"spec", "template"},
	"StatefulSet": {"spec", "template"},
	"DaemonSet":   {"spec", "template"},
	"Job":         {"spec", "template"},
	"CronJob":     {"spec", "jobTemplate", "spec", "template"},
}

// Check applique les règles de sécurité à un texte YAML (un ou plusieurs documents). Les
// violations sont bloquantes ; les avertissements (image sans tag ou en « latest ») ne le
// sont pas.
func Check(src []byte) (violations []Violation, warnings []string) {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	for i := 1; ; i++ {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return violations, warnings
		}
		if err != nil {
			return append(violations, Violation{Rule: RuleParse, Document: i, Detail: err.Error()}), warnings
		}
		if len(doc.Content) == 0 {
			continue
		}
		root := doc.Content[0]
		kind := scalar(root, "kind")
		resource := kind + "/" + scalar(get(root, "metadata"), "name")
		path, ok := workloadKinds[kind]
		if !ok {
			continue
		}
		pod := root
		for _, k := range path {
			pod = get(pod, k)
		}
		v, w := checkPodTemplate(pod)
		for j := range v {
			v[j].Document, v[j].Resource = i, resource
		}
		violations = append(violations, v...)
		for _, msg := range w {
			warnings = append(warnings, fmt.Sprintf("document %d (%s) : %s", i, resource, msg))
		}
	}
}

// checkPodTemplate vérifie un pod ou un modèle de pod (metadata + spec).
func checkPodTemplate(tpl *yaml.Node) (violations []Violation, warnings []string) {
	spec := get(tpl, "spec")
	if spec == nil {
		return []Violation{{Rule: RuleParse, Detail: "spec du pod introuvable"}}, nil
	}
	fail := func(rule, format string, a ...any) {
		violations = append(violations, Violation{Rule: rule, Detail: fmt.Sprintf(format, a...)})
	}
	if scalar(spec, "hostNetwork") == "true" {
		fail(RuleHostNetwork, "hostNetwork: true expose le réseau du nœud")
	}
	if scalar(spec, "hostPID") == "true" {
		fail(RuleHostPID, "hostPID: true expose les processus du nœud")
	}
	for _, vol := range items(get(spec, "volumes")) {
		if get(vol, "hostPath") != nil {
			fail(RuleHostPath, "volume %q de type hostPath", scalar(vol, "name"))
		}
	}
	podNonRoot := scalar(get(spec, "securityContext"), "runAsNonRoot") == "true"
	containers := append(items(get(spec, "containers")), items(get(spec, "initContainers"))...)
	for _, c := range containers {
		name := scalar(c, "name")
		sc := get(c, "securityContext")
		if !podNonRoot && scalar(sc, "runAsNonRoot") != "true" {
			fail(RuleRunAsNonRoot, "conteneur %q : securityContext.runAsNonRoot: true absent", name)
		}
		if scalar(sc, "allowPrivilegeEscalation") != "false" {
			fail(RuleAllowPrivilegeEscalation, "conteneur %q : securityContext.allowPrivilegeEscalation: false absent", name)
		}
		if scalar(sc, "privileged") == "true" {
			fail(RulePrivileged, "conteneur %q : privileged: true", name)
		}
		res := get(c, "resources")
		if get(res, "requests") == nil || get(res, "limits") == nil {
			fail(RuleResources, "conteneur %q : resources.requests et resources.limits obligatoires", name)
		}
		if w := imageWarning(scalar(c, "image")); w != "" {
			warnings = append(warnings, fmt.Sprintf("conteneur %q : %s", name, w))
		}
	}
	return violations, warnings
}

// imageWarning signale une image sans tag explicite ou en « latest » (avertissement non
// bloquant, constitution principe III).
func imageWarning(ref string) string {
	if ref == "" || strings.Contains(ref, "@sha256:") {
		return ""
	}
	name := ref
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		name = ref[i+1:]
	}
	_, tag, ok := strings.Cut(name, ":")
	switch {
	case !ok:
		return fmt.Sprintf("image %q sans tag explicite (latest implicite)", ref)
	case tag == "latest":
		return fmt.Sprintf("image %q avec le tag latest", ref)
	}
	return ""
}

func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node, key string) string {
	if v := get(n, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

func items(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}
