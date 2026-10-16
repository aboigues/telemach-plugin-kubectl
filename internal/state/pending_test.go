package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPending(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "telemach-kubectl")
	pid := os.Getpid()
	if err := AppendPending(dir, pid, "⚠ ligne 1"); err != nil {
		t.Fatal(err)
	}
	if err := AppendPending(dir, pid, "⚠ ligne 2\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(PendingFile(dir, pid))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "⚠ ligne 1\n⚠ ligne 2\n" {
		t.Errorf("contenu : %q", data)
	}
	info, _ := os.Stat(PendingFile(dir, pid))
	dinfo, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o600 || dinfo.Mode().Perm() != 0o700 {
		t.Errorf("droits %o / %o, attendu 600 / 700", info.Mode().Perm(), dinfo.Mode().Perm())
	}
	// Shell disparu : rien n'est écrit, et son ancien fichier est supprimé au chargement suivant.
	const dead = 999999
	if err := AppendPending(dir, dead, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PendingFile(dir, dead)); !os.IsNotExist(err) {
		t.Error("aucun signal ne doit être écrit pour un shell disparu")
	}
	if err := os.WriteFile(PendingFile(dir, dead), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	CleanStalePending(dir)
	if _, err := os.Stat(PendingFile(dir, dead)); !os.IsNotExist(err) {
		t.Error("le fichier d'un shell disparu doit être supprimé")
	}
	if _, err := os.Stat(PendingFile(dir, pid)); err != nil {
		t.Error("le fichier d'un shell vivant doit être conservé")
	}
}

func TestPendingDir(t *testing.T) {
	if got := PendingDir(func(k string) string { return map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}[k] }); got != "/run/user/1000/telemach-kubectl" {
		t.Errorf("PendingDir = %q", got)
	}
}
