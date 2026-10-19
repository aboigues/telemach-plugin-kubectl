package diagnose

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kubeterms"
)

// Suggestions et commande corrigée (FR-011, FR-012) : la commande de l'apprenant est reprise
// telle quelle, seul l'élément fautif est remplacé ; contexte, namespace et autres options
// sont conservés. Les arguments sont déjà masqués.
func init() {
	enrichers = append(enrichers, suggest)
}

// placeholders utilisés quand aucune suggestion fiable n'existe.
const (
	placeholderType    = "TYPE"
	placeholderCommand = "SOUS-COMMANDE"
)

// requiredFlagValues donne une valeur d'exemple lisible pour les options obligatoires.
var requiredFlagValues = map[string]string{"image": "IMAGE", "replicas": "NOMBRE", "port": "PORT"}

func suggest(in Input, d *Diagnosis) {
	args := in.Command.Args
	fixed := args
	if d.Entry != nil {
		switch d.Entry.ID {
		case "cmdline.unknown-resource-type":
			typed := d.Captures["type"]
			repl := placeholderType
			if s, ok := kubeterms.ClosestType(typed); ok {
				d.Captures["suggested_type"] = s
				repl = s
			}
			fixed = replaceType(args, typed, repl)
		case "cmdline.unknown-command":
			typed := d.Captures["command"]
			repl := placeholderCommand
			if s, ok := kubeterms.ClosestSubcommand(typed); ok {
				d.Captures["suggested_command"] = s
				repl = s
			}
			fixed = replaceToken(args, typed, repl)
		case "cmdline.unknown-flag":
			fixed = fixFlag(args, d)
		case "cmdline.plugin-flags-before-name":
			fixed = movePluginFlags(args)
		case "cmdline.required-flag":
			flag := d.Captures["flag"]
			value := requiredFlagValues[flag]
			if value == "" {
				value = "VALEUR"
			}
			fixed = append(append([]string(nil), args...), "--"+flag+"="+value)
		case "cmdline.invalid-name":
			name := d.Captures["name"]
			fixed = replaceToken(args, name, dnsName(name))
			d.Captures["suggested_name"] = dnsName(name)
		case "notfound.resource":
			fixed = withoutNamespace(args)
		case "notfound.container-exec":
			name := "CONTENEUR"
			if first, _, _ := strings.Cut(d.Captures["containers"], ", "); first != "" {
				name = first
			}
			fixed = replaceContainer(args, name)
		case "portforward.local-port-in-use":
			fixed = freeLocalPort(args, d.Captures["localPort"])
		}
	}
	d.Captures["fixed_command"] = joinArgs(fixed)
	if field := d.Captures["field"]; field != "" {
		d.Captures["explain_path"] = ExplainPath(d.Captures["kind"], field)
	}
}

// ExplainPath construit l'argument de « kubectl explain » qui documente le niveau où se
// trouve un champ : « deployment.spec.template.spec.containers » pour
// « spec.template.spec.containers[0].imagePullPolicyy ».
func ExplainPath(kind, field string) string {
	bare := indexes.ReplaceAllString(field, "")
	parent := ""
	if i := strings.LastIndex(bare, "."); i > 0 {
		parent = "." + bare[:i]
	}
	if kind == "" {
		kind = "RESSOURCE"
	}
	return strings.ToLower(kind) + parent
}

var indexes = regexp.MustCompile(`\[\d*\]`)

// replaceType remplace le type tapé, seul (« pdos ») ou suivi d'un nom (« pdo/web »).
func replaceType(args []string, typed, repl string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		switch {
		case a == typed:
			out[i] = repl
			return out
		case strings.HasPrefix(a, typed+"/"):
			out[i] = repl + strings.TrimPrefix(a, typed)
			return out
		}
	}
	return out
}

// replaceToken remplace la première occurrence exacte d'un argument.
func replaceToken(args []string, from, to string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if a == from {
			out[i] = to
			return out
		}
	}
	return out
}

// fixFlag corrige une option inconnue : forme longue la plus proche si elle existe, sinon
// l'option (et sa valeur collée) est retirée.
func fixFlag(args []string, d *Diagnosis) []string {
	flag := d.Captures["flag"]
	suggestion := ""
	if strings.HasPrefix(flag, "--") {
		if s, ok := kubeterms.ClosestFlag(flag); ok {
			suggestion = s
			d.Captures["suggested_flag"] = s
		}
	}
	var out []string
	done := false
	for _, a := range args {
		if done {
			out = append(out, a)
			continue
		}
		name, value, hasValue := strings.Cut(a, "=")
		if name != flag {
			out = append(out, a)
			continue
		}
		done = true
		if suggestion == "" {
			continue
		}
		if hasValue {
			out = append(out, suggestion+"="+value)
		} else {
			out = append(out, suggestion)
		}
	}
	return out
}

// movePluginFlags place après « telemach SOUS-COMMANDE » les options écrites avant le nom
// du plugin, que kubectl refuse (research.md R2).
func movePluginFlags(args []string) []string {
	idx := -1
	for i, a := range args {
		if a == "telemach" {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return args
	}
	rest := args[idx:]
	head := 2
	if len(rest) < 2 || strings.HasPrefix(rest[1], "-") {
		head = 1
	}
	out := append([]string(nil), rest[:head]...)
	out = append(out, args[:idx]...)
	return append(out, rest[head:]...)
}

// withoutNamespace retire -n / --namespace et sa valeur.
func withoutNamespace(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" || a == "--namespace":
			i++
		case strings.HasPrefix(a, "--namespace=") || (strings.HasPrefix(a, "-n") && len(a) > 2 && !strings.HasPrefix(a, "--")):
		default:
			out = append(out, a)
		}
	}
	return out
}

// dnsName transforme un nom en nom d'objet valide : minuscules, « - » à la place des autres
// caractères, sans tiret en début ni en fin.
func dnsName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// joinArgs reconstitue une ligne de commande lisible ; un argument qui contient une espace ou
// un caractère spécial du shell est placé entre apostrophes.
func joinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = quote(a)
	}
	return strings.Join(parts, " ")
}

func quote(a string) string {
	if a == "" {
		return "''"
	}
	special := strings.IndexFunc(a, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`'"$\`+"`"+`;&|<>(){}*?!#~`, r)
	}) >= 0
	if !special {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}

// replaceContainer remplace le nom de conteneur donné par -c ou --container.
func replaceContainer(args []string, name string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		switch {
		case (a == "-c" || a == "--container") && i+1 < len(out):
			out[i+1] = name
			return out
		case strings.HasPrefix(a, "-c="), strings.HasPrefix(a, "--container="):
			flag, _, _ := strings.Cut(a, "=")
			out[i] = flag + "=" + name
			return out
		}
	}
	return out
}

// freeLocalPort retire le port local occupé d'un transfert (« 18081:8080 » devient « :8080 ») :
// kubectl choisit alors lui-même un port libre de la machine.
func freeLocalPort(args []string, local string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if rest, ok := strings.CutPrefix(a, local+":"); ok && local != "" {
			out[i] = ":" + rest
			return out
		}
	}
	return out
}
