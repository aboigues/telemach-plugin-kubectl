//go:build integration

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

// Feature 004 (research.md F12) : cas qui demandent une préparation propre, sur un cluster à
// trois nœuds (testdata/kind/three-nodes.yaml). Chaque cas a son namespace, du nom de son
// identifiant, supprimé à la fin ; les nœuds modifiés (label, cordon) sont remis en état même
// si le cas échoue.
//
// Codes de sortie relus dans le code de kubectl avant d'écrire les attentes (LL-017) :
//   - drain : 1 dès qu'un pod ne peut pas être supprimé ou évincé (cmdutil.CheckErr) ;
//   - wait : 1 à l'expiration du délai (« timed out waiting for the condition ») ;
//   - top : 1 quand l'API de métriques est absente ;
//   - debug : 1 quand les conteneurs éphémères sont refusés par le serveur.
//
// Dans Setup, Args et Cleanup : {NS} est le namespace du cas, {SC} le dossier des scénarios,
// {W1} le premier worker, {IMG} l'image web du relevé.
type stagedCase struct {
	ID      string
	Setup   [][]string
	Args    []string
	Success bool // relevé de la sortie d'erreur d'une commande RÉUSSIE (avertissement)
	Cleanup [][]string
}

// badImage est une image au tag inexistant : le pod reste en attente (wait expiré).
const badImage = "nginxinc/nginx-unprivileged:0.0.0"

var stagedCases = []stagedCase{
	// Refus d'admission (US1).
	{
		ID:    "run-quota-exceeded",
		Setup: [][]string{words("apply -f {SC}/quota.yaml -n {NS}")},
		Args: []string{
			"run", "gros", "--image={IMG}", "-n", "{NS}", "--overrides",
			`{"spec":{"containers":[{"name":"gros","image":"{IMG}","resources":{"limits":{"cpu":"2","memory":"64Mi"}}}]}}`,
		},
	},
	{
		ID:    "run-quota-must-specify",
		Setup: [][]string{words("apply -f {SC}/quota.yaml -n {NS}")},
		Args:  words("run nu --image={IMG} -n {NS}"),
	},
	{
		ID:    "apply-limitrange",
		Setup: [][]string{words("apply -f {SC}/limitrange.yaml -n {NS}")},
		Args:  words("apply -f {SC}/limitrange-pod.yaml -n {NS}"),
	},
	{
		ID:    "apply-pod-security",
		Setup: [][]string{words("label namespace {NS} pod-security.kubernetes.io/enforce=restricted")},
		Args:  words("apply -f {SC}/pss-violation.yaml -n {NS}"),
	},
	{
		ID:      "apply-pod-security-warn",
		Setup:   [][]string{words("label namespace {NS} pod-security.kubernetes.io/warn=restricted")},
		Args:    words("apply -f {SC}/pss-violation.yaml -n {NS}"),
		Success: true,
	},
	// Maintenance des nœuds (US2) : l'ordre des refus est celui de kubectl (DaemonSet, puis
	// stockage local, puis éviction).
	drainCase("drain-daemonset-pods", "drain {W1}"),
	drainCase("drain-local-storage", "drain {W1} --ignore-daemonsets"),
	drainCase("drain-pdb-blocked", "drain {W1} --ignore-daemonsets --delete-emptydir-data --timeout=20s"),
	// Métriques (US4) : aucun metrics-server dans ce cluster.
	{ID: "top-metrics-unavailable", Args: words("top pods -n {NS}")},
	// wait, rollout undo (US6).
	{
		ID:    "wait-condition-timeout",
		Setup: [][]string{words("run web --image=" + badImage + " -n {NS}")},
		Args:  words("wait --for=condition=Ready pod/web --timeout=10s -n {NS}"),
	},
	{
		ID:    "wait-deployment-timeout",
		Setup: [][]string{words("create deployment api --image=" + badImage + " -n {NS}")},
		Args:  words("wait --for=condition=Available deployment/api --timeout=10s -n {NS}"),
	},
	{
		ID:    "wait-jsonpath-timeout",
		Setup: [][]string{words("run web --image=" + badImage + " -n {NS}")},
		Args:  words("wait --for=jsonpath={.status.phase}=Running pod/web --timeout=10s -n {NS}"),
	},
	{
		ID:    "wait-delete-timeout",
		Setup: [][]string{words("create configmap reste --from-literal=mode=demo -n {NS}")},
		Args:  words("wait --for=delete configmap/reste --timeout=5s -n {NS}"),
	},
	{
		ID:    "rollout-undo-revision-missing",
		Setup: [][]string{words("create deployment api --image={IMG} -n {NS}")},
		Args:  words("rollout undo deployment/api --to-revision=5 -n {NS}"),
	},
}

// drainCase prépare le nœud {W1} : label formation=drain, Deployment protégé par un PDB et
// Deployment à volume emptyDir (drain-pdb.yaml), puis lance drain.
func drainCase(id, args string) stagedCase {
	return stagedCase{
		ID: id,
		Setup: [][]string{
			words("label node {W1} formation=drain --overwrite"),
			words("apply -f {SC}/drain-pdb.yaml -n {NS}"),
			words("wait --for=condition=Available deployment/web deployment/cache --timeout=120s -n {NS}"),
		},
		Args: words(args),
		Cleanup: [][]string{
			words("uncordon {W1}"),
			words("label node {W1} formation-"),
		},
	}
}

// onlyStaged (option -only f004) relève tous les cas préparés, et eux seuls.
const onlyStaged = "f004"

// isStaged indique si id est un cas préparé.
func isStaged(id string) bool {
	for _, tc := range stagedCases {
		if tc.ID == id {
			return true
		}
	}
	return false
}

// collectStaged relève les cas préparés de la feature 004.
func collectStaged(out, variant, only, scenarios string, c *kindcluster.Cluster) error {
	// Chemin relatif au dossier du dépôt : kubectl le recopie dans ses messages, un chemin
	// absolu ferait entrer le dossier personnel du mainteneur dans le corpus.
	sc := filepath.ToSlash(filepath.Clean(scenarios))
	for _, tc := range stagedCases {
		if only != "" && only != onlyStaged && tc.ID != only {
			continue
		}
		repl := strings.NewReplacer("{NS}", tc.ID, "{SC}", sc, "{W1}", c.Worker(1), "{IMG}", webImage)
		expand := func(args []string) []string {
			out := make([]string, len(args))
			for i, a := range args {
				out[i] = repl.Replace(a)
			}
			return out
		}
		if err := c.Must("create", "namespace", tc.ID); err != nil {
			return err
		}
		var err error
		failed := false
		for _, step := range tc.Setup {
			if err := c.Must(expand(step)...); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ %s : préparation en échec : %v\n", tc.ID, err)
				failed = true
				break
			}
		}
		if !failed {
			time.Sleep(2 * time.Second)
			args := expand(tc.Args)
			r := c.Kubectl(args...)
			switch {
			case tc.Success && r.ExitCode != 0:
				fmt.Fprintf(os.Stderr, "⚠ %s : la commande devait réussir (code %d)\n%s", tc.ID, r.ExitCode, r.Stderr)
			case tc.Success:
				err = save(out, variant, tc.ID, c, args, r)
			default:
				err = saveFailure(out, variant, tc.ID, c, args, r)
			}
		}
		for _, step := range tc.Cleanup {
			_ = c.Kubectl(expand(step)...)
		}
		// Suppression attendue : les pods restants fausseraient le relevé de drain suivant.
		_ = c.Kubectl("delete", "namespace", tc.ID, "--timeout=120s")
		if err != nil {
			return err
		}
	}
	return nil
}
