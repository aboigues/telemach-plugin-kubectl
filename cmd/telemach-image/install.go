package main

import (
	"fmt"
	"os"
	"syscall"
)

// Codes de sortie de « install » (contracts/image-cli.md).
const (
	exitRefused = 1 // refus lié à l'environnement : rien n'a été écrit
	exitUsage   = 2 // erreur d'utilisation
)

// install copie le plugin dans le dossier monté (research R7). Implémentation : US3.
func (a *app) install(args []string) int {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			theme, w := a.output(a.stdout)
			fmt.Fprint(w, installHelpText(theme, a.imageTag()))
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(a.stderr, "telemach : argument inconnu pour install : %s\n", args[0])
		return exitUsage
	}
	fmt.Fprintln(a.stderr, "telemach : install n'est pas encore disponible dans cette image")
	return exitUsage
}

// fileOwner renvoie l'identifiant du propriétaire d'un fichier.
func fileOwner(path string) (int, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("propriétaire de %s illisible", path)
	}
	return int(st.Uid), nil
}
