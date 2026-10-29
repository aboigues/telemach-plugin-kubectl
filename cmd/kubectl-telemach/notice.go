package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// noticeEntry est la seule entrée expliquée après une commande réussie (feature 004, FR-007).
const noticeEntry = "admission.pod-security-warning"

// noticeCommand explique un avertissement de kubectl écrit par une commande RÉUSSIE
// (« Warning: would violate PodSecurity … ») ; appelée par l'intégration shell, elle ne lit
// jamais le cluster et rend toujours le code 0 (research.md F5).
func (a *app) noticeCommand() *cobra.Command {
	var stderrFile string
	cmd := &cobra.Command{
		Use:    "notice --stderr-file <fichier> -- <args kubectl…>",
		Short:  "Explique un avertissement d'une commande réussie (usage interne)",
		Hidden: true,
		RunE: func(_ *cobra.Command, args []string) error {
			a.notice(stderrFile, args)
			return nil
		},
	}
	cmd.Flags().StringVar(&stderrFile, "stderr-file", "", "fichier contenant la sortie d'erreur de la commande")
	return cmd
}

func (a *app) notice(stderrFile string, args []string) {
	raw, err := readTail(stderrFile, maxStderr)
	if err != nil {
		return
	}
	cat, err := catalog.Default()
	if err != nil {
		return
	}
	cmd := command.Parse(cleanArgs(args))
	d := diagnose.Diagnose(diagnose.Input{Command: cmd, Stderr: redact.Redact(sanitize.Clean(raw))}, cat)
	if d.Entry == nil || d.Entry.ID != noticeEntry {
		return
	}
	ns := cmd.Namespace
	if ns == "" {
		ns = "courant"
	}
	key, now := "telemach:pod-security:"+ns, a.now()
	if !a.store.ShouldHint(key, now) {
		return
	}
	theme, w, width := a.output(a.stderr)
	fmt.Fprintln(w, theme.Hint(render.Wrap(render.SymbolInfo+" "+catalog.Render(d.Entry.Summary, d.Captures)+
		" Pour comprendre et corriger : kubectl telemach deepen", width, "")))
	_ = a.store.RememberHint(key, now)
	// Enregistré comme dernier diagnostic : last et deepen l'expliquent en détail.
	_ = a.store.Save(state.LastDiagnosis{
		Fingerprint: d.Fingerprint, Captures: d.Captures, RecordedAt: now, Source: state.SourceCommand,
		Origin: string(d.Origin), Confidence: string(d.Confidence), EntryID: d.Entry.ID,
		InvokedAs: "kubectl", Stderr: d.Captures["violations"], Args: cmd.Args, Subcommand: cmd.Sub(),
	})
}
