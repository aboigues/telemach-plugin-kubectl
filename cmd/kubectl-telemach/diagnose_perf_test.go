package main

import (
	"os"
	"sort"
	"strconv"
	"testing"
	"time"
)

// slowCluster est un faux kubectl dont chaque appel prend FAKE_DELAY secondes, comme un aller-
// retour vers un vrai serveur d'API : la durée de diagnose dépend alors du nombre de lectures
// faites l'une après l'autre.
const slowCluster = `#!/bin/sh
echo "$*" >> "$CALL_LOG"
sleep "$FAKE_DELAY"
case "$1 $2" in
"get pods") cat "$FAKE_DIR/list.json" ;;
"get events")
  name=$(echo "$4" | sed 's/.*involvedObject.name=//')
  cat "$FAKE_PODS/$name/events.json" ;;
"logs "*)
  case "$*" in *--previous*) f=logs-previous.txt ;; *) f=logs.txt ;; esac
  cat "$FAKE_PODS/$2/$f" ;;
"get services"|"get endpointslices"|"get deployments"|"get ingresses"|"get ingressclasses"|"get pvc"|"get storageclasses")
  echo '{"items":[]}' ;;
*) echo "appel inattendu : $*" >&2; exit 99 ;;
esac
`

// fakeDelay est la durée simulée d'un appel à kubectl (aller-retour réseau et démarrage du
// processus, plutôt pessimiste pour un cluster local).
const fakeDelay = 400 * time.Millisecond

// SC-004 : diagnose sans argument répond en moins de 5 s au p95, lectures du namespace
// comprises (D-LL-012). Les lectures des autres objets sont parallèles (research.md S8) : en
// série, elles dépasseraient l'objectif. Mesure faite sans le détecteur de courses (LL-006).
func TestDiagnoseNamespaceLatency(t *testing.T) {
	if raceEnabled {
		t.Skip("latence mesurée sans le détecteur de courses (étape dédiée de la CI)")
	}
	env, log, at := fakeEnv(t, "crash", "healthy")
	bin := env["TELEMACH_KUBECTL"]
	if err := os.WriteFile(bin, []byte(slowCluster), 0o700); err != nil { //nolint:gosec // script de test
		t.Fatal(err)
	}
	t.Setenv("FAKE_DELAY", strconv.FormatFloat(fakeDelay.Seconds(), 'f', 3, 64))
	var durations []time.Duration
	for range 10 {
		_ = os.Remove(log)
		a, _, errb := newTestApp(t, env)
		a.now = func() time.Time { return at }
		start := time.Now()
		if code := run(a, []string{"diagnose", "--color=never", "-n", "demo"}); code != 0 {
			t.Fatalf("code %d\n%s", code, errb)
		}
		durations = append(durations, time.Since(start))
	}
	calls := len(readCalls(t, log))
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[len(durations)*95/100-1]
	serial := time.Duration(calls) * fakeDelay
	t.Logf("diagnose sans argument : p95 = %v pour %d lectures (%v en série)", p95, calls, serial)
	if p95 > 5*time.Second {
		t.Errorf("SC-004 non atteint : p95 = %v > 5 s", p95)
	}
	if p95 >= serial {
		t.Errorf("les lectures ne sont pas parallèles : %v pour %d lectures de %v", p95, calls, fakeDelay)
	}
}
