package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
)

// Scénarios V7, V8, V9 et V11 du quickstart : rendu des problèmes de manifestes.
func TestManifestGolden(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path, version string
	}{
		{"v7-tabulation", "tab-indent/deploy.yaml", "v1.37.1"},
		{"v8-champ-multidoc", "unknown-field-multidoc/all.yaml", "v1.37.1"},
		{"v9-selector", "selector-mismatch/deploy.yaml", "v1.37.1"},
		{"v10-secret", "env-secret/deploy.yaml", "v1.36.5"},
		{"v11-valide", "valid", "v1.36.5"},
		{"v11-hors-plage", "valid/deploy.yaml", "v1.32.3"},
	}
	root := filepath.Join("..", "..", "testdata", "manifests")
	for _, c := range cases {
		for _, color := range []bool{false, true} {
			suffix := map[bool]string{false: "nocolor", true: "color"}[color]
			t.Run(c.name+"/"+suffix, func(t *testing.T) {
				wd, _ := os.Getwd()
				if err := os.Chdir(root); err != nil {
					t.Fatal(err)
				}
				defer os.Chdir(wd) //nolint:errcheck
				r := manifest.Analyze([]string{c.path}, false, c.version)
				var b bytes.Buffer
				opts := ManifestOptions{Options: Options{Theme: NewTheme(color, true), Width: 80}, Kubectl: "kubectl", ShowRules: true, ShowSuccess: true}
				if err := RenderManifestIssues(&b, r, cat, opts); err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(wd); err != nil {
					t.Fatal(err)
				}
				checkGolden(t, filepath.Join("us2", c.name+"."+suffix+".txt"), b.String())
				if !color && strings.Contains(b.String(), "\x1b") {
					t.Error("aucune séquence ESC attendue sans couleur")
				}
				if strings.Contains(b.String(), "s3cret-fake") {
					t.Error("secret en clair dans le rendu (SC-006)")
				}
			})
		}
	}
}
