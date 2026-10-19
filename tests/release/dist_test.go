//go:build dist

// Package release vérifie les artefacts produits par GoReleaser (dossier DIST_DIR) contre le
// contrat specs/003-distribution/contracts/release-assets.md.
// Lancement : DIST_DIR=dist go test -tags dist ./tests/release/...
package release

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/aboigues/telemach-plugin-kubectl"

var platforms = []string{"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"}

// archiveContent est le contenu exact de chaque archive (voie Krew, research R2).
var archiveContent = []string{"LICENSE", "NOTICE", "kubectl-telemach"}

// distDir renvoie le dossier des artefacts, ou ignore le test s'il est absent.
func distDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("DIST_DIR")
	if dir == "" {
		t.Skip("DIST_DIR non défini")
	}
	if !filepath.IsAbs(dir) {
		// go test s'exécute dans le dossier du paquet : le chemin est relatif à la racine.
		dir = filepath.Join("..", "..", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("dossier %s absent", dir)
	}
	return dir
}

// artifact est une entrée de dist/artifacts.json.
type artifact struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type metadata struct {
	Version string `json:"version"`
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // artefacts de la compilation
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s : %v", path, err)
	}
}

// releaseFiles renvoie, par nom publié, le chemin local des artefacts téléversés dans la Release.
func releaseFiles(t *testing.T, dir string) (map[string]string, string) {
	t.Helper()
	var meta metadata
	readJSON(t, filepath.Join(dir, "metadata.json"), &meta)
	var arts []artifact
	readJSON(t, filepath.Join(dir, "artifacts.json"), &arts)
	files := map[string]string{}
	for _, a := range arts {
		// Avec le format d'archive « binary », un binaire publié est un artefact de type Binary
		// renommé kubectl-telemach_<version>_<os>_<arch> ; les sorties de compilation brutes
		// (kubectl-telemach, telemach-image) ne sont pas téléversées.
		uploaded := a.Type == "Binary" && strings.Contains(a.Name, "_")
		switch {
		case uploaded, a.Type == "Archive", a.Type == "SBOM", a.Type == "Checksum",
			a.Type == "Signature", a.Type == "Certificate":
			p := a.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, "..", p)
			}
			files[a.Name] = p
		}
	}
	return files, meta.Version
}

func sha256Reader(t *testing.T, r io.Reader) string {
	t.Helper()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // artefacts de la compilation
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	return sha256Reader(t, f)
}

// checksums lit checksums.txt (format sha256sum).
func checksums(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // artefacts de la compilation
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 {
			sums[fields[1]] = fields[0]
		}
	}
	return sums
}

func TestBinariesFollowContract(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)

	var binaries []string
	for name := range files {
		if strings.HasPrefix(name, "telemach-image") {
			t.Errorf("le point d'entrée de l'image ne doit pas être publié : %s", name)
		}
		if strings.HasPrefix(name, "kubectl-telemach_") && !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".sbom.json") {
			binaries = append(binaries, name)
		}
	}
	if len(binaries) != len(platforms) {
		t.Errorf("binaires publiés = %v, attendu %d", binaries, len(platforms))
	}
	for _, p := range platforms {
		name := "kubectl-telemach_" + version + "_" + p
		path, ok := files[name]
		if !ok {
			t.Errorf("binaire manquant : %s", name)
			continue
		}
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			t.Errorf("%s : informations de build illisibles : %v", name, err)
			continue
		}
		if info.Main.Path != modulePath {
			t.Errorf("%s : module %q, attendu %q", name, info.Main.Path, modulePath)
		}
		// Avec -trimpath, Go n'inscrit pas -ldflags dans les informations de build ; la version
		// lisible sur tout binaire (même darwin) est celle du module, issue du tag git. Elle
		// vaut vX.Y.Z pour une Release (dépôt propre sur le tag), une pseudo-version sinon.
		if !strings.Contains(version, "SNAPSHOT") && info.Main.Version != "v"+version {
			t.Errorf("%s : version du module %q, attendu %q", name, info.Main.Version, "v"+version)
		}
	}
}

// Chaque archive contient exactement le binaire, LICENSE et NOTICE, et son binaire est
// identique au binaire brut publié (même somme) : Krew installe ce que la voie manuelle installe.
func TestArchives(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)
	for _, p := range platforms {
		name := "kubectl-telemach_" + version + "_" + p + ".tar.gz"
		path, ok := files[name]
		if !ok {
			t.Errorf("archive manquante : %s", name)
			continue
		}
		f, err := os.Open(path) //nolint:gosec // artefacts de la compilation
		if err != nil {
			t.Fatal(err)
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("%s : %v", name, err)
		}
		tr := tar.NewReader(gz)
		var entries []string
		binarySum := ""
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s : %v", name, err)
			}
			entries = append(entries, h.Name)
			if h.Name == "kubectl-telemach" {
				if h.Mode&0o111 == 0 {
					t.Errorf("%s : kubectl-telemach n'est pas exécutable (%o)", name, h.Mode)
				}
				binarySum = sha256Reader(t, tr)
			}
		}
		_ = f.Close()
		sort.Strings(entries)
		if strings.Join(entries, ",") != strings.Join(archiveContent, ",") {
			t.Errorf("%s : contenu %v, attendu %v", name, entries, archiveContent)
		}
		raw, ok := files["kubectl-telemach_"+version+"_"+p]
		if ok && binarySum != sha256File(t, raw) {
			t.Errorf("%s : le binaire de l'archive diffère du binaire brut publié", name)
		}
	}
}

func TestChecksumsCoverArtifacts(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)
	sumPath, ok := files["checksums.txt"]
	if !ok {
		t.Fatal("checksums.txt absent des artefacts")
	}
	sums := checksums(t, sumPath)
	_, withSBOM := files["kubectl-telemach_"+version+"_linux_amd64.sbom.json"]
	for _, p := range platforms {
		names := []string{"kubectl-telemach_" + version + "_" + p, "kubectl-telemach_" + version + "_" + p + ".tar.gz"}
		if withSBOM {
			names = append(names, "kubectl-telemach_"+version+"_"+p+".sbom.json")
		}
		for _, name := range names {
			want, ok := sums[name]
			if !ok {
				t.Errorf("checksums.txt ne couvre pas %s", name)
				continue
			}
			if path, ok := files[name]; ok {
				if got := sha256File(t, path); got != want {
					t.Errorf("%s : somme %s, checksums.txt indique %s", name, got, want)
				}
			}
		}
	}
}

// SBOM SPDX de chaque binaire ; ignoré pour un snapshot local construit sans Syft
// (--skip=sbom), jamais en CI.
func TestSBOMs(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)
	if _, ok := files["kubectl-telemach_"+version+"_linux_amd64.sbom.json"]; !ok && os.Getenv("CI") == "" {
		t.Skip("snapshot local sans SBOM (--skip=sbom)")
	}
	for _, p := range platforms {
		name := "kubectl-telemach_" + version + "_" + p + ".sbom.json"
		path, ok := files[name]
		if !ok {
			t.Errorf("SBOM manquant : %s", name)
			continue
		}
		var doc struct {
			SPDXVersion string `json:"spdxVersion"`
		}
		readJSON(t, path, &doc)
		if !strings.HasPrefix(doc.SPDXVersion, "SPDX-") {
			t.Errorf("%s : spdxVersion = %q, document SPDX attendu", name, doc.SPDXVersion)
		}
	}
}

func TestVersionOutput(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)
	name := "kubectl-telemach_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	path, ok := files[name]
	if !ok {
		t.Skipf("pas de binaire pour la plateforme courante (%s)", name)
	}
	out, err := exec.Command(path, "version").Output() //nolint:gosec // binaire de la compilation
	if err != nil {
		t.Fatalf("%s version : %v", name, err)
	}
	first, _, _ := strings.Cut(string(out), "\n")
	if want := "TELEMACH PLUGIN KUBECTL " + version; first != want {
		t.Errorf("première ligne = %q, attendu %q", first, want)
	}
}
