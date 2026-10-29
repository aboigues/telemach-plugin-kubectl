package manifest

// HPA sans request, vérifié hors ligne (feature 004, FR-018, research.md F10) : quand un
// HorizontalPodAutoscaler et sa cible figurent dans les fichiers vérifiés, chaque conteneur
// de la cible doit déclarer une request pour chaque ressource visée en pourcentage. C'est une
// information : check ne voit pas la cible si elle est ailleurs.

// hpaMissingRequests parcourt tous les documents relus.
func hpaMissingRequests(files []*File) []Issue {
	type target struct {
		f *File
		d *Document
	}
	targets := map[string]target{} // « Kind/nom » (namespace ignoré : même lot de fichiers)
	type hpaRef struct {
		key       string
		resources []string
	}
	var hpas []hpaRef
	for _, f := range files {
		for _, d := range f.Documents {
			if d.Root == nil {
				continue
			}
			switch d.Kind {
			case "Deployment", "StatefulSet":
				targets[d.Kind+"/"+d.Name] = target{f, d}
			case "HorizontalPodAutoscaler":
				spec := get(d.Root, "spec")
				ref := get(spec, "scaleTargetRef")
				h := hpaRef{key: scalar(ref, "kind") + "/" + scalar(ref, "name")}
				for _, m := range seq(get(spec, "metrics")) {
					res := get(m, "resource")
					if scalar(m, "type") == "Resource" && scalar(get(res, "target"), "type") == "Utilization" {
						h.resources = append(h.resources, scalar(res, "name"))
					}
				}
				hpas = append(hpas, h)
			}
		}
	}
	var out []Issue
	for _, h := range hpas {
		t, ok := targets[h.key]
		if !ok {
			continue
		}
		spec := get(get(get(t.d.Root, "spec"), "template"), "spec")
		for i, c := range seq(get(spec, "containers")) {
			requests := get(get(c, "resources"), "requests")
			for _, res := range h.resources {
				if get(requests, res) != nil {
					continue
				}
				key := getKey(c, "name")
				if key == nil {
					continue
				}
				out = append(out, Issue{
					Kind: KindHPAMissingRequest, File: t.f.Path, Document: t.d.Index, Line: key.Line, Column: key.Column,
					Resource: t.d.Resource(), FieldPath: "spec.template.spec.containers[" + itoa(i) + "].resources.requests." + res,
					Suggestion: res, Detail: scalar(c, "name"),
				})
			}
		}
	}
	return out
}
