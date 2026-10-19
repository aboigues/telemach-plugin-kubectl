package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	hintsFile = "hints.json"
	// HintWindow est la durée pendant laquelle une même information (tag mutable d'une image)
	// n'est pas réaffichée (FR-023).
	HintWindow = 24 * time.Hour
)

// HintMemory mémorise les informations déjà affichées (data-model.md § HintMemory) : seule
// l'empreinte SHA-256 de la référence d'image est conservée, jamais la commande ni le fichier.
type HintMemory struct {
	SchemaVersion int
	Seen          map[string]time.Time
}

// ShouldHint indique si l'information sur cette image peut être affichée (jamais vue, ou vue
// il y a plus de HintWindow).
func (s Store) ShouldHint(image string, now time.Time) bool {
	seen, ok := s.loadHints().Seen[hintKey(image)]
	return !ok || now.Sub(seen) >= HintWindow
}

// RememberHint enregistre l'affichage de l'information et purge les entrées périmées.
func (s Store) RememberHint(image string, now time.Time) error {
	m := s.loadHints()
	for k, t := range m.Seen {
		if now.Sub(t) >= HintWindow {
			delete(m.Seen, k)
		}
	}
	m.Seen[hintKey(image)] = now
	m.SchemaVersion = SchemaVersion
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return WritePrivate(s.Dir, hintsFile, data)
}

// loadHints lit la mémoire ; un fichier absent, illisible ou d'un autre schéma vaut une
// mémoire vide.
func (s Store) loadHints() HintMemory {
	m := HintMemory{Seen: map[string]time.Time{}}
	data, err := os.ReadFile(filepath.Join(s.Dir, hintsFile))
	if err != nil {
		return m
	}
	var read HintMemory
	if json.Unmarshal(data, &read) != nil || read.SchemaVersion != SchemaVersion || read.Seen == nil {
		return m
	}
	return read
}

func hintKey(image string) string {
	sum := sha256.Sum256([]byte(image))
	return hex.EncodeToString(sum[:])
}
