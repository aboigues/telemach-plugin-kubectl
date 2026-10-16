package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// WatchDelay est l'attente avant la vérification différée (FR-030, research.md R11) : le
// temps qu'un ImagePullBackOff, un CrashLoopBackOff ou un CreateContainerConfigError
// apparaisse.
const WatchDelay = 10 * time.Second

// watchCommand est la vérification différée, lancée en arrière-plan par l'intégration shell
// après une création réussie (usage interne, FR-028 à FR-031). Elle n'écrit jamais sur le
// terminal : ses signaux sont affichés par le crochet d'invite.
func (a *app) watchCommand() *cobra.Command {
	var shellPID int
	var delay time.Duration
	cmd := &cobra.Command{
		Use:    "watch --shell-pid <pid> -- <args kubectl…>",
		Short:  "Vérification différée après une création réussie (usage interne)",
		Hidden: true,
		Args:   cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			a.watch(shellPID, delay, args)
			return nil
		},
	}
	cmd.Flags().IntVar(&shellPID, "shell-pid", 0, "processus du shell à prévenir")
	cmd.Flags().DurationVar(&delay, "delay", WatchDelay, "attente avant la vérification")
	_ = cmd.Flags().MarkHidden("delay")
	return cmd
}

// watch lit le seul statut des pods ciblés (FR-029) et dépose un signal par pod en échec.
// Toute erreur (cluster injoignable, lecture refusée) est silencieuse.
func (a *app) watch(shellPID int, delay time.Duration, args []string) {
	cmd := command.Parse(cleanArgs(args))
	targets := workload.Targets(cmd)
	if len(targets) == 0 || !state.Alive(shellPID) {
		return
	}
	time.Sleep(delay)
	dir := state.PendingDir(a.getenv)
	now := a.now()
	for _, t := range targets {
		scope := kuberead.Scope{Context: cmd.Context, Kubeconfig: cmd.Kubeconfig, Namespace: t.Namespace}
		r := kuberead.New(a.getenv("TELEMACH_KUBECTL"), scope)
		var pods []kuberead.Pod
		if t.Pod != "" {
			if p, err := r.GetPod(t.Pod); err == nil {
				pods = append(pods, p)
			}
		} else if l, err := r.PodsBySelector(t.Selector); err == nil {
			pods = l.Items
		}
		for _, p := range pods {
			c := workload.Classify(p, now)
			// « NotReady » demanderait les événements, que la vérification ne lit pas (FR-029).
			if c.State != workload.StateFailing || c.Reason == workload.ReasonNotReady {
				continue
			}
			line := "⚠ Le pod " + p.Metadata.Name + " est en " + c.Reason + " : kubectl telemach diagnose pod/" + p.Metadata.Name
			if f := scope.Flags(); f != "" {
				line += " " + f
			}
			_ = state.AppendPending(dir, shellPID, line)
		}
	}
}
