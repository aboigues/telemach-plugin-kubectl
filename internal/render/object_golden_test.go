package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/pitfall"
	"github.com/aboigues/telemach-plugin-kubectl/internal/testcorpus"
)

// objectGolden est un diagnostic d'objet rejoué sur le corpus relevé sur kind (feature 002).
type objectGolden struct {
	corpus string // dossier de testdata/objects/
	kind   pitfall.Kind
	name   string
	file   string // nom du fichier de référence quand <type>-<nom> ne suffit pas
}

var objectGoldens = []objectGolden{
	{"selector-typo", pitfall.KindService, "web", ""},
	{"not-ready", pitfall.KindService, "api", ""},
	{"targetport-name", pitfall.KindService, "shop", ""},
	{"targetport-number", pitfall.KindService, "cart", ""},
	{"targetport-undeclared", pitfall.KindService, "blog", ""},
	{"no-selector", pitfall.KindService, "external", ""},
	{"no-selector", pitfall.KindService, "docs", ""},
	{"healthy", pitfall.KindService, "hello", ""},
	{"rollout-stalled", pitfall.KindDeployment, "stalled", ""},
	{"mutable-tag", pitfall.KindDeployment, "latest-app", ""},
	{"config-env", pitfall.KindDeployment, "conf", ""},
	{"config-env-stale", pitfall.KindDeployment, "conf", "deployment-conf-stale"},
	{"healthy", pitfall.KindDeployment, "hello", ""},
	{"ingress-no-class", pitfall.KindIngress, "web", ""},
	{"ingress-no-default-class", pitfall.KindIngress, "web", "ingress-web-no-default"},
	{"ingress-unknown-class", pitfall.KindIngress, "typo", ""},
	{"ingress-backend-missing", pitfall.KindIngress, "shop", ""},
	{"ingress-backend-port", pitfall.KindIngress, "wrong-port", ""},
	{"ingress-no-address", pitfall.KindIngress, "hello-web", ""},
	{"pvc-unknown-class", pitfall.KindPVC, "fast-data", ""},
	{"pvc-empty-class", pitfall.KindPVC, "static-data", ""},
	{"pvc-no-default-class", pitfall.KindPVC, "nodefault-data", ""},
	{"pvc-waiting", pitfall.KindPVC, "data", ""},
	{"pvc-consumer-pending", pitfall.KindPVC, "cache-data", ""},
}

// TestObjectGolden fige le rendu de diagnose <type>/<nom> (FR-021), en couleur et sans
// couleur ; TestGoldenFrench et TestGoldenNoSecret contrôlent ces rendus (FR-031).
func TestObjectGolden(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range objectGoldens {
		dir := filepath.Join("..", "..", "testdata", "objects", g.corpus)
		at, _ := os.ReadFile(filepath.Join(dir, "captured-at.txt")) //nolint:gosec // corpus du dépôt
		when, _ := time.Parse(time.RFC3339, strings.TrimSpace(string(at)))
		ctx := pitfall.Context{Reader: &testcorpus.ObjectFixture{Dir: dir}, Scope: kuberead.Scope{Namespace: "demo"}, Catalog: cat, Now: when}
		snap, err := pitfall.Diagnose(ctx, g.kind, g.name)
		if err != nil {
			t.Fatalf("%s : %v", g.corpus, err)
		}
		for _, color := range []bool{false, true} {
			suffix := "nocolor"
			if color {
				suffix = "color"
			}
			var b bytes.Buffer
			if err := RenderObject(&b, snap, cat, Options{Theme: NewTheme(color, true), Width: 80}); err != nil {
				t.Fatal(err)
			}
			name := string(g.kind) + "-" + g.name + "." + suffix + ".txt"
			if g.file != "" {
				name = g.file + "." + suffix + ".txt"
			}
			checkGolden(t, filepath.Join("f002", name), b.String())
			if !color && strings.Contains(b.String(), "\x1b") {
				t.Error("aucune séquence ESC attendue sans couleur")
			}
		}
	}
}
