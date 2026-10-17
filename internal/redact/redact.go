package redact

import (
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Mask remplace toute valeur sensible.
const Mask = "••••••"

// sensitiveName reconnaît un nom de variable ou de clé sensible, sans distinction de casse.
const sensitiveName = `[A-Za-z0-9_.-]*(?i:PASSWORD|PASSWD|SECRET|TOKEN|KEY|CREDENTIAL|AUTH|CERT)[A-Za-z0-9_.-]*`

var (
	// NOM: "valeur" ou NOM: 'valeur' (YAML, valeur entre guillemets).
	quotedYAML = regexp.MustCompile(`(` + sensitiveName + `\s*:\s*)(["'])[^"'\n]*(["'])`)
	// NOM: valeur (clé YAML en début de ligne, éventuellement après « - », ou recopiée par
	// kubectl après le « : » d'un message, LL-003). L'ancrage évite de masquer du texte libre
	// comme « error: You must be logged in… » ou « error: invalid token: … ».
	plainYAML = regexp.MustCompile(`(?m)((?:^[ \t]*(?:-[ \t]+)?|:[ \t]+)` + sensitiveName + `[ \t]*:[ \t]+)([^\s"'#|>][^\n]*)`)
	// NOM=valeur (variable d'environnement, fichier .env).
	assignment = regexp.MustCompile(`(` + sensitiveName + `=)([^\s"']+|"[^"\n]*"|'[^'\n]*')`)
	// Variable d'environnement d'un manifeste : « name: NOM_SENSIBLE » puis « value: … ».
	envPair = regexp.MustCompile(`(?m)(name:[ \t]*["']?` + sensitiveName + `["']?[ \t]*\n[ \t]*value:[ \t]*)([^\n]+)`)
	// schéma://utilisateur:mot-de-passe@hôte
	urlCredentials = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s:/@]+:)([^\s@/]+)(@)`)
	// En-tête Authorization: Bearer|Basic|Token <valeur>
	authorization = regexp.MustCompile(`(?i)(Authorization:\s*(?:Bearer|Basic|Token)\s+)(\S+)`)
	// Jetons reconnaissables par leur forme : JWT (jetons de compte de service), GitHub,
	// GitLab, Docker Hub, AWS.
	knownTokens = regexp.MustCompile(`\b(?:eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{22,}|glpat-[A-Za-z0-9_-]{20,}|dckr_pat_[A-Za-z0-9_-]+|AKIA[0-9A-Z]{16})`)
	// Bloc PEM (certificat, clé privée).
	pemBlock = regexp.MustCompile(`(-----BEGIN [A-Z0-9 ]+-----)[\s\S]*?(-----END [A-Z0-9 ]+-----)`)
	// Options kubectl dont la valeur est secrète, sous la forme --option=valeur.
	secretFlagEq = regexp.MustCompile(`((?:^|[\s'"])--(?:token|password|docker-password)=)(\S+)`)
	// Options kubectl dont la valeur est secrète, sous la forme --option valeur.
	secretFlagSep = regexp.MustCompile(`((?:^|[\s'"])--(?:token|password|docker-password)[ \t]+)([^\s-]\S*)`)
	// --from-literal=clé=valeur : seule la valeur est masquée.
	fromLiteral = regexp.MustCompile(`((?:^|[\s'"])--from-literal[= ][^=\s]+=)(\S+)`)
	// Annotation qui recopie tout le manifeste appliqué, secrets compris.
	lastApplied = regexp.MustCompile(`(kubectl\.kubernetes\.io/last-applied-configuration["']?[ \t]*:[ \t]*)([^\n]+)`)
)

// secretFlags sont les options kubectl dont la valeur suivante est secrète.
var secretFlags = map[string]bool{"--token": true, "--password": true, "--docker-password": true}

// Redact masque les valeurs sensibles d'un texte libre (sortie d'erreur, extrait de
// manifeste, commande, journal). Le nom de la variable ou de la clé est conservé pour
// rester pédagogique. La fonction est idempotente.
func Redact(s string) string {
	s = pemBlock.ReplaceAllString(s, "${1}"+Mask+"${2}")
	s = knownTokens.ReplaceAllString(s, Mask)
	s = authorization.ReplaceAllString(s, "${1}"+Mask)
	s = urlCredentials.ReplaceAllString(s, "${1}"+Mask+"${3}")
	s = lastApplied.ReplaceAllString(s, "${1}"+Mask)
	s = secretFlagEq.ReplaceAllString(s, "${1}"+Mask)
	s = secretFlagSep.ReplaceAllString(s, "${1}"+Mask)
	s = fromLiteral.ReplaceAllString(s, "${1}"+Mask)
	s = envPair.ReplaceAllStringFunc(s, func(m string) string {
		sub := envPair.FindStringSubmatch(m)
		if isReference(sub[2]) {
			return m
		}
		return sub[1] + Mask
	})
	s = quotedYAML.ReplaceAllStringFunc(s, func(m string) string {
		sub := quotedYAML.FindStringSubmatch(m)
		inner := strings.TrimSuffix(strings.TrimPrefix(m[len(sub[1]):], sub[2]), sub[3])
		if isReference(inner) || isStructuralKey(sub[1]) {
			return m
		}
		return sub[1] + sub[2] + Mask + sub[3]
	})
	s = plainYAML.ReplaceAllStringFunc(s, func(m string) string {
		sub := plainYAML.FindStringSubmatch(m)
		if strings.Contains(sub[2], Mask) || isReference(sub[2]) || isStructuralKey(sub[1]) {
			return m
		}
		return sub[1] + Mask
	})
	s = assignment.ReplaceAllStringFunc(s, func(m string) string {
		sub := assignment.FindStringSubmatch(m)
		if isReference(sub[2]) {
			return m
		}
		return sub[1] + Mask
	})
	return redactSecretData(s)
}

// structuralKeys sont des clés Kubernetes dont le nom contient un mot sensible mais dont la
// valeur n'est pas un secret : références (secretKeyRef, secretName…), noms de clés (key:),
// chemins de fichiers.
var structuralKeys = map[string]bool{
	"key": true, "secretkeyref": true, "secretname": true, "secretref": true,
	"imagepullsecrets": true, "secrets": true, "serviceaccounttoken": true,
	"automountserviceaccounttoken": true, "client-key": true, "client-certificate": true,
	"tokenfile": true, "keyfile": true, "certfile": true,
}

// isStructuralKey indique si le préfixe « clé: » désigne une clé structurelle.
func isStructuralKey(prefix string) bool {
	name := strings.TrimRight(strings.TrimLeft(prefix, " \t-"), " \t:=")
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "_file") || strings.HasSuffix(lower, "path") {
		return true
	}
	return structuralKeys[lower]
}

// isReference indique si une valeur n'est qu'une référence à une variable (${NOM}, $NOM ou
// $(NOM)), qui ne révèle aucun secret.
func isReference(v string) bool {
	v = strings.Trim(strings.TrimSpace(v), `"'`)
	return strings.HasPrefix(v, "$") && !strings.HasPrefix(v, "$$")
}

// redactSecretData masque, dans un texte YAML, toutes les valeurs des blocs « data: » et
// « stringData: » d'un document « kind: Secret ».
func redactSecretData(s string) string {
	if !strings.Contains(s, "Secret") {
		return s
	}
	docs := splitDocuments(s)
	for i, doc := range docs {
		if !isSecretDocument(doc) {
			continue
		}
		docs[i] = maskDataBlocks(doc)
	}
	return strings.Join(docs, "")
}

// splitDocuments découpe un texte YAML en documents en conservant les séparateurs.
func splitDocuments(s string) []string {
	var docs []string
	start := 0
	for i := 0; i < len(s); {
		end := strings.IndexByte(s[i:], '\n')
		if end < 0 {
			break
		}
		line := s[i : i+end]
		if strings.TrimRight(line, " \t\r") == "---" && i > start {
			docs = append(docs, s[start:i])
			start = i
		}
		i += end + 1
	}
	return append(docs, s[start:])
}

// secretKind reconnaît « kind: Secret » en début de ligne ou recopié après le « : » d'un
// message kubectl (LL-003).
var secretKind = regexp.MustCompile(`(?m)(?:^|:[ \t]+)kind:[ \t]*["']?Secret["']?[ \t]*$`)

func isSecretDocument(doc string) bool { return secretKind.MatchString(doc) }

var dataHeader = regexp.MustCompile(`^([ \t]*)(?:data|stringData):[ \t]*(.*)$`)

// maskDataBlocks masque chaque valeur sous « data: » et « stringData: ».
func maskDataBlocks(doc string) string {
	lines := strings.Split(doc, "\n")
	inBlock, blockIndent, childIndent := false, 0, -1
	for i, line := range lines {
		if inBlock {
			trimmed := strings.TrimLeft(line, " \t")
			indent := len(line) - len(trimmed)
			if trimmed == "" {
				continue
			}
			if indent <= blockIndent {
				inBlock = false
			} else {
				if childIndent < 0 {
					childIndent = indent
				}
				if k := strings.Index(trimmed, ":"); k > 0 && indent == childIndent {
					rest := strings.TrimSpace(trimmed[k+1:])
					if rest != "" && rest != Mask {
						lines[i] = line[:indent] + trimmed[:k+1] + " " + Mask
					}
				} else {
					// Suite d'une valeur sur plusieurs lignes (bloc « | »).
					lines[i] = line[:indent] + Mask
				}
				continue
			}
		}
		if m := dataHeader.FindStringSubmatch(line); m != nil {
			rest := strings.TrimSpace(m[2])
			switch {
			case rest == "" || rest == "|" || rest == ">":
				inBlock, blockIndent, childIndent = true, len(m[1]), -1
			case strings.HasPrefix(rest, "{") && rest != "{}" && !strings.Contains(rest, Mask):
				lines[i] = line[:len(line)-len(m[2])] + "{" + Mask + "}"
			}
		}
	}
	return strings.Join(lines, "\n")
}

// RedactArgs masque les valeurs sensibles d'une liste d'arguments kubectl, sans modifier la
// liste d'origine : --token, --password, --docker-password (forme séparée ou avec =),
// valeurs de --from-literal, et toute chaîne reconnue par Redact.
func RedactArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = Redact(a)
	}
	for i := 0; i < len(out)-1; i++ {
		switch {
		case secretFlags[out[i]]:
			out[i+1] = Mask
		case out[i] == "--from-literal":
			if k := strings.IndexByte(out[i+1], '='); k >= 0 {
				out[i+1] = out[i+1][:k+1] + Mask
			}
		}
	}
	for i, a := range out {
		if strings.HasPrefix(a, "--from-literal=") {
			rest := strings.TrimPrefix(a, "--from-literal=")
			if k := strings.IndexByte(rest, '='); k >= 0 {
				out[i] = "--from-literal=" + rest[:k+1] + Mask
			}
		}
	}
	return out
}

var sensitiveNameOnly = regexp.MustCompile(`^` + sensitiveName + `$`)

// IsSensitiveName indique si un nom de variable désigne vraisemblablement un secret
// (PASSWORD, SECRET, TOKEN, KEY, CREDENTIAL, AUTH, CERT…, sans distinction de casse).
func IsSensitiveName(name string) bool {
	return sensitiveNameOnly.MatchString(name)
}

// Position situe une valeur masquée dans un fichier (1-based).
type Position struct {
	Line, Column int
}

// RedactNode masque en place les valeurs sensibles d'un document YAML : données d'un objet
// Secret, valeurs des variables d'environnement au nom sensible, annotation
// last-applied-configuration, et toute clé sensible dont la valeur est un scalaire. Elle
// renvoie la position de chaque valeur masquée, pour masquer aussi les extraits affichés.
func RedactNode(doc *yaml.Node) []Position {
	var masked []Position
	if doc == nil {
		return nil
	}
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	isSecret := scalarValue(root, "kind") == "Secret"
	var walk func(n *yaml.Node, inSecretData bool)
	maskScalar := func(v *yaml.Node) {
		if v.Kind == yaml.ScalarNode && v.Value != Mask && !isReference(v.Value) {
			masked = append(masked, Position{v.Line, v.Column})
			v.Value = Mask
			v.Style = 0
		}
	}
	walk = func(n *yaml.Node, inSecretData bool) {
		switch n.Kind {
		case yaml.MappingNode:
			if name := scalarValue(n, "name"); name != "" && IsSensitiveName(name) {
				if v := mapValue(n, "value"); v != nil {
					maskScalar(v)
				}
			}
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				switch {
				case inSecretData:
					maskAll(v, maskScalar)
				case n == root && isSecret && (k.Value == "data" || k.Value == "stringData"):
					walk(v, true)
				case k.Value == "kubectl.kubernetes.io/last-applied-configuration":
					maskScalar(v)
				case v.Kind == yaml.ScalarNode && IsSensitiveName(k.Value) && !isStructuralKey(k.Value+":"):
					maskScalar(v)
				default:
					walk(v, false)
				}
			}
		case yaml.SequenceNode, yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c, inSecretData)
			}
		}
	}
	walk(root, false)
	return masked
}

// maskAll masque tous les scalaires sous n.
func maskAll(n *yaml.Node, mask func(*yaml.Node)) {
	if n.Kind == yaml.ScalarNode {
		mask(n)
		return
	}
	for i, c := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 {
			continue // les clés d'un Secret (noms des entrées) restent visibles
		}
		maskAll(c, mask)
	}
}

// mapValue renvoie la valeur associée à key dans un nœud de type mapping.
func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// scalarValue renvoie la valeur scalaire associée à key, ou "".
func scalarValue(n *yaml.Node, key string) string {
	if v := mapValue(n, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}
