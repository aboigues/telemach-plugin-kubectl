package main

import (
	"fmt"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/manifest"
)

// checkPreflight prépare « check » avant de lancer le plugin (research R6). Avec -f, ou pour
// une demande d'aide, les arguments sont transmis tels quels. Sans -f, le dossier monté est
// vérifié, sous-dossiers compris (-f <dossier> -R) ; s'il ne contient aucun manifeste, l'image
// l'explique elle-même (montage du dossier) au lieu du plugin, qui ne sait pas qu'il tourne
// dans un conteneur. handled vaut vrai quand la commande a été traitée ici.
func (a *app) checkPreflight(args []string) (rewritten []string, code int, handled bool) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if usesFilename(arg) || arg == "--help" || hasShortFlag(arg, 'h') {
			return args, 0, false // le plugin traite lui-même les fichiers explicites et l'aide
		}
	}
	if !hasManifest(a.manifest) {
		theme, w := a.output(a.stderr)
		fmt.Fprint(w, noManifestText(theme, a.imageTag(), a.manifest))
		return nil, exitUsage, true
	}
	return append(append([]string(nil), args...), "-f", a.manifest, "-R"), 0, false
}

// usesFilename indique si arg donne l'option -f / --filename, sous toutes les formes
// acceptées par cobra : « -f x », « -f=x », « -fx », « -Rf x », « --filename[=x] ».
func usesFilename(arg string) bool {
	return arg == "--filename" || strings.HasPrefix(arg, "--filename=") || hasShortFlag(arg, 'f')
}

// hasShortFlag indique si arg, groupe d'options courtes (« -Rf »), contient l'option name.
// Seules -R et -h, sans valeur, peuvent précéder une autre option dans un groupe de check.
func hasShortFlag(arg string, name byte) bool {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	for i := 1; i < len(arg); i++ {
		switch arg[i] {
		case name:
			return true
		case 'R', 'h':
		default:
			return false
		}
	}
	return false
}

// hasManifest indique si dir contient, à un niveau quelconque, un fichier que le plugin
// vérifierait (mêmes extensions que kubectl : .yaml, .yml, .json).
func hasManifest(dir string) bool {
	files, _ := manifest.Expand([]string{dir}, true)
	return len(files) > 0
}
