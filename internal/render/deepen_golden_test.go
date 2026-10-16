package render

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
)

func TestDeepenGolden(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cmdline.unknown-resource-type", "schema.plaintext-secret"} {
		e, ok := cat.Get(id)
		if !ok {
			t.Fatalf("entrée %s absente", id)
		}
		for _, exercise := range []bool{false, true} {
			name := id
			if exercise {
				name += ".exercice"
			}
			t.Run(name, func(t *testing.T) {
				var b bytes.Buffer
				if err := RenderDeepen(&b, e, map[string]string{"kubectl": "kubectl"}, exercise, Options{Theme: NewTheme(false, true), Width: 80}); err != nil {
					t.Fatal(err)
				}
				checkGolden(t, filepath.Join("us3", name+".nocolor.txt"), b.String())
			})
		}
	}
}
