package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/pitfall"
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

// WatchReads est le nombre maximal de lectures d'une vérification différée (research.md S9) :
// au-delà, rien n'est signalé pour les objets restants.
const WatchReads = 12

// watch vérifie, après une création réussie, les pods et les objets créés (research.md S9) et
// dépose un signal par piège non transitoire. L'information sur un tag mutable est décidée
// sans lecture, d'après les seuls manifestes locaux. Toute erreur (cluster injoignable,
// lecture refusée, budget épuisé) est silencieuse.
func (a *app) watch(shellPID int, delay time.Duration, args []string) {
	cmd := command.Parse(cleanArgs(args))
	targets, objects := workload.Targets(cmd), workload.ObjectTargets(cmd)
	if !state.Alive(shellPID) {
		return
	}
	dir := state.PendingDir(a.getenv)
	a.imageHints(dir, shellPID, workload.Images(cmd))
	if len(targets) == 0 && len(objects) == 0 {
		return
	}
	time.Sleep(delay)
	now := a.now()
	budget := kuberead.NewBudget(WatchReads)
	reader := func(ns string) (*kuberead.Reader, kuberead.Scope) {
		scope := kuberead.Scope{Context: cmd.Context, Kubeconfig: cmd.Kubeconfig, Namespace: ns}
		r := kuberead.New(a.getenv("TELEMACH_KUBECTL"), scope)
		r.Budget = budget
		return r, scope
	}
	for _, t := range targets {
		r, scope := reader(t.Namespace)
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
	cat, _ := catalog.Default()
	for _, o := range objects {
		r, scope := reader(o.Namespace)
		failed := budget.Failed()
		snap, err := pitfall.Diagnose(pitfall.Context{Reader: r, Scope: scope, Catalog: cat, Now: now}, pitfall.Kind(o.Kind), o.Name)
		// Rien n'est signalé sur une information incomplète (lecture échouée ou hors budget).
		if err != nil || budget.Failed() > failed || snap.Partial() {
			continue
		}
		for _, p := range snap.Pitfalls {
			if p.Severity != pitfall.Warning || p.Transient || (p.ID == pitfall.ServiceNoMatchingPods && o.CoveredBySameApply) {
				continue
			}
			if text := watchLine(p, snap.LessonCaptures(p)); text != "" {
				line := "⚠ " + text + " : kubectl telemach diagnose " + o.Kind + "/" + o.Name
				if f := scope.Flags(); f != "" {
					line += " " + f
				}
				_ = state.AppendPending(dir, shellPID, line)
			}
		}
	}
}

// watchLine décrit en une phrase un piège signalé par la vérification différée ; vide pour un
// piège qu'elle ne signale pas (research.md S9 : détections non transitoires seulement).
func watchLine(p pitfall.Pitfall, caps map[string]string) string {
	switch p.ID {
	case pitfall.ServiceNoMatchingPods:
		return "Le Service " + caps["service"] + " ne désigne aucun pod"
	case pitfall.ServiceTargetPortName:
		return "Le Service " + caps["service"] + " vise le port nommé « " + caps["targetPort"] + " », qu'aucun pod ne déclare"
	case pitfall.IngressNoController, pitfall.IngressUnknownClass, pitfall.IngressNoClass:
		return "L'Ingress " + caps["ingress"] + " n'a aucune classe prise en charge"
	case pitfall.PVCUnknownClass:
		return "Le PVC " + caps["pvc"] + " demande la classe « " + caps["class"] + " », qui n'existe pas"
	}
	return ""
}

// imageHints dépose, sans aucune lecture du cluster, une information par image au tag mutable,
// au plus une fois par image et par 24 h (FR-023, hints.json).
func (a *app) imageHints(dir string, shellPID int, images []string) {
	now := a.now()
	for _, img := range images {
		ref := pitfall.ParseImage(img)
		if !ref.Mutable() || !a.store.ShouldHint(img, now) {
			continue
		}
		line := "ℹ L'image " + img + " n'a pas de tag explicite : un nouvel apply ne la mettra pas à jour (kubectl telemach deepen)"
		if ref.Tag == "latest" {
			line = "ℹ L'image " + img + " utilise le tag latest : un nouvel apply ne la mettra pas à jour (kubectl telemach deepen)"
		}
		if state.AppendPending(dir, shellPID, line) == nil {
			_ = a.store.RememberHint(img, now)
		}
	}
}
