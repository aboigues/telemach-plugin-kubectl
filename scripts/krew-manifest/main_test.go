package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const templatePath = "../../packaging/krew/telemach.yaml.tmpl"

var testPlatforms = []string{"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"}

// sampleChecksums écrit un checksums.txt d'essai : binaires, archives et SBOM, comme la
// Release ; les sommes des archives sont reconnaissables (archive-<plateforme>).
func sampleChecksums(version string, skip string) string {
	var b strings.Builder
	for _, p := range testPlatforms {
		base := "kubectl-telemach_" + version + "_" + p
		fmt.Fprintf(&b, "%064x  %s\n", 1, base)
		if p != skip {
			fmt.Fprintf(&b, "%s  %s.tar.gz\n", archiveSum(p), base)
		}
		fmt.Fprintf(&b, "%064x  %s.sbom.json\n", 2, base)
	}
	return b.String()
}

// archiveSum fabrique une somme SHA-256 d'essai propre à chaque plateforme.
func archiveSum(p string) string {
	sum := sha256.Sum256([]byte("archive-" + p))
	return hex.EncodeToString(sum[:])
}

type manifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Version          string `yaml:"version"`
		Homepage         string `yaml:"homepage"`
		ShortDescription string `yaml:"shortDescription"`
		Description      string `yaml:"description"`
		Caveats          string `yaml:"caveats"`
		Platforms        []struct {
			Selector struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			} `yaml:"selector"`
			URI    string `yaml:"uri"`
			Sha256 string `yaml:"sha256"`
			Bin    string `yaml:"bin"`
			Files  []struct {
				From string `yaml:"from"`
				To   string `yaml:"to"`
			} `yaml:"files"`
		} `yaml:"platforms"`
	} `yaml:"spec"`
}

func readTemplate(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// T019 : manifeste conforme à contracts/krew-manifest.md, sommes lues dans checksums.txt.
func TestGenerate(t *testing.T) {
	out, err := generate(strings.NewReader(sampleChecksums("1.2.3", "")), "1.2.3", defaultBaseURL, readTemplate(t))
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("manifeste illisible : %v\n%s", err, out)
	}
	if m.APIVersion != "krew.googlecontainertools.github.com/v1alpha2" || m.Kind != "Plugin" || m.Metadata.Name != "telemach" {
		t.Errorf("en-tête : %s %s %s", m.APIVersion, m.Kind, m.Metadata.Name)
	}
	if m.Spec.Version != "v1.2.3" || m.Spec.Homepage != "https://github.com/aboigues/telemach-plugin-kubectl" {
		t.Errorf("version %q, homepage %q", m.Spec.Version, m.Spec.Homepage)
	}
	for name, text := range map[string]string{"shortDescription": m.Spec.ShortDescription, "description": m.Spec.Description, "caveats": m.Spec.Caveats} {
		if !strings.ContainsAny(text, "éèàçù") {
			t.Errorf("%s doit être rédigé en français : %q", name, text)
		}
	}
	if !strings.Contains(m.Spec.Caveats, "kubectl-telemach shell-init") {
		t.Errorf("caveats sans la prochaine étape : %q", m.Spec.Caveats)
	}
	if len(m.Spec.Platforms) != len(testPlatforms) {
		t.Fatalf("%d plateformes, attendu %d", len(m.Spec.Platforms), len(testPlatforms))
	}
	seen := map[string]bool{}
	for _, p := range m.Spec.Platforms {
		key := p.Selector.MatchLabels["os"] + "_" + p.Selector.MatchLabels["arch"]
		seen[key] = true
		wantURI := "https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v1.2.3/kubectl-telemach_1.2.3_" + key + ".tar.gz"
		if p.URI != wantURI {
			t.Errorf("%s : uri %q, attendu %q", key, p.URI, wantURI)
		}
		if p.Sha256 != archiveSum(key) {
			t.Errorf("%s : sha256 %q, attendu la somme de l'archive %q", key, p.Sha256, archiveSum(key))
		}
		if p.Bin != "kubectl-telemach" {
			t.Errorf("%s : bin %q", key, p.Bin)
		}
		var from []string
		for _, f := range p.Files {
			from = append(from, f.From)
			if f.To != "." {
				t.Errorf("%s : %s copié vers %q", key, f.From, f.To)
			}
		}
		if strings.Join(from, ",") != "kubectl-telemach,LICENSE,NOTICE" {
			t.Errorf("%s : fichiers %v", key, from)
		}
	}
	for _, p := range testPlatforms {
		if !seen[p] {
			t.Errorf("plateforme absente : %s", p)
		}
	}
}

// Une adresse de base locale (job krew-manifest de la CI) remplace celle de la Release.
func TestGenerateBaseURL(t *testing.T) {
	out, err := generate(strings.NewReader(sampleChecksums("1.2.3", "")), "1.2.3", "http://127.0.0.1:8080", readTemplate(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "uri: http://127.0.0.1:8080/kubectl-telemach_1.2.3_linux_amd64.tar.gz") {
		t.Errorf("adresse de base non appliquée :\n%s", out)
	}
}

// Aucun manifeste n'est produit sur une information incomplète ou pour une pré-version
// (FR-008).
func TestGenerateRefusals(t *testing.T) {
	cases := []struct {
		name, version, checksums, want string
	}{
		{"archive manquante", "1.2.3", sampleChecksums("1.2.3", "darwin_arm64"), "darwin_arm64"},
		{"pré-version", "1.2.3-rc.1", sampleChecksums("1.2.3-rc.1", ""), "pré-version"},
		{"version invalide", "v1.2", sampleChecksums("v1.2", ""), "version"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := generate(strings.NewReader(c.checksums), c.version, defaultBaseURL, readTemplate(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("erreur %v, attendu une erreur mentionnant %q", err, c.want)
			}
			if len(out) != 0 {
				t.Errorf("aucun manifeste attendu :\n%s", out)
			}
		})
	}
}
