package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/diagnose"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
	"github.com/aboigues/telemach-plugin-kubectl/internal/state"
	"github.com/aboigues/telemach-plugin-kubectl/internal/textdist"
	"github.com/aboigues/telemach-plugin-kubectl/internal/workload"
)

// diagnoseCommand diagnostique les pods en difficulté, en lecture seule (FR-022 à FR-027).
func (a *app) diagnoseCommand() *cobra.Command {
	var scope kuberead.Scope
	cmd := &cobra.Command{
		Use:   "diagnose [pod/<nom> | deployment/<nom> | <nom de pod>] [-n <ns>] [--context <ctx>] [--kubeconfig <chemin>]",
		Short: "Diagnostique, en lecture seule, les pods qui ne démarrent pas",
		Long: `Sans argument, repère les pods en difficulté du namespace, explique le plus récent et liste
les autres. Le plugin lit seulement le statut des pods, leurs événements et la fin du journal
d'un conteneur en échec, avec ta propre commande kubectl et tes droits ; il ne modifie rien.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			theme, w, width := a.output(a.stderr)
			opts := render.Options{Theme: theme, Width: width}
			reader := kuberead.New(a.getenv("TELEMACH_KUBECTL"), scope)
			cat, _ := catalog.Default()
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			return a.diagnose(w, reader, cat, target, opts)
		},
	}
	cmd.Flags().StringVarP(&scope.Namespace, "namespace", "n", "", "namespace (par défaut, celui du contexte)")
	cmd.Flags().StringVar(&scope.Context, "context", "", "contexte kubeconfig à utiliser")
	cmd.Flags().StringVar(&scope.Kubeconfig, "kubeconfig", "", "fichier kubeconfig à utiliser")
	cmd.Flags().StringVar(&scope.As, "as", "", "identité à emprunter (vérifier ce qu'un autre utilisateur peut lire)")
	return cmd
}

func (a *app) diagnose(w io.Writer, r *kuberead.Reader, cat *catalog.Catalog, target string, opts render.Options) error {
	kind, name, _ := strings.Cut(target, "/")
	if name == "" {
		kind, name = "pod", kind
	}
	var candidates []workload.Candidate
	label := ""
	switch {
	case target == "":
		pods, err := r.ListPods()
		if err != nil {
			return a.readFailure(w, err, []string{"get", "pods"}, r.Scope, cat, opts)
		}
		candidates = workload.SelectFailing(pods.Items, a.now())
		label = "dans le namespace " + nsLabel(r.Scope)
	case kind == "pod" || kind == "po" || kind == "pods":
		pod, err := r.GetPod(name)
		if kuberead.IsNotFound(err) {
			return a.notFound(w, r, "pod", name, opts)
		}
		if err != nil {
			return a.readFailure(w, err, []string{"get", "pod", name}, r.Scope, cat, opts)
		}
		snap, d := workload.Diagnose(r, pod, r.Scope, cat, a.now())
		return a.showPod(w, snap, d, opts)
	case kind == "deployment" || kind == "deploy" || kind == "deployments":
		dep, err := r.GetDeployment(name)
		if kuberead.IsNotFound(err) {
			fmt.Fprintln(w, opts.Theme.Title(render.SymbolError+" Le deployment « "+name+" » est introuvable dans le namespace "+nsLabel(r.Scope)+"."))
			return exitError{code: 2}
		}
		if err != nil {
			return a.readFailure(w, err, []string{"get", "deployment", name}, r.Scope, cat, opts)
		}
		pods, err := r.PodsBySelector(selector(dep.Spec.Selector.MatchLabels))
		if err != nil {
			return a.readFailure(w, err, []string{"get", "pods"}, r.Scope, cat, opts)
		}
		candidates = workload.SelectFailing(pods.Items, a.now())
		label = "pour deployment/" + name
	default:
		fmt.Fprintln(w, opts.Theme.Title(render.SymbolError+" « "+kind+" » n'est pas pris en charge par diagnose : utilise pod/NOM ou deployment/NOM."))
		return exitError{code: 2}
	}
	// Le plus récent est expliqué ; un pod qui s'avère simplement en démarrage (aucune sonde en
	// échec dans ses événements) est écarté au profit du suivant.
	for i, c := range candidates {
		snap, d := workload.Diagnose(r, c.Pod, r.Scope, cat, a.now())
		if snap.State != workload.StateFailing {
			continue
		}
		for _, other := range candidates[i+1:] {
			if len(snap.Others) == workload.MaxOthers {
				break
			}
			snap.Others = append(snap.Others, workload.PodSummary{Name: other.Pod.Metadata.Name, Reason: other.Class.Reason})
		}
		return a.showPod(w, snap, d, opts)
	}
	fmt.Fprintln(w, opts.Theme.Success(render.SymbolSuccess+" Aucun pod en difficulté "+label+"."))
	fmt.Fprintln(w, render.Wrap("Pour un autre namespace : « kubectl telemach diagnose -n NAMESPACE ». "+
		"Un pod encore en cours de création n'est pas signalé.", opts.Width, "   "))
	return nil
}

// showPod affiche le diagnostic et l'enregistre comme dernier diagnostic (FR-027).
func (a *app) showPod(w io.Writer, snap workload.PodSnapshot, d diagnose.Diagnosis, opts render.Options) error {
	if err := render.RenderPod(w, snap, d, opts); err != nil {
		return err
	}
	if snap.State == workload.StateFailing {
		last := state.LastDiagnosis{
			Fingerprint: snap.Namespace + "/" + snap.Name + "/" + snap.Reason, Captures: d.Captures,
			RecordedAt: a.now(), Source: state.SourcePod, Origin: string(d.Origin), Confidence: string(d.Confidence),
			Args: []string{"telemach", "diagnose", "pod/" + snap.Name},
		}
		if d.Entry != nil {
			last.EntryID = d.Entry.ID
		}
		if data, err := json.Marshal(snap); err == nil {
			last.Pod = data
		}
		_ = a.store.Save(last)
	}
	return nil
}

// readFailure explique une lecture impossible (cluster injoignable, droits) avec le catalogue,
// comme une erreur de commande (FR-026).
func (a *app) readFailure(w io.Writer, err error, args []string, scope kuberead.Scope, cat *catalog.Catalog, opts render.Options) error {
	stderr := err.Error()
	var ke *kuberead.ErrKubectl
	var denied *kuberead.ErrDenied
	switch {
	case errors.As(err, &ke):
		stderr = ke.Stderr
	case errors.As(err, &denied):
		stderr = denied.Message
	}
	cmd := command.Parse(append(args, scopeArgs(scope)...))
	d := diagnose.Diagnose(diagnose.Input{Command: cmd, ExitCode: 1, Stderr: stderr, InvokedAs: "kubectl"}, cat)
	fmt.Fprintln(w, opts.Theme.Title(render.SymbolError+" Le diagnostic n'a pas pu lire le cluster."))
	fmt.Fprintln(w)
	if rerr := render.RenderExplanation(w, d, stderr, opts); rerr != nil {
		return rerr
	}
	return exitError{code: 1}
}

// notFound signale un pod introuvable et propose les noms proches.
func (a *app) notFound(w io.Writer, r *kuberead.Reader, kind, name string, opts render.Options) error {
	t := opts.Theme
	fmt.Fprintln(w, t.Title(render.SymbolError+" Le "+kind+" « "+name+" » est introuvable dans le namespace "+nsLabel(r.Scope)+"."))
	if pods, err := r.ListPods(); err == nil {
		var names []string
		for _, p := range pods.Items {
			names = append(names, p.Metadata.Name)
		}
		if s, ok := textdist.Closest(name, names, max(2, len(name)/3)); ok {
			fmt.Fprintln(w, "   Nom proche : "+t.Emphasis(s))
		}
	}
	fmt.Fprintln(w, "   "+t.Highlight("kubectl get pods"+strings.TrimRight(" "+r.Scope.Flags(), " "), "bash"))
	return exitError{code: 2}
}

func nsLabel(s kuberead.Scope) string {
	if s.Namespace != "" {
		return "« " + s.Namespace + " »"
	}
	return "courant"
}

func scopeArgs(s kuberead.Scope) []string {
	if f := s.Flags(); f != "" {
		return strings.Fields(f)
	}
	return nil
}

// selector écrit les labels d'un selector sous la forme « clé=valeur,… ».
func selector(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ",")
}
