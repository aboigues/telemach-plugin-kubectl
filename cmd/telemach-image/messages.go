package main

import (
	"fmt"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
)

// imageRef renvoie la référence complète de l'image, avec un tag explicite (principe III).
func imageRef(tag string) string { return "telemachlearning/telemach-kubectl:" + tag }

// Commandes canoniques (contracts/image-cli.md), à copier telles quelles.
func tryCommand(tag string) string {
	return `docker run --rm -t -v "$PWD":/manifestes:ro ` + imageRef(tag) + " check"
}

func installCommand(tag, hostOS string) string {
	env := ""
	if hostOS == "darwin" {
		env = "-e HOST_OS=darwin "
	}
	return `docker run --rm --user "$(id -u):$(id -g)" ` + env + "-v ~/.local/bin:/sortie " + imageRef(tag) + " install"
}

const mkdirCommand = "mkdir -p ~/.local/bin"

// indent décale chaque ligne de n espaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func helpText(t render.Theme, tag string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", t.Title("TELEMACH — explique en français les erreurs kubectl et ce qui ne marche pas"))
	fmt.Fprintf(&b, "%s\n\n", t.Dim("Image "+imageRef(tag)+" : essayer TELEMACH ou l'installer sur ta machine."))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Essayer sans rien installer (vérifier les manifestes du dossier courant, sans cluster)"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(tryCommand(tag)), 3))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Installer le plugin — Linux et WSL2"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(mkdirCommand+"\n"+installCommand(tag, "linux")), 3))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Installer le plugin — macOS (Docker Desktop)"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(mkdirCommand+"\n"+installCommand(tag, "darwin")), 3))

	fmt.Fprintf(&b, "%s\n", t.Concept(render.LabelConcept+" — montage (bind mount)"))
	fmt.Fprintf(&b, "%s\n", indent("L'option -v DOSSIER_MACHINE:DOSSIER_CONTENEUR donne au conteneur accès à un dossier\nprécis de ta machine ; « :ro » le rend accessible en lecture seule.", 3))
	return b.String()
}

func installHelpText(t render.Theme, tag string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", t.Title("install — copie le plugin TELEMACH dans un dossier du PATH de ta machine"))
	fmt.Fprintf(&b, "%s\n", t.Label("Linux et WSL2"))
	fmt.Fprintf(&b, "%s\n\n", indent(mkdirCommand+"\n"+installCommand(tag, "linux"), 3))
	fmt.Fprintf(&b, "%s\n", t.Label("macOS (Docker Desktop)"))
	fmt.Fprintf(&b, "%s\n\n", indent(mkdirCommand+"\n"+installCommand(tag, "darwin"), 3))
	fmt.Fprintf(&b, "%s\n", t.Label("Options"))
	fmt.Fprintf(&b, "%s\n", indent("-v ~/.local/bin:/sortie       dossier de ta machine où copier le plugin (obligatoire, dans le PATH)\n--user \"$(id -u):$(id -g)\"    le conteneur écrit avec ton identité\n-e HOST_OS=darwin             installe le binaire macOS (linux par défaut)", 3))
	return b.String()
}
