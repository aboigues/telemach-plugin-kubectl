package command

import (
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kubeterms"
)

// Command est la commande kubectl analysée (data-model.md § Command). Les arguments reçus
// sont déjà masqués : aucune valeur sensible n'y figure.
type Command struct {
	Args          []string          // arguments d'origine, sans « kubectl »
	Subcommand    []string          // « get », « config use-context », « create secret generic »…
	ResourceType  string            // type tel que tapé (« pdos », « deploy »)
	ResourceNames []string          // noms des ressources visées
	Namespace     string            // -n / --namespace
	Context       string            // --context
	Kubeconfig    string            // --kubeconfig
	Files         []string          // valeurs de -f / --filename, dans l'ordre
	Kustomize     string            // -k / --kustomize
	Recursive     bool              // -R / --recursive
	DryRun        bool              // --dry-run (client ou server)
	Flags         map[string]string // autres options (forme longue si connue), valeur ou ""
	UnknownFlags  []string          // options inconnues de kubectl, telles que tapées
	Positionals   []string          // arguments hors options, après la sous-commande
	Interactive   bool              // commande interactive ou de longue durée (research.md R3)
	AfterDash     []string          // arguments après « -- » (commande du conteneur)

	verbHint string // sous-commande devinée avant l'analyse (sens de -f pour logs)
}

// Sub renvoie la sous-commande complète, mots séparés par une espace (« rollout status »).
func (c Command) Sub() string { return strings.Join(c.Subcommand, " ") }

// Verb renvoie la sous-commande de premier niveau (« get »), ou "".
func (c Command) Verb() string {
	if len(c.Subcommand) == 0 {
		return ""
	}
	return c.Subcommand[0]
}

// IsRereadable indique si une valeur de -f désigne un fichier ou un dossier local que le
// plugin peut relire (ni l'entrée standard « - », ni une URL).
func IsRereadable(path string) bool {
	if path == "-" || path == "" {
		return false
	}
	lower := strings.ToLower(path)
	return !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://")
}

// LocalFiles renvoie les valeurs de -f relisibles.
func (c Command) LocalFiles() []string {
	var out []string
	for _, f := range c.Files {
		if IsRereadable(f) {
			out = append(out, f)
		}
	}
	return out
}

// HasUnrereadableFiles indique si au moins un -f désigne l'entrée standard ou une URL, ou si
// -k est utilisé : la ligne d'une erreur ne peut alors pas être localisée.
func (c Command) HasUnrereadableFiles() bool {
	if c.Kustomize != "" {
		return true
	}
	for _, f := range c.Files {
		if !IsRereadable(f) {
			return true
		}
	}
	return false
}

// Parse analyse les arguments d'une commande kubectl (sans le « kubectl » initial). Les
// options globales sont reconnues avant comme après la sous-commande, avec ou sans « = »,
// et les options courtes avec une valeur collée (« -ndemo »).
func Parse(args []string) Command {
	c := Command{Args: append([]string(nil), args...), Flags: map[string]string{}}
	c.verbHint = guessVerb(args)
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			c.AfterDash = append([]string(nil), args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a, "=")
			f, known := kubeterms.LookupFlag(name)
			if !known {
				c.UnknownFlags = append(c.UnknownFlags, name)
			}
			if !hasValue && known && f.TakesArg && i+1 < len(args) {
				i++
				value, hasValue = args[i], true
			}
			c.setFlag(name, value, hasValue)
		case strings.HasPrefix(a, "-") && len(a) > 1:
			i = c.parseShort(args, i)
		default:
			positionals = append(positionals, a)
		}
	}
	c.splitPositionals(positionals)
	c.Interactive = c.detectInteractive()
	return c
}

// parseShort traite un groupe d'options courtes (« -it », « -n demo », « -ndemo ») et
// renvoie l'indice du dernier argument consommé.
func (c *Command) parseShort(args []string, i int) int {
	a := args[i]
	if name, value, ok := strings.Cut(a, "="); ok && len(name) == 2 {
		c.setFlag(name, value, true)
		return i
	}
	letters := a[1:]
	for k := 0; k < len(letters); k++ {
		name := "-" + string(letters[k])
		if name == "-f" && c.verbHint == "logs" {
			// Pour logs, -f signifie --follow (sans valeur), et non --filename.
			c.setFlag("--follow", "", false)
			continue
		}
		f, known := kubeterms.LookupFlag(name)
		if !known {
			c.UnknownFlags = append(c.UnknownFlags, name)
			c.setFlag(name, "", false)
			continue
		}
		if f.TakesArg {
			if rest := letters[k+1:]; rest != "" {
				c.setFlag(name, rest, true)
				return i
			}
			if i+1 < len(args) {
				c.setFlag(name, args[i+1], true)
				return i + 1
			}
			c.setFlag(name, "", false)
			return i
		}
		c.setFlag(name, "", false)
	}
	return i
}

// setFlag range une option à sa place (champs dédiés ou table Flags, sous sa forme longue).
func (c *Command) setFlag(name, value string, hasValue bool) {
	long := name
	if f, ok := kubeterms.LookupFlag(name); ok {
		long = f.Long
	}
	switch long {
	case "--namespace":
		c.Namespace = value
	case "--context":
		c.Context = value
	case "--kubeconfig":
		c.Kubeconfig = value
	case "--filename":
		c.Files = append(c.Files, value)
	case "--kustomize":
		c.Kustomize = value
	case "--recursive":
		c.Recursive = !hasValue || value == "true"
	case "--dry-run":
		c.DryRun = !hasValue || (value != "none" && value != "false")
	default:
		c.Flags[long] = value
	}
}

// splitPositionals déduit sous-commande, type et noms de ressources des arguments hors
// options.
func (c *Command) splitPositionals(pos []string) {
	if len(pos) == 0 {
		return
	}
	c.Subcommand = []string{pos[0]}
	rest := pos[1:]
	if subs := kubeterms.SubSubcommands(pos[0]); len(subs) > 0 && len(rest) > 0 && contains(subs, rest[0]) {
		c.Subcommand = append(c.Subcommand, rest[0])
		rest = rest[1:]
		// « create secret generic|tls|docker-registry »
		if c.Sub() == "create secret" && len(rest) > 0 {
			c.Subcommand = append(c.Subcommand, rest[0])
			rest = rest[1:]
		}
	}
	c.Positionals = rest
	c.deriveResources(rest)
}

// deriveResources renseigne ResourceType et ResourceNames selon la sous-commande.
func (c *Command) deriveResources(rest []string) {
	switch c.Sub() {
	case "logs", "exec", "attach", "port-forward", "debug", "top pod", "top pods":
		if len(rest) > 0 {
			c.setTypeName(rest[0], "pod")
		}
		return
	case "run":
		c.ResourceType = "pod"
		if len(rest) > 0 {
			c.ResourceNames = []string{rest[0]}
		}
		return
	case "set image", "set env", "set resources", "rollout status", "rollout history",
		"rollout undo", "rollout restart", "rollout pause", "rollout resume", "scale", "expose",
		"autoscale":
		if len(rest) > 0 {
			c.setTypeName(rest[0], "")
			if len(c.ResourceNames) == 0 && len(rest) > 1 && !strings.Contains(rest[1], "=") {
				c.ResourceNames = []string{rest[1]}
			}
		}
		return
	}
	if len(c.Subcommand) >= 2 && c.Verb() == "create" {
		// « create deployment web », « create secret generic s ».
		c.ResourceType = c.Subcommand[1]
		if len(rest) > 0 {
			c.ResourceNames = []string{rest[0]}
		}
		return
	}
	switch c.Verb() {
	case "get", "describe", "delete", "edit", "label", "annotate", "patch", "wait", "explain", "events":
		if len(rest) == 0 {
			return
		}
		if strings.Contains(rest[0], "/") {
			for _, r := range rest {
				t, n, _ := strings.Cut(r, "/")
				if c.ResourceType == "" {
					c.ResourceType = t
				}
				if n != "" {
					c.ResourceNames = append(c.ResourceNames, n)
				}
			}
			return
		}
		c.ResourceType = rest[0]
		for _, n := range rest[1:] {
			if c.Verb() == "label" || c.Verb() == "annotate" {
				if strings.Contains(n, "=") || strings.HasSuffix(n, "-") {
					continue
				}
			}
			c.ResourceNames = append(c.ResourceNames, n)
		}
	}
}

// setTypeName interprète « type/nom » ou un simple nom (avec un type par défaut).
func (c *Command) setTypeName(s, defaultType string) {
	if t, n, ok := strings.Cut(s, "/"); ok {
		c.ResourceType = t
		if n != "" {
			c.ResourceNames = []string{n}
		}
		return
	}
	if defaultType != "" {
		c.ResourceType = defaultType
		c.ResourceNames = []string{s}
		return
	}
	c.ResourceType = s
}

// detectInteractive reconnaît les commandes interactives ou de longue durée : leur sortie
// d'erreur n'est pas redirigée (research.md R3).
func (c *Command) detectInteractive() bool {
	_, stdin := c.Flags["--stdin"]
	_, tty := c.Flags["--tty"]
	_, watch := c.Flags["--watch"]
	_, watchOnly := c.Flags["--watch-only"]
	_, follow := c.Flags["--follow"]
	switch c.Verb() {
	case "attach", "edit", "port-forward", "proxy":
		return true
	case "exec", "run", "debug":
		return stdin || tty
	case "logs":
		return follow
	case "get", "events":
		return watch || watchOnly
	case "apply":
		return c.Sub() == "apply edit-last-applied"
	}
	return false
}

// guessVerb renvoie le premier argument hors option, en sautant les valeurs des options qui
// en prennent une.
func guessVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return ""
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			return a
		}
		if strings.Contains(a, "=") {
			continue
		}
		if f, ok := kubeterms.LookupFlag(a); ok && f.TakesArg {
			i++
		} else if !strings.HasPrefix(a, "--") && len(a) > 2 && kubeterms.ShortTakesArg(a[len(a)-1]) {
			i++
		}
	}
	return ""
}

// HasFlag indique si une option (forme longue) figure dans la commande.
func (c Command) HasFlag(long string) bool {
	_, ok := c.Flags[long]
	return ok
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
