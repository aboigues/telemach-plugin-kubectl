package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// maxHeaderCommand limite la longueur de la commande rappelée en tête de « last ».
const maxHeaderCommand = 70

// lastCommand réaffiche l'explication complète du dernier diagnostic (FR-009a), par exemple
// après une version courte pour une erreur répétée, ou après avoir effacé le terminal.
func (a *app) lastCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "last",
		Short: "Réaffiche l'explication complète de la dernière erreur",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			theme, w, width := a.output(a.stderr)
			a.shellStatusWarning(w, theme, width)
			last, ok := a.store.Load()
			if !ok {
				fmt.Fprintln(w, theme.Hint("Aucune erreur récente à réafficher."))
				fmt.Fprintln(w, render.Wrap("Dès qu'une commande kubectl échoue avec l'intégration shell activée, "+
					"« kubectl telemach last » réaffiche son explication complète.", width, "   "))
				return nil
			}
			// Quand et pour quelle commande : une erreur ancienne ne doit pas passer pour la
			// dernière commande tapée.
			fmt.Fprintln(w, theme.Dim(lastHeader(last, a.now())))
			fmt.Fprintln(w)
			return a.renderLast(w, last, theme, width)
		},
	}
}

// renderLast reconstitue le diagnostic enregistré et l'affiche en entier (jamais en version
// courte).
func (a *app) renderLast(w io.Writer, last state.LastDiagnosis, theme render.Theme, width int) error {
	d := diagnose.Diagnosis{
		Source:      diagnose.Source(last.Source),
		Confidence:  diagnose.Confidence(last.Confidence),
		Origin:      diagnose.Origin(last.Origin),
		Captures:    last.Captures,
		Fingerprint: last.Fingerprint,
	}
	if d.Captures == nil {
		d.Captures = map[string]string{}
	}
	if cat, err := catalog.Default(); err == nil && last.EntryID != "" {
		d.Entry, _ = cat.Get(last.EntryID)
	}
	opts := render.Options{
		Theme: theme, Width: width, KubectlVersion: last.KubectlVersion,
		Interactive: last.Interactive, ExitCode: last.ExitCode,
	}
	if last.Source == state.SourcePod && len(last.Pod) > 0 {
		// Diagnostic de pod réaffiché sans relire le cluster (FR-027).
		var snap workload.PodSnapshot
		if err := json.Unmarshal(last.Pod, &snap); err == nil {
			return render.RenderPod(w, snap, d, opts)
		}
	}
	if last.Source != state.SourceCheck {
		if err := render.RenderExplanation(w, d, last.Stderr, opts); err != nil {
			return err
		}
	}
	if len(last.Files) == 0 {
		return nil
	}
	// Manifestes réanalysés dans leur état actuel (contracts/cli.md § last).
	r := manifest.Analyze(last.Files, false, last.KubectlVersion)
	if last.Source != state.SourceCheck {
		r.AddLocations(manifest.Locate(last.Stderr, r.Files), firstLine(last.Stderr))
		fmt.Fprintln(w)
	}
	cat, _ := catalog.Default()
	kubectl := last.InvokedAs
	if kubectl == "" {
		kubectl = "kubectl"
	}
	return render.RenderManifestIssues(w, r, cat, render.ManifestOptions{Options: opts, Kubectl: kubectl, ShowRules: true, ShowSuccess: true})
}

// lastHeader rappelle la date et la commande du diagnostic réaffiché.
func lastHeader(last state.LastDiagnosis, now time.Time) string {
	kubectl := last.InvokedAs
	if kubectl == "" {
		kubectl = "kubectl"
	}
	command := kubectl
	if len(last.Args) > 0 {
		command += " " + strings.Join(last.Args, " ")
	} else if last.Subcommand != "" {
		command += " " + last.Subcommand
	}
	if r := []rune(command); len(r) > maxHeaderCommand {
		command = string(r[:maxHeaderCommand-1]) + "…"
	}
	if last.RecordedAt.IsZero() {
		return render.LabelLast + " de la dernière erreur enregistrée : " + command
	}
	at := last.RecordedAt.In(now.Location())
	day := "le " + at.Format("02/01/2006") + " à " + at.Format("15:04")
	if y, m, dd := now.Date(); at.Year() == y && at.Month() == m && at.Day() == dd {
		day = "aujourd'hui à " + at.Format("15:04")
	}
	return fmt.Sprintf("%s de l'erreur enregistrée %s : %s", render.LabelLast, day, command)
}
