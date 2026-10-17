package schema

// Supported liste les versions mineures de Kubernetes dont les règles sont embarquées (les
// trois dernières maintenues au 2026-10-16 : research.md R7, FR-014a), de la plus ancienne à
// la plus récente.
var Supported = []string{"1.35", "1.36", "1.37"}

// Latest renvoie la version de règles la plus récente embarquée.
func Latest() string { return Supported[len(Supported)-1] }
