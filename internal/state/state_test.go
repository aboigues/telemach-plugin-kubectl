package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

func newStore(t *testing.T) Store {
	t.Helper()
	return Store{Dir: filepath.Join(t.TempDir(), dirName)}
}

func TestDirFromEnv(t *testing.T) {
	env := map[string]string{"XDG_STATE_HOME": "/xdg/state", "HOME": "/home/x"}
	if got := DefaultDir(func(k string) string { return env[k] }); got != "/xdg/state/telemach-kubectl" {
		t.Errorf("DefaultDir avec XDG_STATE_HOME = %q", got)
	}
	delete(env, "XDG_STATE_HOME")
	if got := DefaultDir(func(k string) string { return env[k] }); got != "/home/x/.local/state/telemach-kubectl" {
		t.Errorf("DefaultDir sans XDG_STATE_HOME = %q", got)
	}
}

// Sans HOME ni XDG_STATE_HOME absolus (image Docker, feature 003), aucun dossier d'état
// n'est retenu, et un Store sans dossier ne lit ni n'écrit rien : jamais de chemin relatif au
// dossier courant.
func TestNoStateDir(t *testing.T) {
	for _, env := range []map[string]string{{}, {"HOME": ""}, {"HOME": "relatif"}, {"XDG_STATE_HOME": "relatif"}} {
		if got := DefaultDir(func(k string) string { return env[k] }); got != "" {
			t.Errorf("DefaultDir(%v) = %q, attendu vide", env, got)
		}
	}
	cwd := t.TempDir()
	t.Chdir(cwd)
	s := Store{}
	if err := s.Save(LastDiagnosis{Fingerprint: "x"}); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("Save sans dossier : %v", err)
	}
	if err := s.RememberHint("nginx", time.Now()); !errors.Is(err, ErrNoStateDir) {
		t.Errorf("RememberHint sans dossier : %v", err)
	}
	if _, ok := s.Load(); ok {
		t.Error("Load sans dossier ne doit rien lire")
	}
	if !s.ShouldHint("nginx", time.Now()) {
		t.Error("ShouldHint sans dossier : information affichée (aucune mémoire)")
	}
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Errorf("rien ne doit être écrit dans le dossier courant : %v", entries)
	}
}

func TestSaveLoadPermissions(t *testing.T) {
	s := newStore(t)
	now := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	in := LastDiagnosis{Fingerprint: "abc", EntryID: "cmdline.unknown-resource-type", Captures: map[string]string{"type": "pdos"}, RecordedAt: now, Source: SourceCommand}
	if err := s.Save(in); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("droits du répertoire = %o, attendu 700", perm)
	}
	finfo, err := os.Stat(filepath.Join(s.Dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if perm := finfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("droits du fichier = %o, attendu 600", perm)
	}
	out, ok := s.Load()
	if !ok {
		t.Fatal("Load : diagnostic attendu")
	}
	if out.SchemaVersion != 1 || out.EntryID != in.EntryID || out.Captures["type"] != "pdos" || !out.RecordedAt.Equal(now) || out.Source != SourceCommand {
		t.Errorf("Load = %+v", out)
	}
}

func TestLoadCorruptedOrUnknownSchema(t *testing.T) {
	s := newStore(t)
	if _, ok := s.Load(); ok {
		t.Error("aucun diagnostic attendu sans fichier")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir, fileName)
	for _, content := range []string{"{pas du json", `{"SchemaVersion": 99, "Fingerprint": "x"}`} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Load(); ok {
			t.Errorf("contenu %q : doit être ignoré", content)
		}
		if err := s.Save(LastDiagnosis{Fingerprint: "y", RecordedAt: time.Now()}); err != nil {
			t.Errorf("le fichier doit pouvoir être réécrit : %v", err)
		}
	}
}

func TestIsRepeat(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	if s.IsRepeat("fp", now) {
		t.Error("pas de répétition sans diagnostic précédent")
	}
	if err := s.Save(LastDiagnosis{Fingerprint: "fp", RecordedAt: now}); err != nil {
		t.Fatal(err)
	}
	if !s.IsRepeat("fp", now.Add(9*time.Minute)) {
		t.Error("répétition attendue à 9 minutes")
	}
	if s.IsRepeat("fp", now.Add(11*time.Minute)) {
		t.Error("pas de répétition au-delà de 10 minutes")
	}
	if s.IsRepeat("autre", now.Add(time.Minute)) {
		t.Error("pas de répétition pour une autre empreinte")
	}
}

// Les appelants masquent avant d'enregistrer : ce test vérifie la chaîne complète sur un
// secret du corpus (SC-006).
func TestNoSecretOnDisk(t *testing.T) {
	s := newStore(t)
	raw, err := os.ReadFile("../../testdata/secrets/flag-token-eq.txt")
	if err != nil {
		t.Fatal(err)
	}
	masked := redact.Redact(string(raw))
	args := redact.RedactArgs(strings.Fields(string(raw))[1:])
	if err := s.Save(LastDiagnosis{Stderr: masked, Args: args, RecordedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fake-token-123") {
		t.Errorf("secret écrit en clair : %s", data)
	}
}

func TestStderrTruncated(t *testing.T) {
	s := newStore(t)
	long := strings.Repeat("a", MaxStderr) + "FIN"
	if err := s.Save(LastDiagnosis{Stderr: long}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.Load()
	if len(d.Stderr) != MaxStderr || !strings.HasSuffix(d.Stderr, "FIN") {
		t.Errorf("le message doit être tronqué en gardant la fin (%d octets)", len(d.Stderr))
	}
}
