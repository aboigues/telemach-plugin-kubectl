package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
)

// checkCommand vérifie des manifestes avant toute application, sans contacter le cluster
// (FR-014, FR-014a, FR-017, FR-017a).
func (a *app) checkCommand() *cobra.Command {
	var files []string
	var recursive bool
	cmd := &cobra.Command{
		Use:   "check -f <fichier|dossier|-> [-f …] [-R]",
		Short: "Vérifie des manifestes sans contacter le cluster",
		Long: `Vérifie la syntaxe YAML et les règles de Kubernetes des manifestes désignés, en lecture
seule et sans contacter le cluster. Les règles appliquées sont celles de la version de ton
client kubectl (1.35, 1.36 ou 1.37), ou les plus récentes. Avec -f -, le manifeste est lu sur
l'entrée standard : « helm template demo ./chart | kubectl telemach check -f - ».`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			theme, w, width := a.output(a.stderr)
			if len(files) == 0 {
				a.checkUsage(w, theme, width)
				return exitError{code: 2}
			}
			return a.check(w, theme, width, files, recursive)
		},
	}
	cmd.Flags().StringArrayVarP(&files, "filename", "f", nil, "fichier ou dossier de manifestes, ou - pour l'entrée standard (répétable)")
	cmd.Flags().BoolVarP(&recursive, "recursive", "R", false, "parcourir aussi les sous-dossiers")
	return cmd
}

// checkUsage explique l'option -f quand elle manque (FR-017a) : aucun fichier n'est lu.
func (a *app) checkUsage(w io.Writer, theme render.Theme, width int) {
	fmt.Fprintln(w, theme.Title(render.SymbolError+" Indique les manifestes à vérifier avec -f."))
	fmt.Fprintln(w, render.Wrap("Kubernetes n'a pas de fichier de manifeste par défaut : comme pour « kubectl apply », "+
		"l'option -f désigne un fichier, ou un dossier dont les fichiers .yaml, .yml et .json sont "+
		"tous vérifiés ; -R parcourt aussi ses sous-dossiers. Rien n'a été lu.", width, "   "))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "   "+theme.Highlight("kubectl telemach check -f deploy.yaml", "bash"))
	fmt.Fprintln(w, "   "+theme.Highlight("kubectl telemach check -f manifests/ -R", "bash"))
}

// check analyse les manifestes et renvoie le code 0 (aucun problème, informations
// éventuelles), 1 (au moins un problème) ou 2 (fichier introuvable).
func (a *app) check(w io.Writer, theme render.Theme, width int, files []string, recursive bool) error {
	if stdins := countStdin(files); stdins > 1 {
		fmt.Fprintln(w, theme.Title(render.SymbolError+" L'entrée standard (-f -) ne peut être lue qu'une seule fois."))
		fmt.Fprintln(w, render.Wrap("Comme avec kubectl, « - » ne peut figurer qu'une fois parmi les -f. Rien n'a été lu.", width, "   "))
		return exitError{code: 2}
	} else if stdins == 1 && a.stdinTerminal != nil && a.stdinTerminal() {
		a.stdinUsage(w, theme, width)
		return exitError{code: 2}
	}
	r := manifest.AnalyzeInput(files, recursive, a.getenv("TELEMACH_KUBECTL_VERSION"), a.stdin)
	cat, _ := catalog.Default()
	opts := render.ManifestOptions{
		Options: render.Options{Theme: theme, Width: width}, Kubectl: "kubectl", ShowRules: true, ShowSuccess: true,
	}
	if err := render.RenderManifestIssues(w, r, cat, opts); err != nil {
		return err
	}
	if r.Problems() == 0 {
		return nil // les informations (ℹ) ne changent pas le code de sortie (issue #12)
	}
	a.saveCheck(r, files, cat)
	for _, is := range r.Issues {
		if is.Kind == manifest.KindFileNotFound || is.Kind == manifest.KindNoManifestFile {
			return exitError{code: 2}
		}
	}
	return exitError{code: 1}
}

// countStdin compte les « - » parmi les valeurs de -f.
func countStdin(files []string) int {
	n := 0
	for _, f := range files {
		if f == "-" {
			n++
		}
	}
	return n
}

// stdinUsage explique -f - quand rien n'est redirigé vers l'entrée standard : check n'attend
// pas une saisie au clavier (FR-021).
func (a *app) stdinUsage(w io.Writer, theme render.Theme, width int) {
	fmt.Fprintln(w, theme.Title(render.SymbolError+" Rien n'est transmis à l'entrée standard (-f -)."))
	fmt.Fprintln(w, render.Wrap("Avec -f -, check lit le manifeste produit par une autre commande, placée avant « | » : "+
		"un rendu Helm, Kustomize ou un générateur kubectl. Rien n'a été lu.", width, "   "))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "   "+theme.Highlight("helm template demo ./chart | kubectl telemach check -f -", "bash"))
	fmt.Fprintln(w, "   "+theme.Highlight("kubectl kustomize overlays/dev | kubectl telemach check -f -", "bash"))
	fmt.Fprintln(w, "   "+theme.Highlight("kubectl create deployment web --image=nginxinc/nginx-unprivileged:1.30-alpine --dry-run=client -o yaml | kubectl telemach check -f -", "bash"))
}

// saveCheck enregistre la vérification comme dernier diagnostic, pour « last » et « deepen ».
func (a *app) saveCheck(r manifest.Result, files []string, cat *catalog.Catalog) {
	last := state.LastDiagnosis{RecordedAt: a.now(), Source: state.SourceCheck, Files: absPaths(files), Args: append([]string{"telemach", "check"}, files...)}
	if cat != nil {
		if e, ok := cat.ByIssueKind(r.Issues[0].Kind); ok {
			last.EntryID = e.ID
		}
	}
	_ = a.store.Save(last)
}

func absPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "-" || p == manifest.StdinPath {
			out = append(out, manifest.StdinPath) // jamais relu par last ni deepen
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			out = append(out, abs)
			continue
		}
		out = append(out, p)
	}
	return out
}
