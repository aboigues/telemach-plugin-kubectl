package main

// checkPreflight prépare « check » avant de lancer le plugin (research R6). Implémentation :
// US4 ; d'ici là, les arguments sont transmis tels quels.
func (a *app) checkPreflight(args []string) (rewritten []string, code int, handled bool) {
	return args, 0, false
}
