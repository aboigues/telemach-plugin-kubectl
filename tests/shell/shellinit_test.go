package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/shellinit"
)

// Faux kubectl : écrit sur stdout et stderr, journalise ses appels et renvoie $FAKE_RC. Il
// répond à « version --client -o json » comme le vrai.
const fakeKubectl = `#!/bin/sh
echo "kubectl $*" >> "$CALL_LOG"
if [ "$1" = "version" ]; then
  printf '{\n  "clientVersion": {\n    "major": "1",\n    "minor": "37",\n    "gitVersion": "v1.37.1"\n  }\n}\n'
  exit 0
fi
if [ -n "$FAKE_SLEEP" ]; then
  echo "en cours" >&2
  sleep "$FAKE_SLEEP"
fi
printf 'STDOUT-LINE\n'
printf 'error: the server doesn'"'"'t have a resource type "pdos"\n' >&2
exit "${FAKE_RC:-0}"
`

// Faux enveloppeur (« microk8s ») : journalise ses arguments puis appelle le faux kubectl.
const fakeWrapper = `#!/bin/sh
echo "fakewrap $*" >> "$CALL_LOG"
shift
exec kubectl "$@"
`

// Faux kubectl-telemach : journalise ses arguments, les droits et le contenu du fichier
// stderr, puis échoue (son code de sortie doit être ignoré).
const fakeTelemach = `#!/bin/sh
echo "telemach $*" >> "$CALL_LOG"
prev=""
for a in "$@"; do
  if [ "$prev" = "--stderr-file" ]; then
    echo "perm=$(stat -c %a "$a")" >> "$CALL_LOG"
    echo "content=$(cat "$a")" >> "$CALL_LOG"
  fi
  prev="$a"
done
exit 3
`

type env struct {
	bin, runtime, log string
}

func setup(t *testing.T, withTelemach bool) env {
	t.Helper()
	dir := t.TempDir()
	e := env{bin: filepath.Join(dir, "bin"), runtime: filepath.Join(dir, "run"), log: filepath.Join(dir, "calls.log")}
	for _, d := range []string{e.bin, e.runtime} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(e.bin, name), []byte(content), 0o700); err != nil { //nolint:gosec // script de test
			t.Fatal(err)
		}
	}
	write("kubectl", fakeKubectl)
	write("fakewrap", fakeWrapper)
	if withTelemach {
		write("kubectl-telemach", fakeTelemach)
	}
	return e
}

// run évalue le script d'intégration puis body, dans un shell non interactif.
func run(t *testing.T, shell string, e env, extraEnv []string, body string) (stdout, stderr string, err error) {
	t.Helper()
	return runShell(t, shell, nil, e, extraEnv, "", body)
}

// runWithAliases définit des alias avant d'évaluer le script, dans une session interactive
// (seule une session interactive développe les alias), sans fichiers de démarrage.
func runWithAliases(t *testing.T, shell string, e env, extraEnv []string, aliases, body string) (stdout, stderr string, err error) {
	t.Helper()
	flags := []string{"--norc", "-i"}
	if shell == "zsh" {
		flags = []string{"-f", "-i"}
	}
	return runShell(t, shell, flags, e, extraEnv, aliases, body)
}

func runShell(t *testing.T, shell string, flags []string, e env, extraEnv []string, before, body string) (stdout, stderr string, err error) {
	t.Helper()
	if _, lookErr := exec.LookPath(shell); lookErr != nil {
		t.Skipf("%s non installé", shell)
	}
	script, scriptErr := shellinit.Script(shell, "", "9.9.9", "/tmp/telemach-pending-test")
	if scriptErr != nil {
		t.Fatal(scriptErr)
	}
	scriptFile := filepath.Join(t.TempDir(), "init.sh")
	if err := os.WriteFile(scriptFile, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	// Le scénario est écrit dans un fichier, lu commande par commande comme un fichier rc :
	// les alias définis sur une ligne valent pour les suivantes (avec « -c », zsh analyse tout
	// le texte d'un coup, avant de définir le moindre alias).
	full := before + "\n" + `eval "$(cat "$INIT")"` + "\n" + body + "\n"
	bodyFile := filepath.Join(t.TempDir(), "body.sh")
	if err := os.WriteFile(bodyFile, []byte(full), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shell, append(flags, bodyFile)...) //nolint:gosec // test
	cmd.Env = append([]string{
		"PATH=" + e.bin + ":/usr/bin:/bin",
		"CALL_LOG=" + e.log,
		"XDG_RUNTIME_DIR=" + e.runtime,
		"INIT=" + scriptFile,
		"HOME=" + t.TempDir(),
	}, extraEnv...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	return out.String(), errb.String(), err
}

func readLog(t *testing.T, e env) string {
	t.Helper()
	data, err := os.ReadFile(e.log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func leftovers(t *testing.T, e env) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(e.runtime, "telemach-kubectl.*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func forEachShell(t *testing.T, f func(t *testing.T, shell string)) {
	for _, sh := range []string{"bash", "zsh"} {
		t.Run(sh, func(t *testing.T) { f(t, sh) })
	}
}

// Garanties 1, 2 et 3 : code de sortie, stdout intact, fichier temporaire 0600 supprimé.
func TestFailureIsExplained(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		stdout, stderr, err := run(t, shell, e, []string{"FAKE_RC=1"}, `kubectl get pdos -n demo; echo "rc=$?"`)
		if err != nil {
			t.Fatalf("erreur du shell : %v\n%s", err, stderr)
		}
		if stdout != "STDOUT-LINE\nrc=1\n" {
			t.Errorf("stdout modifié : %q", stdout)
		}
		if strings.Count(stderr, `resource type "pdos"`) != 1 {
			t.Errorf("le message kubectl d'origine doit rester affiché une seule fois (D-LL-002) : %q", stderr)
		}
		if strings.Contains(stderr, "STDOUT-LINE") {
			t.Errorf("stdout ne doit jamais être recopié sur stderr : %q", stderr)
		}
		log := readLog(t, e)
		for _, want := range []string{
			"telemach explain --exit-code 1 --stderr-file ",
			"--kubectl-version v1.37.1 --invoked-as kubectl -- get pdos -n demo",
			"perm=600",
			`content=error: the server doesn't have a resource type "pdos"`,
		} {
			if !strings.Contains(log, want) {
				t.Errorf("journal sans %q :\n%s", want, log)
			}
		}
		if files := leftovers(t, e); len(files) != 0 {
			t.Errorf("fichiers temporaires non supprimés : %v", files)
		}
	})
}

func TestSuccessIsNotExplained(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		stdout, _, err := run(t, shell, e, nil, `kubectl get ns; echo "rc=$?"`)
		if err != nil {
			t.Fatal(err)
		}
		if stdout != "STDOUT-LINE\nrc=0\n" {
			t.Errorf("stdout : %q", stdout)
		}
		if strings.Contains(readLog(t, e), "telemach") {
			t.Error("explain ne doit pas être appelé pour une commande réussie")
		}
	})
}

// La forme correcte « kubectl telemach … » n'est jamais expliquée par TELEMACH lui-même ; la
// forme refusée par kubectl (option avant le nom du plugin) l'est (D-LL-014).
func TestTelemachSelf(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		if _, _, err := run(t, shell, e, []string{"FAKE_RC=1"}, `kubectl telemach diagnose; kubectl -n demo telemach diagnose; true`); err != nil {
			t.Fatal(err)
		}
		log := readLog(t, e)
		if strings.Count(log, "telemach explain") != 1 || !strings.Contains(log, "-- -n demo telemach diagnose") {
			t.Errorf("seule la forme avec option avant le nom du plugin doit être expliquée :\n%s", log)
		}
	})
}

// Garantie 3 (interruption) : le fichier temporaire est supprimé après SIGINT (D-LL-003).
func TestInterruptCleansUp(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		if _, err := exec.LookPath(shell); err != nil {
			t.Skipf("%s non installé", shell)
		}
		script, _ := shellinit.Script(shell, "", "9.9.9", "/tmp/telemach-pending-test")
		initFile := filepath.Join(t.TempDir(), "init.sh")
		if err := os.WriteFile(initFile, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(shell, "-c", `eval "$(cat "$INIT")"`+"\nkubectl get pods") //nolint:gosec // test
		cmd.Env = []string{"PATH=" + e.bin + ":/usr/bin:/bin", "CALL_LOG=" + e.log, "XDG_RUNTIME_DIR=" + e.runtime, "INIT=" + initFile, "FAKE_SLEEP=5"}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		running := func() bool {
			for _, f := range leftovers(t, e) {
				if data, _ := os.ReadFile(f); len(data) > 0 {
					return true
				}
			}
			return false
		}
		for !running() && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if !running() {
			t.Fatal("la commande kubectl aurait dû être en cours")
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		_ = cmd.Wait()
		time.Sleep(100 * time.Millisecond)
		if files := leftovers(t, e); len(files) != 0 {
			t.Errorf("fichiers temporaires non supprimés après SIGINT : %v", files)
		}
	})
}

// Garantie 5 : TELEMACH_DISABLE=1 ou binaire absent → comportement identique à kubectl.
func TestDisabledOrMissing(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		for _, tc := range []struct {
			name     string
			telemach bool
			env      []string
		}{
			{"TELEMACH_DISABLE=1", true, []string{"FAKE_RC=1", "TELEMACH_DISABLE=1"}},
			{"binaire absent", false, []string{"FAKE_RC=2"}},
		} {
			e := setup(t, tc.telemach)
			stdout, _, err := run(t, shell, e, tc.env, `kubectl get pods; echo "rc=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(stdout, "STDOUT-LINE\nrc=") || strings.Contains(readLog(t, e), "telemach explain") {
				t.Errorf("%s : stdout %q, journal :\n%s", tc.name, stdout, readLog(t, e))
			}
			if files := leftovers(t, e); len(files) != 0 {
				t.Errorf("%s : aucun fichier temporaire attendu : %v", tc.name, files)
			}
		}
	})
}

// Garantie 6 : commande interactive ou longue → stderr non capturé, --interactive transmis.
// exec, attach, debug et port-forward font exception : leur stderr est capté (feature 002,
// research.md S10).
func TestInteractive(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		for _, body := range []string{
			`kubectl exec -it web -- sh`, `kubectl exec -ti web -- sh`, `kubectl -n demo exec -i -t web -- sh`,
			`kubectl attach web`, `kubectl port-forward svc/web 8080:80`, `kubectl debug -it web --image=busybox:1.38`,
		} {
			e := setup(t, true)
			stdout, _, err := run(t, shell, e, []string{"FAKE_RC=127"}, body+`; echo "rc=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(stdout, "rc=127\n") {
				t.Errorf("%s : code de sortie non préservé : %q", body, stdout)
			}
			if log := readLog(t, e); strings.Contains(log, "--interactive") || !strings.Contains(log, "--stderr-file") {
				t.Errorf("%s : explain doit recevoir le stderr capté :\n%s", body, log)
			}
		}
		for _, body := range []string{
			`kubectl run -it debug --image=busybox:1.37 --rm -- sh`, `kubectl edit deploy web`, `kubectl proxy`,
			`kubectl logs -f web`, `kubectl get pods -w`,
		} {
			e := setup(t, true)
			stdout, _, err := run(t, shell, e, []string{"FAKE_RC=127"}, body+`; echo "rc=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(stdout, "rc=127\n") {
				t.Errorf("%s : code de sortie non préservé : %q", body, stdout)
			}
			log := readLog(t, e)
			if !strings.Contains(log, "--interactive") || strings.Contains(log, "--stderr-file") {
				t.Errorf("%s : explain doit recevoir --interactive sans --stderr-file :\n%s", body, log)
			}
		}
		// exec sans -i ni -t : stderr capturé normalement.
		e := setup(t, true)
		if _, _, err := run(t, shell, e, []string{"FAKE_RC=1"}, `kubectl exec web -- ls; true`); err != nil {
			t.Fatal(err)
		}
		if log := readLog(t, e); !strings.Contains(log, "--stderr-file") {
			t.Errorf("exec sans -it doit être capturé :\n%s", log)
		}
	})
}

// Garantie 7 : version lue une seule fois, à l'évaluation ; aucune autre commande lancée.
func TestVersionReadOnce(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		if _, _, err := run(t, shell, e, []string{"FAKE_RC=1"}, `kubectl get a; kubectl get b; kubectl get ns; true`); err != nil {
			t.Fatal(err)
		}
		log := readLog(t, e)
		if n := strings.Count(log, "kubectl version --client -o json"); n != 1 {
			t.Errorf("version lue %d fois, attendu 1 :\n%s", n, log)
		}
		for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
			if strings.HasPrefix(line, "kubectl ") && !strings.HasPrefix(line, "kubectl version") &&
				line != "kubectl get a" && line != "kubectl get b" && line != "kubectl get ns" {
				t.Errorf("appel kubectl inattendu : %q", line)
			}
		}
	})
}

// Variables exportées : commande réelle, version, shell et version du plugin.
func TestExportedVariables(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		stdout, _, err := run(t, shell, e, nil, `printf '%s|%s|%s\n' "$TELEMACH_KUBECTL" "$TELEMACH_KUBECTL_VERSION" "$TELEMACH_KUBECTL_SHELL"`)
		if err != nil {
			t.Fatal(err)
		}
		if want := "kubectl|v1.37.1|" + shell + ":9.9.9\n"; stdout != want {
			t.Errorf("variables : %q, attendu %q", stdout, want)
		}
	})
}

// R4 : « kubectl » est un alias (microk8s kubectl) → la commande de l'alias est exécutée et
// l'erreur expliquée ; « k » alias de kubectl est couvert avec --invoked-as k.
func TestKubectlAlias(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		stdout, stderr, err := runWithAliases(t, shell, e, []string{"FAKE_RC=1"},
			`alias kubectl='fakewrap kubectl'`+"\n"+`alias k=kubectl`,
			`kubectl get pdos; echo "rc=$?"`+"\n"+`k get pdos; echo "rc=$?"`+"\n"+`echo "TK=$TELEMACH_KUBECTL"`)
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if strings.Count(stdout, "rc=1") != 2 || !strings.Contains(stdout, "TK=fakewrap kubectl") {
			t.Errorf("stdout : %q", stdout)
		}
		log := readLog(t, e)
		if n := strings.Count(log, "fakewrap kubectl get pdos"); n != 2 {
			t.Errorf("la commande de l'alias doit être exécutée 2 fois, %d trouvée(s) :\n%s", n, log)
		}
		if !strings.Contains(log, "fakewrap kubectl version --client -o json") {
			t.Errorf("la version doit être lue par la commande de l'alias :\n%s", log)
		}
		if !strings.Contains(log, "--invoked-as kubectl -- get pdos") || !strings.Contains(log, "--invoked-as k -- get pdos") {
			t.Errorf("--invoked-as attendu pour kubectl et pour k :\n%s", log)
		}
	})
}

// FR-001a : « k » absent n'est jamais créé ; « k » qui désigne autre chose n'est pas modifié ;
// un rechargement du script ne casse rien.
func TestAliasKNotCreatedNorModified(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setup(t, true)
		stdout, _, err := runWithAliases(t, shell, e, nil, `unalias k 2>/dev/null; true`,
			`alias k >/dev/null 2>&1 && echo "k=créé" || echo "k=absent"`)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(stdout) != "k=absent" {
			t.Errorf("k ne doit pas être créé : %q", stdout)
		}
		e = setup(t, true)
		stdout, _, err = runWithAliases(t, shell, e, nil, `alias k='ls -la'`, `alias k`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, "ls -la") {
			t.Errorf("k désignant autre chose ne doit pas être modifié : %q", stdout)
		}
		e = setup(t, true)
		stdout, _, err = runWithAliases(t, shell, e, []string{"FAKE_RC=1"}, `alias kubectl='fakewrap kubectl'`,
			`eval "$(cat "$INIT")"`+"\n"+`kubectl get pdos; echo "rc=$?"`)
		if err != nil {
			t.Fatal(err)
		}
		log := readLog(t, e)
		if !strings.HasSuffix(stdout, "rc=1\n") || strings.Count(log, "fakewrap kubectl get pdos") != 1 || strings.Count(log, "telemach explain") != 1 {
			t.Errorf("rechargement : stdout %q, journal :\n%s", stdout, log)
		}
	})
}

func TestUnsupportedShell(t *testing.T) {
	if _, err := shellinit.Script("fish", "", "9.9.9", "/tmp/telemach-pending-test"); err == nil || !strings.Contains(err.Error(), "non pris en charge") {
		t.Errorf("erreur en français attendue pour fish, obtenu %v", err)
	}
}
