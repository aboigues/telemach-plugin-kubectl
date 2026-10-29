package diagnose

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/catalog"
	"github.com/aboigues/telemach-plugin-kubectl/internal/command"
	"github.com/aboigues/telemach-plugin-kubectl/internal/redact"
)

// Confidence indique dans quelle mesure l'erreur est reconnue.
type Confidence string

const (
	Recognized Confidence = "recognized" // un motif du catalogue correspond au message
	Partial    Confidence = "partial"    // reconnue en partie (code de sortie, lecture refusée…)
	Unknown    Confidence = "unknown"    // non reconnue : aucune cause n'est avancée (FR-008)
)

// Origin indique d'où vient l'erreur (FR-004).
type Origin string

const (
	OriginKubectl     Origin = "kubectl"     // refusée par kubectl lui-même (ligne de commande, kubeconfig)
	OriginCluster     Origin = "cluster"     // renvoyée par le serveur API
	OriginApplication Origin = "application" // programme exécuté dans le conteneur
	OriginUnknown     Origin = "unknown"
)

// Source indique ce qui a produit le diagnostic.
type Source string

const (
	SourceCommand Source = "command"
	SourceCheck   Source = "check"
	SourcePod     Source = "pod"
)

// Diagnosis est le résultat de l'analyse (data-model.md § Diagnosis).
type Diagnosis struct {
	Source      Source
	Confidence  Confidence
	Origin      Origin
	Entry       *catalog.Entry
	Captures    map[string]string // valeurs extraites, déjà masquées
	Fingerprint string
}

// Input regroupe ce qui est connu de la commande en échec.
type Input struct {
	Command     command.Command
	ExitCode    int
	Stderr      string // déjà nettoyé et masqué
	Interactive bool   // stderr non capturé
	InvokedAs   string // « kubectl » ou « k »
}

// Diagnose cherche dans le catalogue l'entrée correspondant à l'erreur. Les entrées à motifs
// sont essayées d'abord, dans l'ordre du catalogue, puis les entrées fondées sur le seul code
// de sortie. Sans correspondance, le diagnostic est « unknown » (FR-008).
func Diagnose(in Input, cat *catalog.Catalog) Diagnosis {
	d := Diagnosis{
		Source:      SourceCommand,
		Confidence:  Unknown,
		Origin:      OriginUnknown,
		Captures:    baseCaptures(in),
		Fingerprint: Fingerprint(in.Command, in.Stderr),
	}
	if cat != nil {
		if e, captures, ok := matchPatterns(in, cat); ok {
			d.Entry, d.Confidence = e, Recognized
			for k, v := range captures {
				d.Captures[k] = v
			}
		} else if e, ok := matchExitCode(in, cat); ok {
			d.Entry, d.Confidence = e, Partial
		}
	}
	d.Origin = originFor(in, d)
	if d.Entry == nil && cat != nil && d.Origin == OriginApplication {
		if e, ok := matchFallback(catalog.FallbackApplication, in, cat); ok {
			d.Entry, d.Confidence = e, Partial
		}
	}
	enrich(in, &d)
	return d
}

// baseCaptures fournit aux gabarits les éléments de la commande de l'apprenant.
func baseCaptures(in Input) map[string]string {
	c := in.Command
	kubectl := in.InvokedAs
	if kubectl == "" {
		kubectl = "kubectl"
	}
	captures := map[string]string{"kubectl": kubectl, "sub": c.Sub(), "exit_code": strconv.Itoa(in.ExitCode)}
	if c.Namespace != "" {
		captures["namespace"] = c.Namespace
	}
	if c.Context != "" {
		captures["context"] = c.Context
	}
	if c.ResourceType != "" {
		captures["type"] = c.ResourceType
	}
	if len(c.ResourceNames) > 0 {
		captures["name"] = c.ResourceNames[0]
	}
	captures["scope"] = scopeFlags(c)
	return captures
}

// scopeFlags reconstruit les options de portée (namespace, contexte, kubeconfig) à reprendre
// dans les commandes proposées (FR-012).
func scopeFlags(c command.Command) string {
	var parts []string
	if c.Namespace != "" {
		parts = append(parts, "-n "+c.Namespace)
	}
	if c.Context != "" {
		parts = append(parts, "--context "+c.Context)
	}
	if c.Kubeconfig != "" {
		parts = append(parts, "--kubeconfig "+c.Kubeconfig)
	}
	return strings.Join(parts, " ")
}

func matchPatterns(in Input, cat *catalog.Catalog) (*catalog.Entry, map[string]string, bool) {
	if in.Stderr == "" {
		return nil, nil, false
	}
	for _, e := range cat.Entries {
		if len(e.Compiled()) == 0 || e.Fallback == catalog.FallbackManifestIssue || !applies(e, in) {
			continue
		}
		for _, re := range e.Compiled() {
			m := re.FindStringSubmatch(in.Stderr)
			if m == nil {
				continue
			}
			captures := map[string]string{}
			for i, name := range re.SubexpNames() {
				if name != "" && i < len(m) && m[i] != "" {
					captures[name] = redact.Redact(m[i])
				}
			}
			return e, captures, true
		}
	}
	return nil, nil, false
}

// matchExitCode cherche une entrée de commande qui déclare explicitement ce code de sortie.
func matchExitCode(in Input, cat *catalog.Catalog) (*catalog.Entry, bool) {
	for _, e := range cat.Entries {
		if len(e.ExitCodes) > 0 && len(e.Patterns) == 0 && len(e.PodReasons) == 0 && applies(e, in) {
			return e, true
		}
	}
	return nil, false
}

// matchFallback renvoie l'entrée de repli associée à une origine.
func matchFallback(fallback string, in Input, cat *catalog.Catalog) (*catalog.Entry, bool) {
	for _, e := range cat.Entries {
		if e.Fallback == fallback && applies(e, in) {
			return e, true
		}
	}
	return nil, false
}

// applies vérifie la sous-commande et le code de sortie attendus par l'entrée.
func applies(e *catalog.Entry, in Input) bool {
	if len(e.ExitCodes) > 0 && !slices.Contains(e.ExitCodes, in.ExitCode) {
		return false
	}
	if !e.MatchesFlags(in.Command.Flags) {
		return false
	}
	return e.AppliesTo(in.Command.Verb()) || e.AppliesTo(in.Command.Sub())
}

var (
	spaces     = regexp.MustCompile(`\s+`)
	timestamps = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z?`)
	podSuffix  = regexp.MustCompile(`-[a-z0-9]{8,10}-[a-z0-9]{5}\b`)
	klogPrefix = regexp.MustCompile(`(?m)^[EIW]\d{4} \d{2}:\d{2}:\d{2}\.\d+\s+\d+ \S+\] `)
)

// Fingerprint calcule l'empreinte SHA-256 de la commande et de l'erreur normalisées
// (horodatages, préfixes de journal, suffixes aléatoires de pods et espaces neutralisés), à
// partir de données déjà masquées.
func Fingerprint(cmd command.Command, stderr string) string {
	norm := klogPrefix.ReplaceAllString(stderr, "")
	norm = timestamps.ReplaceAllString(norm, "<date>")
	norm = podSuffix.ReplaceAllString(norm, "-<id>")
	norm = strings.TrimSpace(spaces.ReplaceAllString(norm, " "))
	h := sha256.New()
	h.Write([]byte(cmd.Sub() + "\x00" + strings.Join(cmd.Positionals, " ") + "\x00" + norm))
	return hex.EncodeToString(h.Sum(nil))
}

// originFor détermine l'origine de l'erreur. Elle est précisée par les règles de origin.go
// (FR-004) ; à défaut, une erreur reconnue est attribuée à kubectl.
func originFor(in Input, d Diagnosis) Origin {
	for _, rule := range originRules {
		if o, ok := rule(in, d); ok {
			return o
		}
	}
	if d.Confidence == Recognized {
		return OriginKubectl
	}
	return OriginUnknown
}

// originRules est complété par origin.go.
var originRules []func(Input, Diagnosis) (Origin, bool)

// enrich ajoute des captures calculées (suggestions, commande corrigée…).
func enrich(in Input, d *Diagnosis) {
	for _, f := range enrichers {
		f(in, d)
	}
}

// enrichers est complété par les autres fichiers du paquet.
var enrichers []func(Input, *Diagnosis)
