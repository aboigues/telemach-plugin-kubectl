package shellinit

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed scripts/bash.sh scripts/zsh.sh
var scripts embed.FS

// Shells pris en charge.
var supported = []string{"bash", "zsh"}

// EnvVar est la variable exportée par l'intégration : shell et version du plugin
// (« zsh:0.1.0 »), vérifiée par « last » et « deepen » (D-LL-013).
const EnvVar = "TELEMACH_KUBECTL_SHELL"

// Script renvoie le code d'intégration à évaluer dans le shell donné. binPath est le chemin
// du binaire kubectl-telemach à appeler (vide : cherché dans le PATH) ; version est celle du
// plugin ; pendingDir est le dossier des signaux de la vérification différée.
func Script(shell, binPath, version, pendingDir string) (string, error) {
	for _, s := range supported {
		if s != shell {
			continue
		}
		data, err := scripts.ReadFile("scripts/" + shell + ".sh")
		if err != nil {
			return "", err
		}
		if binPath == "" {
			binPath = "kubectl-telemach"
		}
		out := strings.Replace(string(data), "__TELEMACH_BIN__", shellQuote(binPath), 1)
		out = strings.Replace(out, "__TELEMACH_SHELL__", shellQuote(ShellValue(shell, version)), 1)
		return strings.Replace(out, "__TELEMACH_PENDING__", shellQuote(pendingDir), 1), nil
	}
	return "", fmt.Errorf("shell non pris en charge : %q (shells disponibles : bash, zsh)", shell)
}

// shellQuote protège une chaîne pour bash et zsh : entre apostrophes, chaque apostrophe étant
// remplacée par la séquence apostrophe, barre oblique inverse, apostrophe, apostrophe.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellValue est la valeur de EnvVar pour un shell et une version (« zsh:0.1.0 »).
func ShellValue(shell, version string) string {
	return shell + ":" + version
}
