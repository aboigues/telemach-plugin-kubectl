package manifests

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// LL-007 : aucun exécutable compilé n'est versionné (un « go build » à la racine suivi de
// « git add -A » l'a fait une fois). Les binaires sont reconnus à leur en-tête : ELF, Mach-O,
// PE.
func TestNoBinaryCommitted(t *testing.T) {
	out, err := exec.Command("git", "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git indisponible : %v", err)
	}
	magics := [][]byte{
		{0x7f, 'E', 'L', 'F'},    // ELF (Linux)
		{0xcf, 0xfa, 0xed, 0xfe}, // Mach-O 64 bits (macOS)
		{'M', 'Z'},               // PE (Windows)
	}
	files := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	if len(files) < 10 {
		t.Fatalf("liste des fichiers versionnés suspecte (D-LL-009) : %d fichier(s)", len(files))
	}
	for _, f := range files {
		fh, err := os.Open("../../" + f) //nolint:gosec // fichiers du dépôt
		if err != nil {
			continue // supprimé du dossier de travail mais pas encore de l'index
		}
		head := make([]byte, 4)
		n, _ := fh.Read(head)
		_ = fh.Close()
		for _, m := range magics {
			if n >= len(m) && bytes.Equal(head[:len(m)], m) {
				t.Errorf("%s : exécutable compilé versionné", f)
			}
		}
	}
}
