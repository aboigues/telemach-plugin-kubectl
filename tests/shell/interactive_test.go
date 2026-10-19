package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInteractive est un faux kubectl dont la session dépend de FAKE_SCENARIO :
//   - session : vérifie que stdin et stdout sont le terminal, lit une saisie, écrit une
//     erreur sur stderr et renvoie $FAKE_RC ;
//   - sigint : écrit une erreur de transfert sur stderr puis attend Ctrl-C (code 130), comme
//     un port-forward interrompu.
const fakeInteractive = `#!/bin/sh
echo "kubectl $*" >> "$CALL_LOG"
if [ "$1" = "version" ]; then
  printf '{\n  "clientVersion": {\n    "gitVersion": "v1.37.1"\n  }\n}\n'
  exit 0
fi
case "$FAKE_SCENARIO" in
session)
  [ -t 0 ] && [ -t 1 ] && echo TTY-OK
  printf 'saisie: '
  read line
  echo "lu=[$line]"
  echo 'error: container helo is not valid for pod web-7b86fc74c5-gbmc9 out of: hello' >&2
  exit "${FAKE_RC:-0}" ;;
sigint)
  trap 'echo INT-RECU; exit 130' INT
  echo 'E1019 an error occurred forwarding 8080 -> 80: connection refused' >&2
  echo PRET
  while :; do sleep 0.1; done ;;
esac
exit "${FAKE_RC:-0}"
`

// setupInteractive prépare le faux kubectl interactif et le faux kubectl-telemach.
func setupInteractive(t *testing.T) env {
	t.Helper()
	e := setup(t, true)
	if err := os.WriteFile(filepath.Join(e.bin, "kubectl"), []byte(fakeInteractive), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	return e
}

// T055 (research.md S10) : avec stderr capté, une session « exec -it » reste utilisable dans
// un vrai terminal (stdin et stdout restent le terminal, saisie lue, écho unique, D-LL-002),
// son code de sortie est préservé et son stderr est transmis à explain.
func TestInteractiveSessionCaptured(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		for _, body := range []string{
			"kubectl exec -it -n demo deploy/hello -c helo -- sh",
			"kubectl attach -it web",
			"kubectl debug -it web --image=busybox:1.38",
		} {
			e := setupInteractive(t)
			p := startPty(t, shell, e, []string{"FAKE_SCENARIO=session", "FAKE_RC=1"}, body+"\necho \"rc=$?\"\nexit")
			p.waitFor("saisie: ")
			p.send("bonjour\r")
			out := p.wait()
			for _, want := range []string{"TTY-OK", "lu=[bonjour]", "rc=1", "container helo is not valid"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s : %q absent du terminal :\n%s", body, want, out)
				}
			}
			// Le pilote du terminal affiche la saisie une fois, le programme la relit une fois.
			if n := strings.Count(out, "bonjour"); n != 2 {
				t.Errorf("%s : saisie affichée %d fois au lieu de 2 (écho dupliqué ?) :\n%s", body, n, out)
			}
			log := readLog(t, e)
			if !strings.Contains(log, "--stderr-file") || strings.Contains(log, "--interactive") ||
				!strings.Contains(log, "content=error: container helo is not valid") {
				t.Errorf("%s : explain doit recevoir le stderr capté :\n%s", body, log)
			}
			if files := leftovers(t, e); len(files) != 0 {
				t.Errorf("%s : fichiers temporaires restants : %v", body, files)
			}
		}
	})
}

// Ctrl-C est transmis à kubectl ; un port-forward interrompu (code 130) garde ce qu'il a écrit
// sur stderr avant l'interruption, pour qu'explain n'explique qu'une erreur de transfert
// reconnue (research.md S10).
func TestInteractiveInterrupt(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setupInteractive(t)
		p := startPty(t, shell, e, []string{"FAKE_SCENARIO=sigint"}, "kubectl port-forward -n demo service/hello 8080:80\necho \"rc=$?\"\nexit")
		p.waitFor("PRET")
		// La ligne d'erreur affichée prouve que tee tourne (et ignore déjà Ctrl-C) : sinon, sous
		// forte charge, Ctrl-C pourrait arriver avant qu'il soit prêt.
		p.waitFor("an error occurred forwarding")
		p.send("\x03")
		out := p.wait()
		for _, want := range []string{"INT-RECU", "rc=130"} {
			if !strings.Contains(out, want) {
				t.Errorf("%q absent du terminal :\n%s", want, out)
			}
		}
		log := readLog(t, e)
		if !strings.Contains(log, "--exit-code 130") || !strings.Contains(log, "content=E1019 an error occurred forwarding") {
			t.Errorf("explain doit recevoir le code 130 et le stderr capté avant Ctrl-C :\n%s", log)
		}
		if files := leftovers(t, e); len(files) != 0 {
			t.Errorf("fichiers temporaires restants : %v", files)
		}
	})
}
