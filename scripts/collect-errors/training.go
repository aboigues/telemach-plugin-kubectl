//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

// Relevé d'une formation (feature 004, research.md F13, FR-026) : chaque panne
// (tpNN-*/pannes/*.yaml) est appliquée dans un namespace neuf, après les prérequis facultatifs
// du TP (fichier telemach-releve.yaml à la racine du TP) ; on relève alors l'explication de
// l'application et le diagnostic du namespace. Le rapport liste ce qui reste non reconnu.

// trainingPrereqs est le contenu facultatif de <tp>/telemach-releve.yaml.
type trainingPrereqs struct {
	Apply []string `yaml:"apply"` // manifestes ou dossiers à appliquer avant la panne (chemins relatifs au TP)
	Wait  string   `yaml:"wait"`  // attente avant la panne, par exemple « 30s »
}

// trainingPanne est une panne à rejouer.
type trainingPanne struct {
	TP, File string // dossier du TP, chemin de la panne
}

// TrainingFinding est une ligne du rapport (data-model.md § TrainingFinding).
type TrainingFinding struct {
	TP, Panne, Command, Message, Result string
}

// discoverPannes liste les pannes d'un dépôt de formation, triées.
func discoverPannes(root string) ([]trainingPanne, error) {
	files, err := filepath.Glob(filepath.Join(root, "tp*", "pannes", "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []trainingPanne
	for _, f := range files {
		out = append(out, trainingPanne{TP: filepath.Dir(filepath.Dir(f)), File: f})
	}
	return out, nil
}

// classifyOutput classe une sortie de TELEMACH : entrée reconnue, « unknown » (erreur non
// reconnue) ou « unsupported » (type non pris en charge par diagnose).
func classifyOutput(out string) string {
	switch {
	case strings.Contains(out, "Erreur non reconnue"):
		return "unknown"
	case strings.Contains(out, "n'est pas pris en charge par diagnose"), strings.Contains(out, "pas encore disponible"):
		return "unsupported"
	case strings.Contains(out, "Aucun pod en difficulté") && !strings.Contains(out, "Autres objets en difficulté"):
		// Rien n'est expliqué : la panne ne s'est pas (encore) manifestée, souvent faute des
		// prérequis du TP. Ce n'est ni une réussite ni une erreur non reconnue.
		return "rien signalé"
	}
	return "reconnue"
}

// renderReport écrit le tableau Markdown, avec un résumé des lignes non reconnues.
func renderReport(findings []TrainingFinding) string {
	var b strings.Builder
	b.WriteString("| TP | Panne | Commande | Message | Résultat |\n|----|-------|----------|---------|----------|\n")
	missing := 0
	for _, f := range findings {
		if f.Result == "unknown" || f.Result == "unsupported" {
			missing++
		}
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | %s |\n", f.TP, f.Panne, f.Command, strings.ReplaceAll(f.Message, "|", "\\|"), f.Result)
	}
	fmt.Fprintf(&b, "\n%d ligne(s), %d non reconnue(s) ou non prise(s) en charge, %d sans signal.\n", len(findings), missing, count(findings, "rien signalé"))
	return b.String()
}

// collectTraining rejoue les pannes d'un dépôt de formation sur un cluster jetable.
func collectTraining(root, report, kindConfig string) error {
	image := os.Getenv("TELEMACH_KIND_IMAGE")
	if image == "" {
		return fmt.Errorf("TELEMACH_KIND_IMAGE est obligatoire (image kindest/node épinglée par digest)")
	}
	pannes, err := discoverPannes(root)
	if err != nil || len(pannes) == 0 {
		return fmt.Errorf("aucune panne trouvée sous %s/tp*/pannes/ : %v", root, err)
	}
	plugin := filepath.Join(os.TempDir(), fmt.Sprintf("kubectl-telemach-releve-%d", os.Getpid()))
	if out, err := exec.Command("go", "build", "-o", plugin, "./cmd/kubectl-telemach").CombinedOutput(); err != nil { //nolint:gosec // outil du mainteneur
		return fmt.Errorf("compilation du plugin : %v\n%s", err, out)
	}
	defer func() { _ = os.Remove(plugin) }()
	c, err := kindcluster.CreateWithConfig(image, kindConfig)
	if err != nil {
		return err
	}
	defer func() {
		if err := c.Delete(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	var findings []TrainingFinding
	for i, p := range pannes {
		ns := fmt.Sprintf("panne-%02d", i+1)
		if err := c.Must("create", "namespace", ns); err != nil {
			return err
		}
		tp := filepath.Base(p.TP)
		applyPrereqs(c, p.TP, ns)
		args := []string{"apply", "-f", p.File, "-n", ns}
		r := c.Kubectl(args...)
		if r.ExitCode != 0 {
			findings = append(findings, explainFinding(c, plugin, tp, p, args, r))
		}
		time.Sleep(60 * time.Second)
		findings = append(findings, diagnoseFinding(c, plugin, tp, p, ns))
		_ = c.Kubectl("delete", "namespace", ns, "--wait=false")
	}
	return os.WriteFile(report, []byte(renderReport(findings)), 0o600) //nolint:gosec // rapport du mainteneur
}

// applyPrereqs applique les prérequis déclarés par le TP ; un échec est signalé sans arrêter.
func applyPrereqs(c *kindcluster.Cluster, tp, ns string) {
	data, err := os.ReadFile(filepath.Join(tp, "telemach-releve.yaml")) //nolint:gosec // dépôt de formation choisi par le mainteneur
	if err != nil {
		return
	}
	var pre trainingPrereqs
	if err := yaml.Unmarshal(data, &pre); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ %s : telemach-releve.yaml illisible : %v\n", tp, err)
		return
	}
	for _, a := range pre.Apply {
		if r := c.Kubectl("apply", "-f", filepath.Join(tp, a), "-n", ns); r.ExitCode != 0 {
			fmt.Fprintf(os.Stderr, "⚠ %s : prérequis %s : %s", tp, a, r.Stderr)
		}
	}
	if d, err := time.ParseDuration(pre.Wait); err == nil {
		time.Sleep(d)
	}
}

// explainFinding rejoue l'explication de la commande en échec.
func explainFinding(c *kindcluster.Cluster, plugin, tp string, p trainingPanne, args []string, r kindcluster.Result) TrainingFinding {
	f := filepath.Join(c.Dir, "stderr.txt")
	_ = os.WriteFile(f, []byte(r.Stderr), 0o600)                                                                                                                               //nolint:gosec // dossier du cluster de test
	out, _ := exec.Command(plugin, append([]string{"explain", "--color=never", "--exit-code", fmt.Sprint(r.ExitCode), "--stderr-file", f, "--"}, args...)...).CombinedOutput() //nolint:gosec // plugin compilé par l'outil
	return TrainingFinding{
		TP: tp, Panne: filepath.Base(p.File), Command: "kubectl " + strings.Join(args[:3], " "),
		Message: normalize(firstLine(r.Stderr), c), Result: classifyOutput(string(out)),
	}
}

// diagnoseFinding relève le diagnostic du namespace de la panne.
func diagnoseFinding(c *kindcluster.Cluster, plugin, tp string, p trainingPanne, ns string) TrainingFinding {
	cmd := exec.Command(plugin, "diagnose", "--color=never", "-n", ns) //nolint:gosec // plugin compilé par l'outil
	cmd.Env = append(c.Env(), "TELEMACH_KUBECTL="+c.KubectlBin)
	out, _ := cmd.CombinedOutput()
	text := redact.Redact(string(out))
	return TrainingFinding{
		TP: tp, Panne: filepath.Base(p.File), Command: "kubectl telemach diagnose -n " + ns,
		Message: strings.TrimSpace(firstLine(text)), Result: classifyOutput(text),
	}
}

// count compte les lignes d'un résultat donné.
func count(findings []TrainingFinding, result string) int {
	n := 0
	for _, f := range findings {
		if f.Result == result {
			n++
		}
	}
	return n
}
