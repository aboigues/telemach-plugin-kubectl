package diagnose

import (
	"regexp"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
)

// Origine de l'erreur (FR-004, research.md R5). kubectl renvoie le code 1 pour presque toutes
// ses erreurs : l'origine se déduit donc surtout du texte et de l'entrée reconnue.
func init() {
	originRules = append(originRules, declaredOrigin, applicationOrigin, clusterOrigin)
}

// declaredOrigin : origine déclarée par l'entrée du catalogue (champ origin, feature 004),
// pour une erreur du cluster que son texte ne trahit pas (éviction refusée par un
// PodDisruptionBudget, API absente du cluster).
func declaredOrigin(_ Input, d Diagnosis) (Origin, bool) {
	if d.Entry != nil && d.Entry.Origin != "" {
		return Origin(d.Entry.Origin), true
	}
	return "", false
}

// clusterMarkers signale une réponse du serveur API.
var clusterMarkers = regexp.MustCompile(`(?i)error from server|is forbidden: User|already exists|Operation cannot be fulfilled|\(Unauthorized\)|namespaces "[^"]+" not found|" is invalid:`)

// applicationOrigin : erreur du programme exécuté dans le conteneur.
func applicationOrigin(in Input, d Diagnosis) (Origin, bool) {
	if d.Entry != nil && d.Entry.Category == "runtime" {
		return OriginApplication, true
	}
	if d.Entry != nil {
		return "", false
	}
	switch in.Command.Verb() {
	case "exec", "run", "debug", "attach":
		// Sans message reconnu, un code différent de 1 (convention de kubectl) vient du
		// programme du conteneur, transmis tel quel par kubectl.
		if in.ExitCode != 1 && !clusterMarkers.MatchString(in.Stderr) {
			return OriginApplication, true
		}
	}
	return "", false
}

// clusterOrigin : réponse du serveur API (introuvable, refusé, conflit…).
func clusterOrigin(in Input, d Diagnosis) (Origin, bool) {
	if d.Entry == nil {
		return "", false
	}
	if clusterMarkers.MatchString(in.Stderr) {
		return OriginCluster, true
	}
	switch catalog.Category(d.Entry.Category) {
	case "auth", "namespace", "conflict", "workload", "admission":
		return OriginCluster, true
	case "notfound":
		if d.Entry.ID == "notfound.resource" {
			return OriginCluster, true
		}

	}
	return "", false
}
