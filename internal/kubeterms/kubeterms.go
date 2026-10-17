package kubeterms

import (
	"sort"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/textdist"
)

// ResourceType décrit un type de ressource Kubernetes de base, tel que kubectl l'accepte.
type ResourceType struct {
	Singular   string   // pod
	Plural     string   // pods
	Short      []string // po
	Group      string   // "" (core), apps, batch…
	Namespaced bool
}

// resourceTypes est la table des types de ressources connus du plugin (ordre : fréquence
// d'usage chez un débutant, utilisée pour départager les suggestions).
var resourceTypes = []ResourceType{
	{"pod", "pods", []string{"po"}, "", true},
	{"deployment", "deployments", []string{"deploy"}, "apps", true},
	{"service", "services", []string{"svc"}, "", true},
	{"namespace", "namespaces", []string{"ns"}, "", false},
	{"configmap", "configmaps", []string{"cm"}, "", true},
	{"secret", "secrets", nil, "", true},
	{"ingress", "ingresses", []string{"ing"}, "networking.k8s.io", true},
	{"replicaset", "replicasets", []string{"rs"}, "apps", true},
	{"statefulset", "statefulsets", []string{"sts"}, "apps", true},
	{"daemonset", "daemonsets", []string{"ds"}, "apps", true},
	{"job", "jobs", nil, "batch", true},
	{"cronjob", "cronjobs", []string{"cj"}, "batch", true},
	{"persistentvolumeclaim", "persistentvolumeclaims", []string{"pvc"}, "", true},
	{"persistentvolume", "persistentvolumes", []string{"pv"}, "", false},
	{"node", "nodes", []string{"no"}, "", false},
	{"event", "events", []string{"ev"}, "", true},
	{"endpoints", "endpoints", []string{"ep"}, "", true},
	{"serviceaccount", "serviceaccounts", []string{"sa"}, "", true},
	{"role", "roles", nil, "rbac.authorization.k8s.io", true},
	{"rolebinding", "rolebindings", nil, "rbac.authorization.k8s.io", true},
	{"clusterrole", "clusterroles", nil, "rbac.authorization.k8s.io", false},
	{"clusterrolebinding", "clusterrolebindings", nil, "rbac.authorization.k8s.io", false},
	{"networkpolicy", "networkpolicies", []string{"netpol"}, "networking.k8s.io", true},
	{"horizontalpodautoscaler", "horizontalpodautoscalers", []string{"hpa"}, "autoscaling", true},
	{"storageclass", "storageclasses", []string{"sc"}, "storage.k8s.io", false},
	{"poddisruptionbudget", "poddisruptionbudgets", []string{"pdb"}, "policy", true},
	{"limitrange", "limitranges", []string{"limits"}, "", true},
	{"resourcequota", "resourcequotas", []string{"quota"}, "", true},
	{"customresourcedefinition", "customresourcedefinitions", []string{"crd", "crds"}, "apiextensions.k8s.io", false},
}

// ResourceTypes renvoie la table des types connus.
func ResourceTypes() []ResourceType { return resourceTypes }

// ResolveType reconnaît un type de ressource tel que tapé (singulier, pluriel, nom court,
// avec ou sans groupe : « deployments.apps », « deploy », « Pod »).
func ResolveType(s string) (ResourceType, bool) {
	name := strings.ToLower(s)
	if i := strings.IndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	if name == "" {
		return ResourceType{}, false
	}
	for _, rt := range resourceTypes {
		if name == rt.Singular || name == rt.Plural {
			return rt, true
		}
		for _, sh := range rt.Short {
			if name == sh {
				return rt, true
			}
		}
	}
	return ResourceType{}, false
}

// ClosestType propose le type de ressource le plus proche d'un type inconnu, dans la forme
// la plus proche de ce qui a été tapé (singulier ou pluriel). Les noms courts ne sont
// proposés que pour une saisie de trois caractères au plus.
func ClosestType(s string) (string, bool) {
	var candidates []string
	for _, rt := range resourceTypes {
		candidates = append(candidates, rt.Singular, rt.Plural)
		if len(s) <= 3 {
			candidates = append(candidates, rt.Short...)
		}
	}
	return closest(strings.ToLower(s), candidates)
}

// subcommands est la liste des sous-commandes de premier niveau de kubectl.
var subcommands = []string{
	"get", "describe", "apply", "create", "delete", "run", "expose", "logs", "exec", "scale",
	"rollout", "set", "edit", "diff", "config", "port-forward", "attach", "cp", "label",
	"annotate", "patch", "replace", "explain", "top", "cordon", "uncordon", "drain", "taint",
	"auth", "api-resources", "api-versions", "cluster-info", "version", "wait", "debug",
	"proxy", "kustomize", "autoscale", "certificate", "plugin", "completion", "events", "alpha",
}

// subSubcommands liste, pour les sous-commandes qui en ont, les mots de second niveau.
var subSubcommands = map[string][]string{
	"config": {
		"use-context", "current-context", "get-contexts", "set-context", "view",
		"get-clusters", "get-users", "set-cluster", "set-credentials", "delete-context",
		"rename-context", "set", "unset", "delete-cluster", "delete-user",
	},
	"rollout": {"status", "history", "undo", "restart", "pause", "resume"},
	"set":     {"image", "env", "resources", "selector", "serviceaccount", "subject"},
	"create": {
		"deployment", "namespace", "secret", "configmap", "service", "job", "cronjob",
		"ingress", "role", "rolebinding", "clusterrole", "clusterrolebinding", "serviceaccount",
		"quota", "priorityclass", "poddisruptionbudget", "token",
	},
	"auth":        {"can-i", "reconcile", "whoami"},
	"top":         {"pod", "pods", "node", "nodes"},
	"certificate": {"approve", "deny"},
	"apply":       {"edit-last-applied", "set-last-applied", "view-last-applied"},
	"plugin":      {"list"},
}

// IsSubcommand indique si s est une sous-commande de premier niveau de kubectl.
func IsSubcommand(s string) bool {
	for _, c := range subcommands {
		if c == s {
			return true
		}
	}
	return false
}

// SubSubcommands renvoie les mots de second niveau d'une sous-commande (ex. « config »).
func SubSubcommands(cmd string) []string { return subSubcommands[cmd] }

// ClosestSubcommand propose la sous-commande la plus proche d'une sous-commande inconnue.
func ClosestSubcommand(s string) (string, bool) {
	return closest(strings.ToLower(s), subcommands)
}

// Flag décrit une option de kubectl.
type Flag struct {
	Long      string // --namespace
	Short     string // -n
	TakesArg  bool
	Global    bool
	Sensitive bool // valeur secrète (--token, --password)
}

// flags regroupe les options globales de kubectl et les options courantes des
// sous-commandes couvertes (FR-003).
var flags = []Flag{
	// Options globales (kubectl options).
	{"--namespace", "-n", true, true, false},
	{"--context", "", true, true, false},
	{"--kubeconfig", "", true, true, false},
	{"--cluster", "", true, true, false},
	{"--user", "", true, true, false},
	{"--server", "-s", true, true, false},
	{"--token", "", true, true, true},
	{"--as", "", true, true, false},
	{"--as-group", "", true, true, false},
	{"--as-uid", "", true, true, false},
	{"--request-timeout", "", true, true, false},
	{"--v", "-v", true, true, false},
	{"--certificate-authority", "", true, true, false},
	{"--client-certificate", "", true, true, false},
	{"--client-key", "", true, true, false},
	{"--tls-server-name", "", true, true, false},
	{"--cache-dir", "", true, true, false},
	{"--username", "", true, true, false},
	{"--password", "", true, true, true},
	{"--profile", "", true, true, false},
	{"--profile-output", "", true, true, false},
	{"--log-flush-frequency", "", true, true, false},
	{"--vmodule", "", true, true, false},
	{"--insecure-skip-tls-verify", "", false, true, false},
	{"--match-server-version", "", false, true, false},
	{"--warnings-as-errors", "", false, true, false},
	{"--disable-compression", "", false, true, false},
	// Options des sous-commandes.
	{"--filename", "-f", true, false, false},
	{"--kustomize", "-k", true, false, false},
	{"--output", "-o", true, false, false},
	{"--selector", "-l", true, false, false},
	{"--field-selector", "", true, false, false},
	{"--all-namespaces", "-A", false, false, false},
	{"--watch", "-w", false, false, false},
	{"--watch-only", "", false, false, false},
	{"--recursive", "-R", false, false, false},
	{"--dry-run", "", false, false, false},
	{"--show-labels", "", false, false, false},
	{"--sort-by", "", true, false, false},
	{"--no-headers", "", false, false, false},
	{"--container", "-c", true, false, false},
	{"--stdin", "-i", false, false, false},
	{"--tty", "-t", false, false, false},
	{"--image", "", true, false, false},
	{"--port", "", true, false, false},
	{"--target-port", "", true, false, false},
	{"--replicas", "", true, false, false},
	{"--previous", "-p", false, false, false},
	{"--tail", "", true, false, false},
	{"--follow", "", false, false, false},
	{"--since", "", true, false, false},
	{"--timestamps", "", false, false, false},
	{"--force", "", false, false, false},
	{"--grace-period", "", true, false, false},
	{"--all", "", false, false, false},
	{"--overwrite", "", false, false, false},
	{"--type", "", true, false, false},
	{"--name", "", true, false, false},
	{"--restart", "", true, false, false},
	{"--rm", "", false, false, false},
	{"--env", "", true, false, false},
	{"--labels", "", true, false, false},
	{"--from-literal", "", true, false, true},
	{"--from-file", "", true, false, false},
	{"--from-env-file", "", true, false, false},
	{"--docker-server", "", true, false, false},
	{"--docker-username", "", true, false, false},
	{"--docker-password", "", true, false, true},
	{"--docker-email", "", true, false, false},
	{"--server-side", "", false, false, false},
	{"--prune", "", false, false, false},
	{"--validate", "", false, false, false},
	{"--field-manager", "", true, false, false},
	{"--to-revision", "", true, false, false},
	{"--revision", "", true, false, false},
	{"--current-replicas", "", true, false, false},
	{"--resource-version", "", true, false, false},
	{"--wait", "", false, false, false},
	{"--timeout", "", true, false, false},
	{"--cascade", "", false, false, false},
	{"--ignore-not-found", "", false, false, false},
	{"--now", "", false, false, false},
	{"--raw", "", true, false, false},
	{"--subresource", "", true, false, false},
	{"--for", "", true, false, false},
	{"--address", "", true, false, false},
	{"--quiet", "-q", false, false, false},
	{"--command", "", false, false, false},
	{"--expose", "", false, false, false},
	{"--protocol", "", true, false, false},
	{"--cluster-ip", "", true, false, false},
	{"--external-ip", "", true, false, false},
	{"--min", "", true, false, false},
	{"--max", "", true, false, false},
	{"--cpu-percent", "", true, false, false},
	{"--limits", "", true, false, false},
	{"--requests", "", true, false, false},
	{"--local", "", false, false, false},
	{"--current", "", false, false, false},
	{"--minify", "", false, false, false},
	{"--raw-output", "", false, false, false},
	{"--diff-filter", "", true, false, false},
	{"--pod-running-timeout", "", true, false, false},
	{"--limit-bytes", "", true, false, false},
	{"--max-log-requests", "", true, false, false},
	{"--prefix", "", false, false, false},
	{"--overrides", "", true, false, false},
	{"--attach", "", false, false, false},
	{"--leave-stdin-open", "", false, false, false},
	{"--privileged", "", false, false, false},
	{"--hostport", "", true, false, false},
	{"--schedule", "", true, false, false},
	{"--from", "", true, false, false},
	{"--verb", "", true, false, false},
	{"--resource", "", true, false, false},
	{"--clusterrole", "", true, false, false},
	{"--role", "", true, false, false},
	{"--serviceaccount", "", true, false, false},
	{"--group", "", true, false, false},
	{"--rule", "", true, false, false},
	{"--class", "", true, false, false},
	{"--annotation", "", true, false, false},
	{"--target", "", true, false, false},
	{"--copy-to", "", true, false, false},
	{"--share-processes", "", false, false, false},
	{"--profile-name", "", true, false, false},
}

// LookupFlag cherche une option par sa forme longue (« --namespace ») ou courte (« -n »).
func LookupFlag(name string) (Flag, bool) {
	for _, f := range flags {
		if name == f.Long || (f.Short != "" && name == f.Short) {
			return f, true
		}
	}
	return Flag{}, false
}

// ShortTakesArg indique si une option courte d'une lettre prend une valeur.
func ShortTakesArg(letter byte) bool {
	f, ok := LookupFlag("-" + string(letter))
	return ok && f.TakesArg
}

// ClosestFlag propose l'option longue la plus proche d'une option inconnue (« --nmespace »).
func ClosestFlag(name string) (string, bool) {
	var candidates []string
	for _, f := range flags {
		candidates = append(candidates, f.Long)
	}
	return closest(strings.ToLower(name), candidates)
}

// closest choisit le candidat le plus proche, avec un seuil qui dépend de la longueur du mot
// et, à distance égale, préfère un candidat de même longueur (« pdos » → « pods » plutôt que
// « pod »), puis l'ordre de la liste.
func closest(word string, candidates []string) (string, bool) {
	maxDist := 2
	if l := len([]rune(word)); l > 8 {
		maxDist = l / 4
	}
	type scored struct {
		name      string
		dist, gap int
		order     int
	}
	var best []scored
	for i, c := range candidates {
		d := textdist.TypoDistance(word, c)
		if d == 0 {
			return "", false
		}
		if d > maxDist {
			continue
		}
		gap := len([]rune(word)) - len([]rune(c))
		if gap < 0 {
			gap = -gap
		}
		best = append(best, scored{c, d, gap, i})
	}
	if len(best) == 0 {
		return "", false
	}
	sort.SliceStable(best, func(i, j int) bool {
		if best[i].dist != best[j].dist {
			return best[i].dist < best[j].dist
		}
		if best[i].gap != best[j].gap {
			return best[i].gap < best[j].gap
		}
		return best[i].order < best[j].order
	})
	return best[0].name, true
}
