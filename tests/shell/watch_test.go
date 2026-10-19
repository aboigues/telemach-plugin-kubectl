package shell

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Faux kubectl-telemach pour la vérification différée : « watch » journalise ses arguments
// et la destination de ses entrées et sorties, puis dort (le shell ne doit jamais l'attendre).
const fakeTelemachWatch = `#!/bin/sh
echo "telemach $*" >> "$CALL_LOG"
if [ "$1" = "watch" ]; then
  for fd in 0 1 2; do echo "watch-fd$fd=$(readlink /proc/$$/fd/$fd)" >> "$CALL_LOG"; done
  sleep "${FAKE_WATCH_SLEEP:-0}"
  echo "watch-fin" >> "$CALL_LOG"
fi
exit 0
`

func setupWatch(t *testing.T) env {
	t.Helper()
	e := setup(t, false)
	if err := os.WriteFile(filepath.Join(e.bin, "kubectl-telemach"), []byte(fakeTelemachWatch), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	return e
}

// waitWatchers attend la fin des vérifications lancées en arrière-plan, avant la suppression
// du dossier temporaire du test.
func waitWatchers(t *testing.T, e env) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			log := readLog(t, e)
			if strings.Count(log, "watch-fin") >= strings.Count(log, "telemach watch") {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

func interactiveFlags(shell string) []string {
	if shell == "zsh" {
		return []string{"-f", "-i"}
	}
	return []string{"--norc", "-i"}
}

// Garantie 9 : après une création réussie dans un shell interactif, watch est lancé en
// arrière-plan, entrées et sorties sur /dev/null, sans message de tâche et sans attente.
func TestWatchLaunched(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setupWatch(t)
		waitWatchers(t, e)
		start := time.Now()
		stdout, stderr, err := runShell(t, shell, interactiveFlags(shell), e, []string{"FAKE_WATCH_SLEEP=3"}, "",
			`kubectl apply -f deploy.yaml -n demo; echo "rc=$?"`)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		if !strings.Contains(stdout, "rc=0") {
			t.Errorf("stdout : %q", stdout)
		}
		if elapsed > 2500*time.Millisecond {
			t.Errorf("le shell a attendu la vérification (%v)", elapsed)
		}
		for _, noise := range []string{"[1]", "Done", "done", "terminé"} {
			if strings.Contains(stderr, noise) {
				t.Errorf("message de tâche de fond %q :\n%s", noise, stderr)
			}
		}
		time.Sleep(200 * time.Millisecond)
		log := readLog(t, e)
		if !strings.Contains(log, "telemach watch --shell-pid ") || !strings.Contains(log, "-- apply -f deploy.yaml -n demo") {
			t.Fatalf("watch non lancé :\n%s", log)
		}
		for _, fd := range []string{"watch-fd0=/dev/null", "watch-fd1=/dev/null", "watch-fd2=/dev/null"} {
			if !strings.Contains(log, fd) {
				t.Errorf("%s attendu :\n%s", fd, log)
			}
		}
	})
}

// T049 : « expose » crée un Service et lance la vérification, comme apply, create et run
// (feature 002, research.md S9).
func TestWatchLaunchedByExpose(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setupWatch(t)
		waitWatchers(t, e)
		if _, stderr, err := runShell(t, shell, interactiveFlags(shell), e, nil, "",
			`kubectl expose deployment hello --name=hello-typo --port=80 -n demo; true`); err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		time.Sleep(200 * time.Millisecond)
		if log := readLog(t, e); !strings.Contains(log, "-- expose deployment hello --name=hello-typo --port=80 -n demo") {
			t.Fatalf("watch non lancé après expose :\n%s", log)
		}
	})
}

// Désactivation (FR-031), shell non interactif, commande non concernée : aucun appel.
func TestWatchNotLaunched(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		cases := []struct {
			name  string
			flags []string
			env   []string
			body  string
		}{
			{"TELEMACH_WATCH=0", interactiveFlags(shell), []string{"TELEMACH_WATCH=0"}, "kubectl apply -f x.yaml"},
			{"TELEMACH_DISABLE=1", interactiveFlags(shell), []string{"TELEMACH_DISABLE=1"}, "kubectl apply -f x.yaml"},
			{"expose, TELEMACH_WATCH=0", interactiveFlags(shell), []string{"TELEMACH_WATCH=0"}, "kubectl expose deployment hello --port=80"},
			{"expose, TELEMACH_DISABLE=1", interactiveFlags(shell), []string{"TELEMACH_DISABLE=1"}, "kubectl expose deployment hello --port=80"},
			{"non interactif", nil, nil, "kubectl apply -f x.yaml"},
			{"get", interactiveFlags(shell), nil, "kubectl get pods"},
			{"échec", interactiveFlags(shell), []string{"FAKE_RC=1"}, "kubectl apply -f x.yaml"},
		}
		for _, c := range cases {
			e := setupWatch(t)
			if _, _, err := runShell(t, shell, c.flags, e, c.env, "", c.body+"; true"); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(readLog(t, e), "telemach watch") {
				t.Errorf("%s : watch ne doit pas être lancé", c.name)
			}
		}
	})
}

// Garantie 10 : le crochet d'invite affiche puis supprime le fichier de signaux ; il est
// installé une seule fois même si le script est rechargé.
func TestPromptHook(t *testing.T) {
	forEachShell(t, func(t *testing.T, shell string) {
		e := setupWatch(t)
		pending := "/tmp/telemach-pending-test"
		hooks := `printf '%s\n' "$PROMPT_COMMAND"`
		if shell == "zsh" {
			hooks = `print -r -- "${precmd_functions[*]}"`
		}
		body := `eval "$(cat "$INIT")"` + "\n" + // rechargement : pas de doublon
			`mkdir -p ` + pending + ` && printf '⚠ Le pod web est en ImagePullBackOff : kubectl telemach diagnose pod/web -n demo\n' > ` + pending + `/pending-$$` + "\n" +
			`__telemach_prompt` + "\n" +
			`[ -e ` + pending + `/pending-$$ ] && echo "fichier=présent" || echo "fichier=supprimé"` + "\n" + hooks
		stdout, stderr, err := runShell(t, shell, interactiveFlags(shell), e, nil, "", body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, "⚠ Le pod web est en ImagePullBackOff") {
			t.Errorf("signal non affiché :\n%s", stderr)
		}
		if !strings.Contains(stdout, "fichier=supprimé") {
			t.Errorf("le fichier de signaux doit être supprimé : %q", stdout)
		}
		if n := strings.Count(stdout, "__telemach_prompt"); n != 1 {
			t.Errorf("crochet installé %d fois (attendu 1) : %q", n, stdout)
		}
	})
}

// SC-010 : l'intégration retarde l'invite de moins de 50 ms au p95 après une création réussie
// (watch dort 3 s en arrière-plan sans être attendu), et le crochet sans signal est immédiat.
// Mesure faite sans le détecteur de courses (LL-006).
func TestWatchLatency(t *testing.T) {
	if raceEnabled {
		t.Skip("latence mesurée sans le détecteur de courses (étape dédiée de la CI, LL-006)")
	}
	forEachShell(t, func(t *testing.T, shell string) {
		e := setupWatch(t)
		waitWatchers(t, e)
		now := `${EPOCHREALTIME/,/.}`
		body := ""
		if shell == "zsh" {
			body = "zmodload zsh/datetime\n"
		}
		body += `for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do` + "\n" +
			`  t0=` + now + `; command kubectl apply -f x.yaml >/dev/null 2>&1; t1=` + now + "\n" +
			`  kubectl apply -f x.yaml >/dev/null 2>&1; t2=` + now + "\n" +
			`  __telemach_prompt; t3=` + now + "\n" +
			`  echo "$t0 $t1 $t2 $t3"` + "\n" +
			`done`
		stdout, stderr, err := runShell(t, shell, interactiveFlags(shell), e, []string{"FAKE_WATCH_SLEEP=1"}, "", body)
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		var added, hook []float64
		for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
			f := strings.Fields(line)
			if len(f) != 4 {
				continue
			}
			v := make([]float64, 4)
			for i, s := range f {
				v[i], _ = strconv.ParseFloat(s, 64)
			}
			added = append(added, (v[2]-v[1])-(v[1]-v[0]))
			hook = append(hook, v[3]-v[2])
		}
		if len(added) < 20 {
			t.Fatalf("mesures incomplètes (D-LL-009) : %q", stdout)
		}
		if p := p95(added); p > 0.050 {
			t.Errorf("retard ajouté par l'intégration : p95 = %.1f ms > 50 ms", p*1000)
		} else {
			t.Logf("retard ajouté : p95 = %.1f ms ; crochet : p95 = %.2f ms", p*1000, p95(hook)*1000)
		}
		if p := p95(hook); p > 0.010 {
			t.Errorf("crochet d'invite sans signal : p95 = %.1f ms > 10 ms", p*1000)
		}
	})
}

func p95(values []float64) float64 {
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	return s[len(s)*95/100]
}
