package manifest

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// MaxFileSize est la taille maximale d'un manifeste relu (1 Mio), fichier ou entrée standard.
const MaxFileSize = 1 << 20

// StdinPath désigne l'entrée standard (check -f -) dans les positions affichées.
const StdinPath = "<stdin>"

// File est un manifeste relu, en lecture seule (FR-013, data-model.md § ManifestFile).
type File struct {
	Path      string
	Lines     []string
	Documents []*Document
	Readable  bool
}

// Document est un document YAML d'un fichier, séparé des autres par « --- ».
type Document struct {
	Index      int // 1-based, documents vides exclus (comme kubectl)
	StartLine  int // première ligne du document dans le fichier
	EndLine    int
	Source     string // template d'origine d'un rendu Helm (commentaire « # Source: »)
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	Root       *yaml.Node // mapping racine, positions ramenées au fichier ; nil si illisible
	SyntaxErr  error
	// Masked liste les positions des valeurs sensibles masquées dans Root.
	Masked []Position
}

// Position situe une valeur dans un fichier.
type Position struct{ Line, Column int }

// Resource renvoie « Kind/nom » (ou « /nom » sans kind).
func (d *Document) Resource() string {
	if d.Kind == "" && d.Name == "" {
		return ""
	}
	return d.Kind + "/" + d.Name
}

// manifestExt sont les extensions relues dans un dossier, comme kubectl.
var manifestExt = map[string]bool{".yaml": true, ".yml": true, ".json": true}

// Expand développe les valeurs de -f : fichiers, dossiers (et sous-dossiers avec recursive),
// dans l'ordre de kubectl. Les chemins introuvables produisent un problème.
func Expand(paths []string, recursive bool) (files []string, issues []Issue) {
	for _, p := range paths {
		if p == "-" { // entrée standard, lue par AnalyzeInput
			files = append(files, p)
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			issues = append(issues, Issue{Kind: KindFileNotFound, File: p, Blocking: true})
			continue
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		var found []string
		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // un sous-dossier illisible est ignoré, comme kubectl
			}
			if d.IsDir() {
				if path != p && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			if manifestExt[strings.ToLower(filepath.Ext(path))] {
				found = append(found, path)
			}
			return nil
		})
		sort.Strings(found)
		if len(found) == 0 {
			issues = append(issues, Issue{Kind: KindNoManifestFile, File: p, Blocking: true})
		}
		files = append(files, found...)
	}
	return files, issues
}

// Read relit un manifeste, sans jamais le modifier, et découpe ses documents.
func Read(path string) (*File, []Issue) {
	f := &File{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return f, []Issue{{Kind: KindFileNotFound, File: path, Blocking: true}}
	}
	if info.Size() > MaxFileSize {
		return f, []Issue{{Kind: KindFileUnreadable, File: path, Detail: "fichier de plus de 1 Mio", Blocking: true}}
	}
	data, err := os.ReadFile(path) //nolint:gosec // chemin donné par l'apprenant (-f), lecture seule
	if err != nil {
		detail := "lecture impossible"
		if errors.Is(err, fs.ErrPermission) {
			detail = "droits insuffisants pour lire le fichier"
		}
		return f, []Issue{{Kind: KindFileUnreadable, File: path, Detail: detail, Blocking: true}}
	}
	return parseContent(f, data)
}

// ReadStdin lit l'entrée standard (check -f -) avec la même taille maximale et les mêmes
// contrôles qu'un fichier ; elle est désignée par StdinPath (feature 004, research.md F11).
func ReadStdin(r io.Reader) (*File, []Issue) {
	f := &File{Path: StdinPath}
	if r == nil {
		r = strings.NewReader("")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return f, []Issue{{Kind: KindFileUnreadable, File: StdinPath, Detail: "Lecture de l'entrée standard impossible", Blocking: true}}
	}
	if len(data) > MaxFileSize {
		return f, []Issue{{Kind: KindFileUnreadable, File: StdinPath, Detail: "L'entrée standard dépasse 1 Mio : vérifie un rendu plus petit (un chart, un dossier Kustomize)", Blocking: true}}
	}
	f, issues := parseContent(f, data)
	if len(issues) == 0 && len(f.Documents) == 0 {
		return f, []Issue{{
			Kind: KindNoManifestFile, File: StdinPath, Blocking: true,
			Detail: "L'entrée standard est vide : aucun manifeste n'a été transmis (la commande placée avant « | » n'a-t-elle rien produit ?)",
		}}
	}
	return f, issues
}

// parseContent vérifie l'encodage d'un contenu relu et découpe ses documents.
func parseContent(f *File, data []byte) (*File, []Issue) {
	if !utf8.Valid(data) {
		line, col := firstInvalidUTF8(data)
		return f, []Issue{{Kind: KindNotUTF8, File: f.Path, Line: line, Column: col, Blocking: true}}
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	f.Readable = true
	f.Lines = strings.Split(text, "\n")
	f.Documents = splitDocuments(f.Lines)
	return f, nil
}

// sourceComment reconnaît le commentaire que « helm template » place en tête de chaque
// document rendu : « # Source: chart/templates/api.yaml ».
var sourceComment = regexp.MustCompile(`^#\s*Source:\s*(\S.*?)\s*$`)

// helmSource renvoie le template d'origine annoncé avant la première clé du document.
func helmSource(body []string) string {
	source := ""
	for _, l := range body {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "#") {
			break
		}
		if m := sourceComment.FindStringSubmatch(t); m != nil {
			source = m[1]
		}
	}
	return source
}

// separator reconnaît une ligne « --- » (éventuellement suivie d'un commentaire).
var separator = regexp.MustCompile(`^---(\s.*)?$`)

// splitDocuments découpe le fichier et analyse chaque document, en ramenant les positions
// des nœuds YAML aux lignes du fichier.
func splitDocuments(lines []string) []*Document {
	var docs []*Document
	start := 0
	flush := func(end int) {
		body := lines[start:end]
		if isBlank(body) {
			return
		}
		d := &Document{Index: len(docs) + 1, StartLine: start + 1, EndLine: end, Source: helmSource(body)}
		parse(d, strings.Join(body, "\n"))
		docs = append(docs, d)
	}
	for i, l := range lines {
		if separator.MatchString(strings.TrimRight(l, " \t")) {
			flush(i)
			start = i + 1
		}
	}
	flush(len(lines))
	return docs
}

// parse analyse un document. Les erreurs de syntaxe sont conservées ; elles sont rendues
// pédagogiques par syntax.go.
func parse(d *Document, text string) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		d.SyntaxErr = err
		d.Kind, d.Name = resourceFromText(text)
		return
	}
	if len(doc.Content) == 0 {
		return
	}
	shift(&doc, d.StartLine-1)
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return
	}
	d.Root = root
	d.APIVersion = scalar(root, "apiVersion")
	d.Kind = scalar(root, "kind")
	if meta := get(root, "metadata"); meta != nil {
		d.Name = scalar(meta, "name")
		d.Namespace = scalar(meta, "namespace")
	}
}

// shift décale les numéros de ligne de tous les nœuds.
func shift(n *yaml.Node, offset int) {
	n.Line += offset
	for _, c := range n.Content {
		shift(c, offset)
	}
}

func isBlank(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" && !strings.HasPrefix(t, "#") {
			return false
		}
	}
	return true
}

func firstInvalidUTF8(data []byte) (line, col int) {
	line, col = 1, 1
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return line, col
		}
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
		i += size
	}
	return line, col
}

func get(n *yaml.Node, key string) *yaml.Node {
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

func getKey(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i]
		}
	}
	return nil
}

func scalar(n *yaml.Node, key string) string {
	if v := get(n, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// Paths indexe un document : chemin de champ → nœud de la valeur
// (« spec.template.spec.containers[0].image »).
func Paths(root *yaml.Node) map[string]*yaml.Node {
	out := map[string]*yaml.Node{}
	var walk func(n *yaml.Node, path string)
	walk = func(n *yaml.Node, path string) {
		if path != "" {
			out[path] = n
		}
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				p := n.Content[i].Value
				if path != "" {
					p = path + "." + p
				}
				out[p+"#key"] = n.Content[i]
				walk(n.Content[i+1], p)
			}
		case yaml.SequenceNode:
			for i, c := range n.Content {
				walk(c, path+"["+itoa(i)+"]")
			}
		}
	}
	if root != nil {
		walk(root, "")
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

var (
	kindLine = regexp.MustCompile(`(?m)^kind:[ \t]*["']?([A-Za-z]+)`)
	nameLine = regexp.MustCompile(`(?m)^metadata:[^\n]*\n(?:[ \t]+[^\n]*\n)*?[ \t]+name:[ \t]*["']?([A-Za-z0-9._-]+)`)
)

// resourceFromText retrouve kind et metadata.name dans le texte d'un document que le parseur
// YAML n'a pas pu lire, pour nommer la ressource concernée par l'erreur de syntaxe.
func resourceFromText(text string) (kind, name string) {
	if m := kindLine.FindStringSubmatch(text); m != nil {
		kind = m[1]
	}
	if m := nameLine.FindStringSubmatch(text); m != nil {
		name = m[1]
	}
	return kind, name
}
