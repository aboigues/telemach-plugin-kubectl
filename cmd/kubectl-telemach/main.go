// Commande kubectl-telemach : plugin kubectl pédagogique qui explique en français les erreurs
// de kubectl et des manifestes Kubernetes.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"

	"golang.org/x/term"
)

// version est renseignée à la compilation (-ldflags "-X main.version=…").
var version = "dev"

// exitError porte un code de sortie précis jusqu'à main.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("code de sortie %d", e.code) }

// app regroupe les dépendances injectables (pour les tests).
type app struct {
	stdout, stderr io.Writer
	stdin          io.Reader   // check -f - (feature 004)
	stdinTerminal  func() bool // l'entrée standard est-elle un terminal ?
	getenv         func(string) string
	now            func() time.Time
	getwd          func() (string, error)
	executable     func() string
	store          state.Store
	version        string

	colorFlag string
	width     int
}

func newApp() *app {
	return &app{
		stdout: os.Stdout,
		stderr: os.Stderr,
		stdin:  os.Stdin,
		stdinTerminal: func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) //nolint:gosec // descripteur de fichier valide
		},
		getenv: os.Getenv,
		now:    time.Now,
		getwd:  os.Getwd,
		executable: func() string {
			p, err := os.Executable()
			if err != nil {
				return ""
			}
			if resolved, err := filepath.EvalSymlinks(p); err == nil {
				return resolved
			}
			return p
		},
		store:   state.Store{Dir: state.DefaultDir(os.Getenv)},
		version: resolveVersion(version, debug.ReadBuildInfo),
	}
}

func main() {
	os.Exit(run(newApp(), os.Args[1:]))
}

// run exécute la commande et renvoie le code de sortie. kubectl transmet au plugin les
// arguments placés après « telemach » ; il refuse toute option placée avant (research.md R2).
func run(a *app, args []string) int {
	root := a.rootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		fmt.Fprintf(a.stderr, "kubectl telemach : %v\n", err)
		return 2
	}
	return 0
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "kubectl-telemach",
		Short:         "Explique en français les erreurs de kubectl et des manifestes Kubernetes",
		Long:          "TELEMACH aide à apprendre Kubernetes : il explique en français les erreurs des commandes kubectl, localise les erreurs des manifestes à la ligne près et diagnostique les pods qui ne démarrent pas.",
		Version:       a.version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if _, err := render.ParseColorMode(a.colorFlag); err != nil {
				return err
			}
			if a.width != 0 && a.width < 40 {
				return fmt.Errorf("--width doit valoir au moins 40 (reçu %d)", a.width)
			}
			return nil
		},
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.PersistentFlags().StringVar(&a.colorFlag, "color", "auto", "couleurs : auto, always ou never")
	root.PersistentFlags().IntVar(&a.width, "width", 0, "largeur de mise en page (40 minimum, largeur du terminal par défaut)")
	root.SetHelpCommand(&cobra.Command{Hidden: true})
	root.CompletionOptions.DisableDefaultCmd = true
	frenchHelp(root)
	root.SetVersionTemplate(versionTemplate)
	root.AddCommand(a.shellInitCommand(), a.versionCommand(), a.explainCommand(), a.lastCommand(), a.checkCommand(), a.deepenCommand(), a.diagnoseCommand(), a.watchCommand(), a.noticeCommand())
	return root
}

// frenchHelp traduit les gabarits d'aide de cobra.
func frenchHelp(root *cobra.Command) {
	root.SetUsageTemplate(`Utilisation :{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [commande]{{end}}{{if .HasAvailableSubCommands}}

Commandes :{{range .Commands}}{{if .IsAvailableCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Options :
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Options globales :
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`)
	root.PersistentFlags().BoolP("help", "h", false, "affiche l'aide")
	root.Flags().BoolP("version", "v", false, "affiche la version, la licence et le site du projet")
}

// output prépare le thème et l'écrivain coloré pour w.
func (a *app) output(w io.Writer) (render.Theme, io.Writer, int) {
	colorOn := render.ResolveColor(a.colorFlag, a.getenv, render.IsTerminal(w))
	dark := true
	if colorOn {
		dark = render.HasDarkBackground(w, a.getenv)
	}
	width := a.width
	if width == 0 {
		width = render.TerminalWidth(w)
	}
	return render.NewTheme(colorOn, dark), render.NewWriter(w, colorOn), width
}
