package diagnose

import (
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kubeterms"
)

// Captures calculées pour les entrées de la feature 004 (research.md F7, F14) : le nœud visé
// par drain, et l'objet d'un wait expiré, à diagnostiquer avec « telemach diagnose ».
func init() {
	enrichers = append(enrichers, followUp)
}

// diagnosable associe un type de ressource (nom canonique de kubeterms) au type accepté par
// « kubectl telemach diagnose ».
var diagnosable = map[string]string{
	"pod": "pod", "service": "service", "deployment": "deployment", "ingress": "ingress",
	"persistentvolumeclaim": "pvc", "job": "job", "cronjob": "cronjob", "daemonset": "daemonset",
	"statefulset": "statefulset", "horizontalpodautoscaler": "hpa",
}

func followUp(in Input, d *Diagnosis) {
	if d.Entry == nil {
		return
	}
	c := in.Command
	switch {
	case strings.HasPrefix(d.Entry.ID, "drain.") && c.Verb() == "drain" && len(c.Positionals) > 0:
		d.Captures["node"] = c.Positionals[0]
	case d.Entry.ID == "wait.timeout":
		if f := c.Flags["--for"]; f != "" {
			d.Captures["wait_for"] = f
		}
		// Le type vient du message (« pods/web ») : il est ramené au nom canonique.
		if rt, ok := kubeterms.ResolveType(d.Captures["restype"]); ok {
			if kind, ok := diagnosable[rt.Singular]; ok && d.Captures["objname"] != "" {
				d.Captures["diag_target"] = kind + "/" + d.Captures["objname"]
			}
		}
	}
}
