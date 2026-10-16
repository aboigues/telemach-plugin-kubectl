package main

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// maxStderr est la taille maximale lue : au-delà, seule la fin du message est conservée.
const maxStderr = 1 << 20

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+([-+.][0-9A-Za-z.-]+)?$`)

type explainFlags struct {
	exitCode       int
	stderrFile     string
	interactive    bool
	kubectlVersion string
	invokedAs      string
}

func (a *app) explainCommand() *cobra.Command {
	var f explainFlags
	cmd := &cobra.Command{
		Use:   "explain --exit-code <n> [--stderr-file <chemin>] [--interactive] [--kubectl-version <v>] [--invoked-as kubectl|k] -- <args kubectl…>",
		Short: "Explique une commande kubectl en échec (appelée par l'intégration shell)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if f.exitCode < 1 || f.exitCode > 255 {
				fmt.Fprintf(a.stderr, "kubectl telemach : --exit-code doit être compris entre 1 et 255 (reçu %d)\n", f.exitCode)
				return exitError{code: 2}
			}
			if f.invokedAs != "kubectl" && f.invokedAs != "k" {
				fmt.Fprintf(a.stderr, "kubectl telemach : --invoked-as vaut kubectl ou k (reçu %q)\n", f.invokedAs)
				return exitError{code: 2}
			}
			return a.explain(f, args)
		},
	}
	cmd.Flags().IntVar(&f.exitCode, "exit-code", 0, "code de sortie de la commande kubectl (1 à 255)")
	cmd.Flags().StringVar(&f.stderrFile, "stderr-file", "", "fichier contenant la sortie d'erreur de la commande")
	cmd.Flags().BoolVar(&f.interactive, "interactive", false, "la commande était interactive (sortie d'erreur non capturée)")
	cmd.Flags().StringVar(&f.kubectlVersion, "kubectl-version", "", "version du client kubectl")
	cmd.Flags().StringVar(&f.invokedAs, "invoked-as", "kubectl", "nom tapé par l'apprenant : kubectl ou k")
	_ = cmd.MarkFlagRequired("exit-code")
	return cmd
}

// explain enchaîne : lecture → nettoyage → masquage → analyse → diagnostic → rendu. Une
// explication est toujours produite (code 0), même pour une erreur non reconnue.
func (a *app) explain(f explainFlags, args []string) error {
	raw := ""
	if f.stderrFile != "" && !f.interactive {
		var err error
		if raw, err = readTail(f.stderrFile, maxStderr); err != nil {
			raw = ""
		}
	}
	stderr := redact.Redact(sanitize.Clean(raw))
	clean := cleanArgs(args)

	cmd := command.Parse(clean)
	cat, err := catalog.Default()
	if err != nil {
		cat = nil // catalogue invalide : le repli « non reconnue » reste disponible
	}
	in := diagnose.Input{
		Command: cmd, ExitCode: f.exitCode, Stderr: stderr,
		Interactive: f.interactive || cmd.Interactive, InvokedAs: f.invokedAs,
	}
	d := diagnose.Diagnose(in, cat)

	now := a.now()
	repeated := a.store.IsRepeat(d.Fingerprint, now)
	kubectlVersion := ""
	if versionPattern.MatchString(f.kubectlVersion) {
		kubectlVersion = f.kubectlVersion
	}
	theme, w, width := a.output(a.stderr)
	opts := render.Options{
		Theme: theme, Width: width, KubectlVersion: kubectlVersion, Repeated: repeated,
		Interactive: in.Interactive, ExitCode: f.exitCode,
	}
	fmt.Fprintln(w)
	if err := render.RenderExplanation(w, d, stderr, opts); err != nil {
		return err
	}
	last := state.LastDiagnosis{
		Fingerprint: d.Fingerprint, Captures: d.Captures, RecordedAt: now, Source: state.SourceCommand,
		Origin: string(d.Origin), Confidence: string(d.Confidence), ExitCode: f.exitCode,
		Interactive: in.Interactive, InvokedAs: f.invokedAs, KubectlVersion: kubectlVersion,
		Stderr: stderr, Args: clean, Subcommand: cmd.Sub(),
	}
	if d.Entry != nil {
		last.EntryID = d.Entry.ID
	}
	_ = a.store.Save(last)
	return nil
}

// readTail lit au plus limit octets, en gardant la fin du fichier, sans le modifier.
func readTail(path string, limit int64) (string, error) {
	f, err := os.Open(path) //nolint:gosec // chemin fourni par l'intégration shell
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > limit {
		if _, err := f.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	return string(data), err
}

// cleanArgs prépare les arguments pour l'analyse, le disque et l'affichage : séquences de
// contrôle retirées, secrets masqués.
func cleanArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = sanitize.Clean(arg)
	}
	return redact.RedactArgs(out)
}
