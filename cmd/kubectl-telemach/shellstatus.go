package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/shellinit"
)

// shellStatusWarning signale, avant last ou deepen, un terminal où l'intégration shell n'est
// pas active ou a été chargée par une autre version du plugin : les erreurs récentes n'y sont
// alors ni expliquées ni enregistrées, et le diagnostic réaffiché peut être ancien (D-LL-006,
// D-LL-013). Rien n'est écrit quand l'intégration est à jour.
func (a *app) shellStatusWarning(w io.Writer, theme render.Theme, width int) {
	value := a.getenv(shellinit.EnvVar)
	shell, version, _ := strings.Cut(value, ":")
	var text, command string
	switch {
	case value == "":
		sh := filepath.Base(a.getenv("SHELL"))
		if sh != "bash" {
			sh = "zsh"
		}
		text = "L'intégration shell de TELEMACH n'est pas active dans ce terminal : les erreurs " +
			"de kubectl qui s'y produisent ne sont ni expliquées ni enregistrées, et ce qui suit " +
			"peut dater d'avant. Pour l'activer (ou relance simplement ton terminal si cette ligne " +
			"est déjà dans ~/." + sh + "rc) :"
		command = `eval "$(kubectl-telemach shell-init ` + sh + `)"`
	case version != a.version:
		text = fmt.Sprintf("L'intégration shell de ce terminal a été chargée par la version %s de TELEMACH "+
			"(version installée : %s) : relance ton terminal, ou tape « exec %s », pour utiliser la "+
			"version installée.", version, a.version, shell)
	default:
		return
	}
	// Première ligne sans retrait, les suivantes alignées sous le texte ; la commande reste sur
	// sa propre ligne, jamais repliée, pour pouvoir être copiée.
	fmt.Fprintln(w, theme.Title(strings.TrimPrefix(render.Wrap(render.SymbolWarning+" "+text, width, "   "), "   ")))
	if command != "" {
		fmt.Fprintln(w, "   "+theme.Highlight(command, "bash"))
	}
	fmt.Fprintln(w)
}
