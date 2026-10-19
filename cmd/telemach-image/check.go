package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// checkPreflight prépare « check » avant de lancer le plugin (research R6). Avec -f, les
// arguments sont transmis tels quels. Sans -f, le dossier monté est vérifié, sous-dossiers
// compris (-f <dossier> -R) ; s'il ne contient aucun manifeste, l'image l'explique elle-même
// (montage du dossier) au lieu du plugin, qui ne sait pas qu'il tourne dans un conteneur.
// handled vaut vrai quand la commande a été traitée ici.
func (a *app) checkPreflight(args []string) (rewritten []string, code int, handled bool) {
	for _, arg := range args {
		if arg == "-f" || arg == "--filename" || strings.HasPrefix(arg, "-f=") || strings.HasPrefix(arg, "--filename=") {
			return args, 0, false // fichiers explicites : le plugin les vérifie lui-même
		}
	}
	if !hasManifest(a.manifest) {
		theme, w := a.output(a.stderr)
		fmt.Fprint(w, noManifestText(theme, a.imageTag(), a.manifest))
		return nil, exitUsage, true
	}
	return append(append([]string(nil), args...), "-f", a.manifest, "-R"), 0, false
}

// hasManifest indique si dir contient, à un niveau quelconque, un fichier .yaml ou .yml.
func hasManifest(dir string) bool {
	found := false
	// #nosec G703 -- simple parcours, dans le conteneur, du dossier que l'apprenant a lui-même
	// monté (en lecture seule).
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // dossier illisible : ignoré, comme par le plugin
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}
