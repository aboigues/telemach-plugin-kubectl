package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
)

// deepenCommand approfondit le concept du dernier diagnostic (US3, FR-009).
func (a *app) deepenCommand() *cobra.Command {
	var exercise bool
	cmd := &cobra.Command{
		Use:   "deepen [--exercise]",
		Short: "Approfondit le concept Kubernetes du dernier diagnostic",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			theme, w, width := a.output(a.stderr)
			a.shellStatusWarning(w, theme, width)
			last, ok := a.store.Load()
			cat, err := catalog.Default()
			var entry *catalog.Entry
			if ok && err == nil && last.EntryID != "" {
				entry, ok = cat.Get(last.EntryID)
			}
			if !ok || entry == nil {
				fmt.Fprintln(w, theme.Hint("Aucun diagnostic récent à approfondir."))
				fmt.Fprintln(w, render.Wrap("Active l'explication automatique avec « eval \"$(kubectl-telemach shell-init zsh)\" » "+
					"(ou bash), puis lance une commande kubectl : dès qu'une erreur est expliquée, "+
					"« kubectl telemach deepen » permet d'aller plus loin. « kubectl telemach check » et "+
					"« kubectl telemach diagnose » enregistrent aussi leur diagnostic.", width, "   "))
				return nil
			}
			captures := last.Captures
			if captures == nil {
				captures = map[string]string{}
			}
			if last.InvokedAs != "" {
				captures["kubectl"] = last.InvokedAs
			}
			return render.RenderDeepen(w, entry, captures, exercise, render.Options{Theme: theme, Width: width})
		},
	}
	cmd.Flags().BoolVar(&exercise, "exercise", false, "affiche aussi un exercice court et sans risque")
	return cmd
}
