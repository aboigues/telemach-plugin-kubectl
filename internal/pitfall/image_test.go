package pitfall

import (
	"strings"
	"testing"
)

// FuzzImageRef vérifie les invariants du découpage d'une référence d'image (T067, LL-001) :
// aucune panique, un nom sans « @ », un tag sans « / » ni « @ », une image désignée par digest
// jamais mutable, et la référence reconstruite à l'identique quand tag et digest sont non vides.
func FuzzImageRef(f *testing.F) {
	for _, seed := range []string{
		"nginx", "nginx:latest", "nginxinc/nginx-unprivileged:1.30-alpine", "registry.example:5000/app",
		"registry.example:5000/app:1.0@sha256:abc", "app@sha256:abc", "app:", "@", ":", "a/b:c/d", "",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		r := ParseImage(ref)
		if strings.Contains(r.Name, "@") {
			t.Errorf("%q : nom %q avec « @ »", ref, r.Name)
		}
		if strings.ContainsAny(r.Tag, "/@") {
			t.Errorf("%q : tag %q avec « / » ou « @ »", ref, r.Tag)
		}
		if r.Digest != "" && r.Mutable() {
			t.Errorf("%q : une image désignée par digest n'est jamais mutable", ref)
		}
		if r.Tag != "" && r.Digest != "" {
			if got := r.Name + ":" + r.Tag + "@" + r.Digest; got != ref {
				t.Errorf("%q : reconstruit en %q", ref, got)
			}
		}
	})
}
