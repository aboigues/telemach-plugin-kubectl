package pitfall

import "strings"

// ImageRef est une référence d'image OCI découpée : [registre[:port]/]chemin[:tag][@digest].
type ImageRef struct {
	Name   string // registre et chemin
	Tag    string // vide si absent
	Digest string // sha256:…, vide si absent
}

// ParseImage découpe une référence d'image. Le « : » d'un tag est celui qui suit le dernier
// « / » : « registre:5000/app » n'a pas de tag.
func ParseImage(ref string) ImageRef {
	var r ImageRef
	if i := strings.Index(ref, "@"); i >= 0 {
		r.Digest = ref[i+1:]
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		r.Tag = ref[i+1:]
		ref = ref[:i]
	}
	r.Name = ref
	return r
}

// Mutable indique une image désignée par un tag qui peut changer de contenu sous le même nom :
// aucun tag (latest implicite) ou le tag latest. Une référence par digest ne l'est jamais.
func (r ImageRef) Mutable() bool {
	return r.Digest == "" && (r.Tag == "" || r.Tag == "latest")
}
