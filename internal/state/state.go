package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	// SchemaVersion est la version du format du fichier d'état (à ne pas confondre avec la
	// version des règles Kubernetes, RulesVersion).
	SchemaVersion = 1
	// RepeatWindow est la durée pendant laquelle une même erreur est considérée comme répétée
	// (FR-009b).
	RepeatWindow = 10 * time.Minute
	fileName     = "last.json"
	// dirName distingue l'état de ce plugin de celui de TELEMACH PLUGIN DOCKER.
	dirName = "telemach-kubectl"
)

// MaxStderr est la taille maximale du message d'origine conservé (on garde la fin).
const MaxStderr = 64 << 10

// Source du dernier diagnostic.
const (
	SourceCommand = "command" // erreur d'une commande kubectl captée
	SourceCheck   = "check"   // vérification de manifestes
	SourcePod     = "pod"     // diagnostic d'un pod (diagnose)
	SourceObject  = "object"  // diagnostic d'un Service, Deployment, Ingress ou PVC (feature 002)
)

// LastDiagnosis est le dernier diagnostic (data-model.md). Toutes les données textuelles sont
// déjà nettoyées et masquées avant d'arriver ici : aucune donnée non masquée n'est écrite.
type LastDiagnosis struct {
	SchemaVersion  int
	Fingerprint    string
	EntryID        string
	Captures       map[string]string
	RecordedAt     time.Time
	Source         string
	Origin         string
	Confidence     string
	ExitCode       int
	Interactive    bool
	InvokedAs      string
	KubectlVersion string
	Stderr         string
	Args           []string // arguments kubectl, nettoyés et masqués
	Subcommand     string
	Files          []string // manifestes à réanalyser par « last »
	WorkDir        string   // dossier de travail au moment de l'erreur (chemins relatifs de Files)
	// Pod est le diagnostic d'un pod (workload.PodSnapshot encodé), déjà masqué : « last »
	// le réaffiche sans relire le cluster.
	Pod json.RawMessage `json:",omitempty"`
	// Object est le diagnostic d'un objet (pitfall.ObjectSnapshot encodé), déjà masqué :
	// « last » le réaffiche sans relire le cluster (feature 002).
	Object json.RawMessage `json:",omitempty"`
}

// Store lit et écrit l'état dans un répertoire privé. Sans répertoire (Dir vide), il ne lit
// ni n'écrit rien : c'est le cas sans HOME, par exemple dans l'image Docker (feature 003).
type Store struct {
	Dir string
}

// ErrNoStateDir signale un état désactivé, faute de répertoire.
var ErrNoStateDir = errors.New("aucun répertoire d'état")

// DefaultDir renvoie $XDG_STATE_HOME/telemach-kubectl, ou ~/.local/state/telemach-kubectl ;
// une chaîne vide si aucun des deux n'est un chemin absolu (jamais de chemin relatif au
// dossier courant).
func DefaultDir(env func(string) string) string {
	if d := env("XDG_STATE_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, dirName)
	}
	if h := env("HOME"); filepath.IsAbs(h) {
		return filepath.Join(h, ".local", "state", dirName)
	}
	return ""
}

// Load lit le dernier diagnostic. Un fichier absent, illisible ou d'un schéma inconnu est
// ignoré sans erreur.
func (s Store) Load() (LastDiagnosis, bool) {
	if s.Dir == "" {
		return LastDiagnosis{}, false
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, fileName))
	if err != nil {
		return LastDiagnosis{}, false
	}
	var d LastDiagnosis
	if err := json.Unmarshal(data, &d); err != nil || d.SchemaVersion != SchemaVersion {
		return LastDiagnosis{}, false
	}
	return d, true
}

// Save écrit le diagnostic de façon atomique, avec les droits 0700 pour le répertoire et
// 0600 pour le fichier.
func (s Store) Save(d LastDiagnosis) error {
	if s.Dir == "" {
		return ErrNoStateDir
	}
	d.SchemaVersion = SchemaVersion
	if len(d.Stderr) > MaxStderr {
		d.Stderr = d.Stderr[len(d.Stderr)-MaxStderr:]
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return WritePrivate(s.Dir, fileName, data)
}

// WritePrivate écrit name dans dir de façon atomique (fichier temporaire puis renommage),
// avec les droits 0700 pour le répertoire et 0600 pour le fichier.
func WritePrivate(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // un répertoire privé a besoin du droit x
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // sans effet après le renommage
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// IsRepeat indique si l'empreinte est identique à celle du dernier diagnostic, enregistré il
// y a moins de RepeatWindow.
func (s Store) IsRepeat(fingerprint string, now time.Time) bool {
	d, ok := s.Load()
	if !ok || d.Fingerprint != fingerprint {
		return false
	}
	return now.Sub(d.RecordedAt) < RepeatWindow
}
