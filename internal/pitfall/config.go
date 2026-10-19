package pitfall

import (
	"sort"
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges de configuration figée (catalogue : internal/catalog/data/rollout.yaml).
const (
	ConfigEnvStale  = "config.env-stale"
	ConfigSecretEnv = "config.secret-env" //nolint:gosec // identifiant de leçon, aucun secret
)

func init() {
	knownIDs = append(knownIDs, ConfigEnvStale, ConfigSecretEnv)
}

// configRefs recense les ConfigMap et Secret dont les valeurs sont figées au démarrage d'un pod
// (variables d'environnement, montages subPath), avec la façon dont chacun est utilisé.
type configRefs struct {
	configMaps map[string][]string
	secrets    map[string][]string
}

func templateRefs(spec kuberead.PodSpec) configRefs {
	refs := configRefs{configMaps: map[string][]string{}, secrets: map[string][]string{}}
	add := func(m map[string][]string, name, use string) {
		if name != "" && !contains(m[name], use) {
			m[name] = append(m[name], use)
		}
	}
	volumes := map[string]kuberead.Volume{}
	for _, v := range spec.Volumes {
		volumes[v.Name] = v
	}
	for _, c := range spec.Containers {
		for _, e := range c.Env {
			if e.ValueFrom == nil {
				continue
			}
			if e.ValueFrom.ConfigMapKeyRef != nil {
				add(refs.configMaps, e.ValueFrom.ConfigMapKeyRef.Name, "variable "+e.Name)
			}
			if e.ValueFrom.SecretKeyRef != nil {
				add(refs.secrets, e.ValueFrom.SecretKeyRef.Name, "variable "+e.Name)
			}
		}
		for _, e := range c.EnvFrom {
			if e.ConfigMapRef != nil {
				add(refs.configMaps, e.ConfigMapRef.Name, "envFrom")
			}
			if e.SecretRef != nil {
				add(refs.secrets, e.SecretRef.Name, "envFrom")
			}
		}
		for _, m := range c.VolumeMounts {
			v, ok := volumes[m.Name]
			if !ok || m.SubPath == "" {
				continue // un volume monté sans subPath est mis à jour par le kubelet
			}
			if v.ConfigMap != nil {
				add(refs.configMaps, v.ConfigMap.Name, "montage subPath "+m.MountPath)
			}
			if v.Secret != nil {
				add(refs.secrets, v.Secret.SecretName, "montage subPath "+m.MountPath)
			}
		}
	}
	return refs
}

// configLink compare la date de dernière modification de chaque ConfigMap figée au démarrage
// du plus ancien pod du Deployment ; un Secret n'est jamais lu, seule la leçon est donnée.
func configLink(ctx Context, s *ObjectSnapshot, dep kuberead.Deployment, pods []kuberead.Pod, podsRead bool) {
	refs := templateRefs(dep.Spec.Template.Spec)
	if len(refs.configMaps) == 0 && len(refs.secrets) == 0 {
		s.AddLink(StepConfig, StateOK, Observation{Value: "aucune ConfigMap ni Secret figé au démarrage"})
		return
	}
	oldest, known := oldestStart(pods)
	state := StateOK
	var obs []Observation
	var stale []Pitfall
	for _, name := range sortedKeys(refs.configMaps) {
		uses := strings.Join(refs.configMaps[name], ", ")
		if !podsRead || !known {
			state = StateUnknown
			obs = append(obs, Observation{Label: "ConfigMap " + name, Value: "pods non lus"})
			continue
		}
		dates, err := ctx.Reader.ConfigMapDates(name)
		if err != nil {
			s.Deny(err)
			state = StateUnknown
			obs = append(obs, Observation{Label: "ConfigMap " + name, Value: "non lue"})
			continue
		}
		if latest := dates.Latest(); latest.After(oldest) {
			obs = append(obs, Observation{Label: "ConfigMap " + name, Value: "modifiée après le démarrage des pods"})
			stale = append(stale, Pitfall{ID: ConfigEnvStale, Step: StepConfig, Severity: Warning, Captures: map[string]string{
				"configmap": name, "refs": uses, "modified": latest.UTC().Format(time.RFC3339), "started": oldest.UTC().Format(time.RFC3339),
			}})
			continue
		}
		obs = append(obs, Observation{Label: "ConfigMap " + name, Value: "inchangée depuis le démarrage"})
	}
	for _, name := range sortedKeys(refs.secrets) {
		obs = append(obs, Observation{Label: "Secret " + name, Value: "non lu (jamais lu par TELEMACH)"})
	}
	s.AddLink(StepConfig, state, obs...)
	for _, p := range stale {
		s.Add(p)
	}
	for _, name := range sortedKeys(refs.secrets) {
		s.Add(Pitfall{ID: ConfigSecretEnv, Step: StepConfig, Severity: Info, Captures: map[string]string{
			"secret": name, "refs": strings.Join(refs.secrets[name], ", "),
		}})
	}
}

// oldestStart renvoie la date de démarrage du plus ancien pod démarré.
func oldestStart(pods []kuberead.Pod) (time.Time, bool) {
	var oldest time.Time
	for _, p := range pods {
		if p.Status.StartTime == nil {
			continue
		}
		if oldest.IsZero() || p.Status.StartTime.Before(oldest) {
			oldest = *p.Status.StartTime
		}
	}
	return oldest, !oldest.IsZero()
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
