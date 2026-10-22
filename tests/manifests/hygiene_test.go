package manifests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// LL-007 : aucun exécutable compilé n'est versionné (un « go build » à la racine suivi de
// « git add -A » l'a fait une fois). Les binaires sont reconnus à leur en-tête : ELF, Mach-O,
// PE.
func TestNoBinaryCommitted(t *testing.T) {
	out, err := exec.Command("git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git indisponible : %v", err)
	}
	magics := [][]byte{
		{0x7f, 'E', 'L', 'F'},    // ELF (Linux)
		{0xcf, 0xfa, 0xed, 0xfe}, // Mach-O 64 bits (macOS)
		{'M', 'Z'},               // PE (Windows)
	}
	files := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	if len(files) < 10 {
		t.Fatalf("liste des fichiers versionnés suspecte (D-LL-009) : %d fichier(s)", len(files))
	}
	for _, f := range files {
		fh, err := os.Open("../../" + f) //nolint:gosec // fichiers du dépôt
		if err != nil {
			continue // supprimé du dossier de travail mais pas encore de l'index
		}
		head := make([]byte, 4)
		n, _ := fh.Read(head)
		_ = fh.Close()
		for _, m := range magics {
			if n >= len(m) && bytes.Equal(head[:len(m)], m) {
				t.Errorf("%s : exécutable compilé versionné", f)
			}
		}
	}
}

// workflow est la partie d'un workflow GitHub Actions utile aux contrôles ci-dessous.
type workflow struct {
	Permissions any `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions any    `yaml:"permissions"`
		RunsOn      any    `yaml:"runs-on"`
		Timeout     int    `yaml:"timeout-minutes"`
		Uses        string `yaml:"uses"`
		Secrets     any    `yaml:"secrets"`
		Steps       []struct {
			Name    string            `yaml:"name"`
			Run     string            `yaml:"run"`
			Uses    string            `yaml:"uses"`
			Env     map[string]string `yaml:"env"`
			Timeout int               `yaml:"timeout-minutes"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readWorkflows(t *testing.T) map[string]workflow {
	t.Helper()
	files, _ := filepath.Glob("../../.github/workflows/*.yml")
	if len(files) < 3 {
		t.Fatalf("workflows introuvables (D-LL-009) : %v", files)
	}
	out := map[string]workflow{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // fichiers du dépôt
		if err != nil {
			t.Fatal(err)
		}
		var w workflow
		if err := yaml.Unmarshal(data, &w); err != nil {
			t.Fatalf("%s : %v", f, err)
		}
		out[filepath.Base(f)] = w
	}
	return out
}

// Principe IV (T028) : aucune expression ${{ … }} dans un script « run: » (injection de
// commande par un nom de branche, de tag ou une entrée) ; les valeurs passent par « env: ».
func TestWorkflowsNoExpressionInRun(t *testing.T) {
	scripts := 0
	for name, w := range readWorkflows(t) {
		for job, j := range w.Jobs {
			for _, s := range j.Steps {
				if s.Run != "" {
					scripts++
				}
				if strings.Contains(s.Run, "${{") {
					t.Errorf("%s, job %s, étape « %s » : expression ${{ … }} dans run (passer par env:)", name, job, s.Name)
				}
			}
		}
	}
	if scripts < 20 {
		t.Fatalf("seulement %d scripts run: lus : analyse des workflows suspecte (D-LL-009)", scripts)
	}
}

// Principe IV (T028) : permissions déclarées au niveau du workflow, et écriture réservée au
// job « release » de release.yml.
func TestWorkflowsLeastPrivilege(t *testing.T) {
	for name, w := range readWorkflows(t) {
		if w.Permissions == nil {
			t.Errorf("%s : permissions absentes au niveau du workflow", name)
		}
		for job, j := range w.Jobs {
			perms, _ := j.Permissions.(map[string]any)
			for scope, level := range perms {
				if level == "write" && (name != "release.yml" || job != "release") {
					t.Errorf("%s, job %s : %s: write hors du job release", name, job, scope)
				}
			}
		}
	}
}

// LL-011 : chaque job exécuté sur un runner a un délai maximal (sinon GitHub attend 6 heures),
// et chaque étape apt-get, qui dépend d'un miroir externe, a le sien. Les jobs qui appellent
// un workflow réutilisable (uses:) héritent des délais de ses jobs.
func TestWorkflowsTimeouts(t *testing.T) {
	jobs := 0
	for name, w := range readWorkflows(t) {
		for job, j := range w.Jobs {
			if j.RunsOn == nil {
				continue
			}
			jobs++
			if j.Timeout <= 0 {
				t.Errorf("%s, job %s : timeout-minutes absent", name, job)
			}
			for _, st := range j.Steps {
				if strings.Contains(st.Run, "apt-get") && st.Timeout <= 0 {
					t.Errorf("%s, job %s, étape « %s » : apt-get sans timeout-minutes", name, job, st.Name)
				}
			}
		}
	}
	if jobs < 15 {
		t.Fatalf("seulement %d jobs lus : analyse des workflows suspecte (D-LL-009)", jobs)
	}
}

// LL-012 : GoReleaser reçoit le tag qui a déclenché la publication. Sans cela, il choisit parmi
// les tags du commit (v0.1.0 et v0.1.0-rc.2 sur le même commit) et a republié une pré-version,
// en écrasant son image signée, alors que verify avait contrôlé l'autre version.
func TestReleaseUsesTriggeringTag(t *testing.T) {
	w, ok := readWorkflows(t)["release.yml"]
	if !ok {
		t.Fatal("release.yml introuvable")
	}
	found := 0
	for _, st := range w.Jobs["release"].Steps {
		if !strings.HasPrefix(st.Uses, "goreleaser/goreleaser-action@") {
			continue
		}
		found++
		if got := st.Env["GORELEASER_CURRENT_TAG"]; got != "${{ github.ref_name }}" {
			t.Errorf("étape « %s » : GORELEASER_CURRENT_TAG = %q, attendu ${{ github.ref_name }}", st.Name, got)
		}
	}
	if found != 1 {
		t.Fatalf("%d étape(s) GoReleaser dans le job release, 1 attendue (D-LL-009)", found)
	}
}

// secretRef repère un secret lu par un workflow (GITHUB_TOKEN est fourni à tout workflow).
var secretRef = regexp.MustCompile(`secrets\.([A-Z][A-Z0-9_]*)`)

// LL-013 : un workflow appelé (uses: ./…) ne lit un secret, même d'environnement, que si
// l'appelant le lui transmet ; la transmission est nominative (jamais « secrets: inherit »).
func TestCalledWorkflowsReceiveTheirSecrets(t *testing.T) {
	workflows := readWorkflows(t)
	calls := 0
	for name, w := range workflows {
		for job, j := range w.Jobs {
			called, ok := strings.CutPrefix(j.Uses, "./.github/workflows/")
			if !ok {
				continue
			}
			calls++
			if j.Secrets == "inherit" {
				t.Errorf("%s, job %s : secrets: inherit (transmettre chaque secret par son nom)", name, job)
				continue
			}
			passed, _ := j.Secrets.(map[string]any)
			data, err := os.ReadFile(filepath.Join("../../.github/workflows", called)) //nolint:gosec // fichier du dépôt
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range secretRef.FindAllStringSubmatch(string(data), -1) {
				if m[1] == "GITHUB_TOKEN" {
					continue
				}
				if _, ok := passed[m[1]]; !ok {
					t.Errorf("%s, job %s : %s lit secrets.%s, que l'appelant ne transmet pas", name, job, called, m[1])
				}
			}
		}
	}
	if calls < 2 {
		t.Fatalf("seulement %d appel(s) de workflow lu(s) : analyse suspecte (D-LL-009)", calls)
	}
}
