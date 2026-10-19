// Commande krew-manifest : outil de publication (jamais appelé par le plugin) qui écrit le
// manifeste Krew du plugin telemach à partir du checksums.txt d'une Release (research.md R9,
// contracts/krew-manifest.md).
//
//	go run ./scripts/krew-manifest -checksums checksums.txt -version 0.1.0 -out telemach.yaml
//
// Il échoue, sans rien écrire, si une des quatre archives manque ou pour une pré-version :
// seules les versions stables entrent dans l'index (FR-008).
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"
)

// defaultBaseURL est l'adresse des fichiers d'une Release ; {{version}} reçoit la version.
const defaultBaseURL = "https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v{{version}}"

// platforms sont les plateformes publiées, dans l'ordre du manifeste.
var platforms = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"}}

var (
	stableVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	anyVersion    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z.-]+$`)
	sha256Hex     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// platform est une entrée du manifeste.
type platform struct {
	OS, Arch, URI, Sha256 string
}

func main() {
	checksums := flag.String("checksums", "checksums.txt", "fichier de sommes de la Release")
	version := flag.String("version", "", "version publiée, sans le v (ex. 0.1.0)")
	base := flag.String("base-url", defaultBaseURL, "adresse des archives ({{version}} est remplacé)")
	tmplPath := flag.String("template", "packaging/krew/telemach.yaml.tmpl", "modèle du manifeste")
	out := flag.String("out", "", "fichier écrit (sortie standard si vide)")
	snapshot := flag.Bool("allow-prerelease", false, "accepte une pré-version : validation d'un snapshot en CI uniquement, jamais pour l'index")
	flag.Parse()
	if err := runCLI(*checksums, *version, *base, *tmplPath, *out, *snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "krew-manifest :", err)
		os.Exit(1)
	}
}

func runCLI(checksums, version, base, tmplPath, out string, allowPrerelease bool) error {
	sums, err := os.Open(checksums) //nolint:gosec // fichier désigné par le mainteneur ou la CI
	if err != nil {
		return err
	}
	defer sums.Close()                 //nolint:errcheck
	tmpl, err := os.ReadFile(tmplPath) //nolint:gosec // modèle du dépôt
	if err != nil {
		return err
	}
	manifest, err := generate(sums, version, base, string(tmpl), allowPrerelease)
	if err != nil {
		return err
	}
	if out == "" {
		_, err = os.Stdout.Write(manifest)
		return err
	}
	return os.WriteFile(out, manifest, 0o644) //nolint:gosec // manifeste public
}

// generate rend le manifeste d'une version stable ; les sommes sont celles des archives dans
// checksums.txt. allowPrerelease n'est vrai que pour valider le manifeste d'un snapshot (job
// krew-manifest de la CI) ; la publication de l'index ne le demande jamais.
func generate(checksums io.Reader, version, base, tmpl string, allowPrerelease bool) ([]byte, error) {
	switch {
	case anyVersion.MatchString(version) && !allowPrerelease:
		return nil, fmt.Errorf("%s est une pré-version : l'index Krew ne reçoit que les versions stables", version)
	case !stableVersion.MatchString(version) && !anyVersion.MatchString(version):
		return nil, fmt.Errorf("version %q invalide (attendu X.Y.Z, sans le v)", version)
	}
	sums := map[string]string{}
	sc := bufio.NewScanner(checksums)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 {
			sums[fields[1]] = fields[0]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	base = strings.TrimSuffix(strings.ReplaceAll(base, "{{version}}", version), "/")
	var list []platform
	for _, p := range platforms {
		name := "kubectl-telemach_" + version + "_" + p[0] + "_" + p[1] + ".tar.gz"
		sum, ok := sums[name]
		if !ok {
			return nil, fmt.Errorf("archive %s absente de checksums.txt (%s_%s)", name, p[0], p[1])
		}
		if !sha256Hex.MatchString(sum) {
			return nil, fmt.Errorf("somme invalide pour %s : %q", name, sum)
		}
		list = append(list, platform{OS: p[0], Arch: p[1], URI: base + "/" + name, Sha256: sum})
	}
	t, err := template.New("manifest").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, struct {
		Version   string
		Platforms []platform
	}{version, list}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
