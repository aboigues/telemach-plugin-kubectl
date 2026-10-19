//go:build dist

package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// La compilation est reproductible (FR-005) : reconstruire le binaire linux/amd64 avec les
// options de .goreleaser.yaml donne la même somme que checksums.txt.
func TestReproducibleBuild(t *testing.T) {
	dir := distDir(t)
	files, version := releaseFiles(t, dir)
	name := "kubectl-telemach_" + version + "_linux_amd64"
	sums := checksums(t, files["checksums.txt"])
	want, ok := sums[name]
	if !ok {
		t.Fatalf("%s absent de checksums.txt", name)
	}

	out := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-trimpath",
		"-ldflags", "-s -w -buildid= -X main.version="+version,
		"-o", out, "./cmd/kubectl-telemach")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compilation : %v\n%s", err, b)
	}
	if got := sha256File(t, out); got != want {
		t.Errorf("somme reconstruite %s, publiée %s : la compilation n'est pas reproductible", got, want)
	}
}
