package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// T013 : mémoire des informations déjà affichées (tag mutable, FR-023).
func TestHintMemory(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "state")}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	image := "nginxinc/nginx-unprivileged"
	if !s.ShouldHint(image, now) {
		t.Fatal("première fois : l'information doit être affichée")
	}
	if err := s.RememberHint(image, now); err != nil {
		t.Fatal(err)
	}
	if s.ShouldHint(image, now.Add(23*time.Hour)) {
		t.Error("moins de 24 h : pas de nouvel affichage")
	}
	if !s.ShouldHint(image, now.Add(25*time.Hour)) {
		t.Error("plus de 24 h : nouvel affichage attendu")
	}
	if !s.ShouldHint("busybox", now) {
		t.Error("autre image : affichage attendu")
	}
	path := filepath.Join(s.Dir, "hints.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("hints.json en 0600 attendu : %v %v", info, err)
	}
	data, _ := os.ReadFile(path) //nolint:gosec // fichier de test
	if strings.Contains(string(data), "nginx") {
		t.Errorf("seule l'empreinte de l'image est conservée : %s", data)
	}
	// Purge des entrées de plus de 24 h à l'écriture suivante.
	if err := s.RememberHint("busybox", now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if m := s.loadHints(); len(m.Seen) != 1 {
		t.Errorf("entrée périmée non purgée : %+v", m.Seen)
	}
	// Fichier illisible : ignoré sans erreur.
	if err := os.WriteFile(path, []byte("{pas du json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !s.ShouldHint(image, now) {
		t.Error("fichier illisible : l'information doit être affichée")
	}
}
