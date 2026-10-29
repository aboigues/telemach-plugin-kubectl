package pitfall

import (
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
	"github.com/aboigues/telemach-plugin-kubectl/internal/sanitize"
)

// clean nettoie et masque un texte lu du cluster (message de condition ou d'événement) avant
// de le garder dans un instantané (feature 004 : messages des Jobs, HPA, ReplicaSets).
func clean(s string) string { return redact.Redact(sanitize.Clean(s)) }
