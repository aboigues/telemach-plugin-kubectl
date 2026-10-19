package main

import (
	"debug/buildinfo"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
)

// version est renseignée à la compilation (-ldflags "-X main.version=…").
var version = "dev"

const (
	// imageRoot contient les binaires du plugin embarqués dans l'image, par système.
	imageRoot = "/opt/telemach"
	// destDir est le point de montage du dossier de destination de l'hôte (dans le PATH).
	destDir = "/sortie"
	// manifestDir est le dossier des manifestes monté pour l'essai ; c'est aussi le WORKDIR
	// de l'image.
	manifestDir = "/manifestes"
	// pluginName est le nom de fichier sous lequel kubectl trouve le plugin dans le PATH.
	pluginName = "kubectl-telemach"
)

// app regroupe les dépendances injectables (pour les tests).
type app struct {
	stdout, stderr io.Writer
	getenv         func(string) string
	environ        func() []string
	exec           func(path string, argv, env []string) error
	mountinfo      func() ([]byte, error)
	getuid         func() int
	owner          func(path string) (int, error)
	buildInfo      func(path string) (*buildinfo.BuildInfo, error)
	goarch         string
	version        string
	root, dest     string
	manifest       string
}

func newApp() *app {
	return &app{
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		getenv:    os.Getenv,
		environ:   os.Environ,
		exec:      syscall.Exec,
		mountinfo: func() ([]byte, error) { return os.ReadFile("/proc/self/mountinfo") },
		getuid:    os.Getuid,
		owner:     fileOwner,
		buildInfo: buildinfo.ReadFile,
		goarch:    runtime.GOARCH,
		version:   version,
		root:      imageRoot,
		dest:      destDir,
		manifest:  manifestDir,
	}
}

func main() {
	os.Exit(run(newApp(), os.Args[1:]))
}

// run aiguille la commande et renvoie le code de sortie.
func run(a *app, args []string) int {
	if len(args) == 0 {
		return a.help()
	}
	switch args[0] {
	case "help", "--help", "-h":
		return a.help()
	case "install":
		return a.install(args[1:])
	case "check":
		rewritten, code, handled := a.checkPreflight(args[1:])
		if handled {
			return code
		}
		args = append([]string{"check"}, rewritten...)
	}
	return a.passThrough(args)
}

// passThrough remplace le processus par le plugin linux, avec les mêmes arguments : sortie
// et code de sortie sont ceux du plugin installé (FR-016).
func (a *app) passThrough(args []string) int {
	argv := append([]string{pluginName}, args...)
	if err := a.exec(linuxPlugin(a.root), argv, a.environ()); err != nil {
		fmt.Fprintf(a.stderr, "telemach : impossible de lancer le plugin : %v\n", err)
		return exitUsage
	}
	return 0
}

// linuxPlugin renvoie le chemin du plugin linux embarqué.
func linuxPlugin(root string) string {
	return filepath.Join(root, "linux", pluginName)
}

// output prépare le thème et l'écrivain coloré pour w, selon les règles du plugin.
func (a *app) output(w io.Writer) (render.Theme, io.Writer) {
	colorOn := render.ResolveColor("auto", a.getenv, render.IsTerminal(w))
	dark := true
	if colorOn {
		dark = render.HasDarkBackground(w, a.getenv)
	}
	return render.NewTheme(colorOn, dark), render.NewWriter(w, colorOn)
}

// imageTag renvoie le tag de l'image à afficher dans les commandes : la version exacte, ou
// un modèle générique pour une construction de développement.
func (a *app) imageTag() string {
	if a.version == "" || a.version == "dev" {
		return "X.Y.Z"
	}
	return a.version
}

func (a *app) help() int {
	theme, w := a.output(a.stdout)
	fmt.Fprint(w, helpText(theme, a.imageTag()))
	return 0
}
