package state

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Signaux de la vérification différée (FR-028 à FR-031, data-model.md § PendingSignal) : une
// ligne par pod en échec, dans $XDG_RUNTIME_DIR/telemach-kubectl/pending-<pid du shell>, lue
// puis supprimée par le crochet d'invite.

const pendingPrefix = "pending-"

// PendingDir renvoie le dossier des signaux : $XDG_RUNTIME_DIR/telemach-kubectl, sinon un
// dossier propre à l'utilisateur dans le dossier temporaire.
func PendingDir(env func(string) string) string {
	if d := env("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, dirName)
	}
	return filepath.Join(os.TempDir(), dirName+"-"+strconv.Itoa(os.Getuid()))
}

// PendingFile renvoie le fichier de signaux d'un shell.
func PendingFile(dir string, shellPID int) string {
	return filepath.Join(dir, pendingPrefix+strconv.Itoa(shellPID))
}

// Alive indique si le processus pid existe encore.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// AppendPending ajoute une ligne au fichier de signaux du shell, seulement si ce shell est
// toujours vivant (terminal fermé : rien n'est écrit). Dossier 0700, fichier 0600 ; une
// seule écriture par ligne, en mode ajout.
func AppendPending(dir string, shellPID int, line string) error {
	if !Alive(shellPID) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // un répertoire privé a besoin du droit x
		return err
	}
	f, err := os.OpenFile(PendingFile(dir, shellPID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // chemin construit
	if err != nil {
		return err
	}
	_, werr := f.WriteString(strings.TrimRight(line, "\n") + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

// CleanStalePending supprime les fichiers de signaux des shells disparus.
func CleanStalePending(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(strings.TrimPrefix(e.Name(), pendingPrefix))
		if !strings.HasPrefix(e.Name(), pendingPrefix) || err != nil {
			continue
		}
		if !Alive(pid) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
