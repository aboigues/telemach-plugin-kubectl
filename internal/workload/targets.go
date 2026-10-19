package workload

import (
	"sort"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
)

// Target est un pod, ou un ensemble de pods désigné par un selector, à vérifier après une
// création réussie (FR-028). Les cibles sont déduites de la commande et des manifestes
// locaux, sans aucune lecture du cluster.
type Target struct {
	Pod       string // nom du pod, ou vide
	Selector  string // « app=web » pour les pods d'un deployment, ou vide
	Namespace string
}

// Targets renvoie les cibles de la vérification différée ; aucune pour une commande non
// concernée (--dry-run, -f -, URL, -k, autre sous-commande).
func Targets(cmd command.Command) []Target {
	if cmd.DryRun || cmd.Kustomize != "" {
		return nil
	}
	switch {
	case cmd.Sub() == "run" && len(cmd.ResourceNames) == 1:
		return []Target{{Pod: cmd.ResourceNames[0], Namespace: cmd.Namespace}}
	case cmd.Sub() == "create deployment" && len(cmd.ResourceNames) == 1:
		return []Target{{Selector: "app=" + cmd.ResourceNames[0], Namespace: cmd.Namespace}}
	case (cmd.Verb() == "apply" || cmd.Sub() == "create") && len(cmd.Files) > 0:
		if cmd.HasUnrereadableFiles() {
			return nil
		}
		return manifestTargets(cmd)
	}
	return nil
}

// manifestTargets relit les manifestes locaux (lecture seule) et retient les Pod et les
// Deployment.
func manifestTargets(cmd command.Command) []Target {
	paths, _ := manifest.Expand(cmd.LocalFiles(), cmd.Recursive)
	var out []Target
	for _, p := range paths {
		f, issues := manifest.Read(p)
		if len(issues) > 0 || !f.Readable {
			continue
		}
		for _, d := range f.Documents {
			ns := d.Namespace
			if ns == "" {
				ns = cmd.Namespace
			}
			switch d.Kind {
			case "Pod":
				if d.Name != "" {
					out = append(out, Target{Pod: d.Name, Namespace: ns})
				}
			case "Deployment":
				if sel := matchLabels(d); sel != "" {
					out = append(out, Target{Selector: sel, Namespace: ns})
				}
			}
		}
	}
	return out
}

// matchLabels écrit spec.selector.matchLabels d'un deployment sous la forme « k=v,… ».
func matchLabels(d *manifest.Document) string {
	paths := manifest.Paths(d.Root)
	var parts []string
	for p, n := range paths {
		if k, ok := strings.CutPrefix(p, "spec.selector.matchLabels."); ok && !strings.HasSuffix(k, "#key") && !strings.ContainsAny(k, ".[") {
			parts = append(parts, k+"="+n.Value)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// ObjectTarget est un Service, un Ingress ou un PVC à vérifier après une création réussie
// (feature 002, research.md S9), déduit de la commande et des manifestes locaux sans aucune
// lecture du cluster.
type ObjectTarget struct {
	Kind      string // service, ingress, pvc
	Name      string
	Namespace string
	// CoveredBySameApply : Service dont le selector est satisfait par les labels d'un Pod ou
	// d'un Deployment de la même commande ; ses pods peuvent ne pas exister encore, il n'est
	// pas signalé comme ne désignant aucun pod.
	CoveredBySameApply bool
}

// manifestKinds associe un type de manifeste au type d'objet vérifié.
var manifestKinds = map[string]string{"Service": "service", "Ingress": "ingress", "PersistentVolumeClaim": "pvc"}

// ObjectTargets renvoie les objets à vérifier ; aucun pour une commande non concernée
// (--dry-run, -f -, URL, -k, autre sous-commande).
func ObjectTargets(cmd command.Command) []ObjectTarget {
	if cmd.DryRun || cmd.Kustomize != "" {
		return nil
	}
	switch {
	case cmd.Verb() == "expose" && len(cmd.Files) == 0 && len(cmd.ResourceNames) == 1:
		name := cmd.Flags["--name"]
		if name == "" {
			name = cmd.ResourceNames[0]
		}
		return []ObjectTarget{{Kind: "service", Name: name, Namespace: cmd.Namespace}}
	case cmd.Verb() == "create" && len(cmd.Files) == 0:
		// « create service <type> <nom> » ; l'alias « svc » n'est pas reconnu comme
		// sous-commande, le nom est alors le troisième argument.
		pos := cmd.Positionals
		if len(pos) == 3 && (pos[0] == "service" || pos[0] == "svc") {
			return []ObjectTarget{{Kind: "service", Name: pos[2], Namespace: cmd.Namespace}}
		}
		if cmd.Sub() == "create service" && len(pos) == 2 {
			return []ObjectTarget{{Kind: "service", Name: pos[1], Namespace: cmd.Namespace}}
		}
	case (cmd.Verb() == "apply" || cmd.Verb() == "create") && len(cmd.Files) > 0:
		if cmd.HasUnrereadableFiles() {
			return nil
		}
		return manifestObjects(cmd)
	}
	return nil
}

// manifestObjects relit les manifestes locaux (lecture seule) et retient les Service, Ingress
// et PVC, en notant les Services dont les pods sont créés par la même commande.
func manifestObjects(cmd command.Command) []ObjectTarget {
	var out []ObjectTarget
	var selectors []map[string]string // selector de chaque Service de out, dans l'ordre
	var podLabels []map[string]string
	for _, d := range manifestDocuments(cmd) {
		switch d.Kind {
		case "Pod":
			podLabels = append(podLabels, mapAt(d, "metadata.labels."))
		case "Deployment":
			podLabels = append(podLabels, mapAt(d, "spec.template.metadata.labels."))
		}
		kind, ok := manifestKinds[d.Kind]
		if !ok || d.Name == "" {
			continue
		}
		ns := d.Namespace
		if ns == "" {
			ns = cmd.Namespace
		}
		out = append(out, ObjectTarget{Kind: kind, Name: d.Name, Namespace: ns})
		selectors = append(selectors, mapAt(d, "spec.selector."))
	}
	for i := range out {
		if out[i].Kind != "service" || len(selectors[i]) == 0 {
			continue
		}
		for _, labels := range podLabels {
			if satisfies(selectors[i], labels) {
				out[i].CoveredBySameApply = true
				break
			}
		}
	}
	return out
}

// Images renvoie les images des conteneurs créés par la commande (manifestes locaux, ou
// --image de create deployment et run), pour l'information sur le tag mutable (FR-023).
func Images(cmd command.Command) []string {
	if cmd.DryRun || cmd.Kustomize != "" {
		return nil
	}
	switch {
	case (cmd.Sub() == "create deployment" || cmd.Sub() == "run") && cmd.Flags["--image"] != "":
		return []string{cmd.Flags["--image"]}
	case (cmd.Verb() == "apply" || cmd.Verb() == "create") && len(cmd.Files) > 0:
		if cmd.HasUnrereadableFiles() {
			return nil
		}
	default:
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range manifestDocuments(cmd) {
		paths := manifest.Paths(d.Root)
		var keys []string
		for p := range paths {
			if strings.HasSuffix(p, ".image") && strings.Contains(p, "ontainers[") {
				keys = append(keys, p)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if img := paths[k].Value; img != "" && !seen[img] {
				seen[img] = true
				out = append(out, img)
			}
		}
	}
	return out
}

// manifestDocuments relit les documents lisibles des manifestes locaux de la commande.
func manifestDocuments(cmd command.Command) []*manifest.Document {
	paths, _ := manifest.Expand(cmd.LocalFiles(), cmd.Recursive)
	var out []*manifest.Document
	for _, p := range paths {
		f, issues := manifest.Read(p)
		if len(issues) > 0 || !f.Readable {
			continue
		}
		out = append(out, f.Documents...)
	}
	return out
}

// mapAt lit une table de chaînes d'un document (labels, selector) sous le préfixe donné.
func mapAt(d *manifest.Document, prefix string) map[string]string {
	out := map[string]string{}
	for p, n := range manifest.Paths(d.Root) {
		if k, ok := strings.CutPrefix(p, prefix); ok && !strings.HasSuffix(k, "#key") && !strings.ContainsAny(k, ".[") {
			out[k] = n.Value
		}
	}
	return out
}

// satisfies indique si des labels satisfont un selector d'égalités.
func satisfies(selector, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
