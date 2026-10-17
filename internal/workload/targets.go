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
