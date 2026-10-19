package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/aboigues/telemach-plugin-kubectl/internal/shellinit"
)

// ptySession est un shell lancé dans un pseudo-terminal, comme dans un vrai terminal : stdin,
// stdout et stderr sont le terminal, le shell en est le processus de contrôle (job control,
// Ctrl-C envoyé par le pilote du terminal au groupe au premier plan).
type ptySession struct {
	t      *testing.T
	master *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    bytes.Buffer
	done   chan struct{}
}

// openPty ouvre un pseudo-terminal (maître, esclave).
func openPty(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	m, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("pseudo-terminal indisponible : %v", err)
	}
	if err := unix.IoctlSetPointerInt(m, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(m, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(m), "/dev/ptmx"), s
}

// startPty lance un shell interactif sur le scénario body (intégration évaluée d'abord).
func startPty(t *testing.T, shell string, e env, extraEnv []string, body string) *ptySession {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s non installé", shell)
	}
	script := writeInit(t, shell)
	bodyFile := t.TempDir() + "/body.sh"
	if err := os.WriteFile(bodyFile, []byte(`eval "$(cat "$INIT")"`+"\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	master, slave := openPty(t)
	cmd := exec.Command(shell, append(interactiveFlags(shell), bodyFile)...) //nolint:gosec // test
	cmd.Env = append([]string{
		"PATH=" + e.bin + ":/usr/bin:/bin", "CALL_LOG=" + e.log, "XDG_RUNTIME_DIR=" + e.runtime,
		"INIT=" + script, "HOME=" + t.TempDir(), "TERM=xterm", "PS1=", "PROMPT=",
	}, extraEnv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = slave.Close()
	p := &ptySession{t: t, master: master, cmd: cmd, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			p.mu.Lock()
			p.out.Write(buf[:n])
			p.mu.Unlock()
			if err != nil {
				close(p.done)
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = master.Close()
	})
	return p
}

// output renvoie ce que le terminal a affiché jusqu'ici.
func (p *ptySession) output() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.String()
}

// waitFor attend qu'un texte apparaisse sur le terminal.
func (p *ptySession) waitFor(text string) {
	p.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(p.output(), text) {
		if time.Now().After(deadline) {
			p.t.Fatalf("%q n'est pas apparu sur le terminal :\n%s", text, p.output())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// send tape du texte au clavier (« \x03 » : Ctrl-C).
func (p *ptySession) send(text string) {
	p.t.Helper()
	if _, err := p.master.Write([]byte(text)); err != nil {
		p.t.Fatal(err)
	}
}

// wait attend la fin du shell et renvoie tout ce que le terminal a affiché.
func (p *ptySession) wait() string {
	p.t.Helper()
	exited := make(chan struct{})
	go func() { _ = p.cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		p.t.Fatalf("le shell ne s'est pas terminé :\n%s", p.output())
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
	}
	return strings.ReplaceAll(p.output(), "\r\n", "\n")
}

// writeInit écrit le script d'intégration du shell dans un fichier temporaire.
func writeInit(t *testing.T, shell string) string {
	t.Helper()
	script, err := shellinit.Script(shell, "", "9.9.9", "/tmp/telemach-pending-test")
	if err != nil {
		t.Fatal(err)
	}
	f := t.TempDir() + "/init.sh"
	if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}
