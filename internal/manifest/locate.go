package manifest

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Location rattache un message de kubectl à un endroit d'un manifeste relu (FR-013, FR-015).
type Location struct {
	File      string
	Document  int
	Line      int
	Column    int
	FieldPath string
	Resource  string
}

var (
	reParse      = regexp.MustCompile(`error parsing (\S+?): error converting YAML to JSON: yaml: line (\d+): `)
	reHandled    = regexp.MustCompile(`error when [a-z ]+"([^"]+)": (\w+) in version "[^"]+" cannot be handled as a \w+: (.+)`)
	reUnknown    = regexp.MustCompile(`unknown field "([^"]+)"`)
	reUnmarshal  = regexp.MustCompile(`json: cannot unmarshal \w+ into Go struct field \w+\.([\w.]+) of type`)
	reInvalid    = regexp.MustCompile(`The (\w+) "([^"]+)" is invalid: `)
	reInvalidAt  = regexp.MustCompile(`(?m)(?:^\* |is invalid: )([a-zA-Z][\w.\[\]]*): (?:Invalid value|Required value|Unsupported value|Forbidden|Duplicate value|Too long|Too many)`)
	reMapping    = regexp.MustCompile(`resource mapping not found for name: "([^"]*)" namespace: "[^"]*" from "([^"]+)": no matches for kind "([^"]+)"`)
	reValidating = regexp.MustCompile(`error validating "([^"]+)": error validating data: (apiVersion|kind) not set`)
	reIndex      = regexp.MustCompile(`\[\d+\]`)
)

// Locate traduit un message de kubectl en positions dans les fichiers relus : ligne d'erreur
// YAML relative au document (ramenée au fichier), chemin de champ (FR-015), ressource nommée.
func Locate(stderr string, files []*File) []Location {
	var out []Location
	if m := reParse.FindStringSubmatch(stderr); m != nil {
		if f := findFile(files, m[1]); f != nil {
			rel, _ := strconv.Atoi(m[2])
			for _, d := range f.Documents {
				if d.SyntaxErr != nil {
					line := d.StartLine + rel - 1
					out = append(out, Location{File: f.Path, Document: d.Index, Line: line, Column: firstColumn(f, line), Resource: d.Resource()})
					return out
				}
			}
		}
	}
	if m := reHandled.FindStringSubmatch(stderr); m != nil {
		f := findFile(files, m[1])
		kind := m[2]
		var paths []string
		for _, u := range reUnknown.FindAllStringSubmatch(m[3], -1) {
			paths = append(paths, u[1])
		}
		if u := reUnmarshal.FindStringSubmatch(m[3]); u != nil {
			paths = append(paths, u[1])
		}
		for _, p := range paths {
			if loc, ok := locatePath(f, kind, "", p); ok {
				out = append(out, loc)
			}
		}
		return out
	}
	if m := reInvalid.FindStringSubmatch(stderr); m != nil {
		for _, p := range reInvalidAt.FindAllStringSubmatch(stderr, -1) {
			for _, f := range files {
				if loc, ok := locatePath(f, m[1], m[2], p[1]); ok {
					out = append(out, loc)
					break
				}
			}
		}
		return out
	}
	if m := reMapping.FindStringSubmatch(stderr); m != nil {
		if f := findFile(files, m[2]); f != nil {
			for _, d := range f.Documents {
				if d.Name == m[1] && d.Kind == m[3] && d.Root != nil {
					n := get(d.Root, "kind")
					out = append(out, Location{File: f.Path, Document: d.Index, Line: n.Line, Column: n.Column, FieldPath: "kind", Resource: d.Resource()})
					break
				}
			}
		}
		return out
	}
	if m := reValidating.FindStringSubmatch(stderr); m != nil {
		if f := findFile(files, m[1]); f != nil {
			for _, d := range f.Documents {
				if d.Root != nil && getKey(d.Root, m[2]) == nil {
					out = append(out, Location{File: f.Path, Document: d.Index, Line: d.StartLine, Column: 1, FieldPath: m[2], Resource: d.Resource()})
					break
				}
			}
		}
	}
	return out
}

// locatePath cherche, dans les documents de f de ce kind (et de ce nom s'il est donné), le
// nœud du chemin de champ. Les chemins sans indices (messages « json: cannot unmarshal »)
// sont comparés sans les indices de listes.
func locatePath(f *File, kind, name, path string) (Location, bool) {
	if f == nil {
		return Location{}, false
	}
	for _, d := range f.Documents {
		if d.Root == nil || d.Kind != kind || (name != "" && d.Name != name) {
			continue
		}
		paths := Paths(d.Root)
		n := paths[path+"#key"]
		if n == nil {
			n = paths[path]
		}
		if n == nil {
			n = fuzzyPath(paths, path)
		}
		if n == nil {
			// Chemin inconnu (champ manquant) : on désigne le parent existant le plus proche.
			n = nearestParent(paths, path)
		}
		if n == nil {
			continue
		}
		return Location{File: f.Path, Document: d.Index, Line: n.Line, Column: n.Column, FieldPath: path, Resource: d.Resource()}, true
	}
	return Location{}, false
}

// fuzzyPath compare les chemins sans leurs indices de listes ; en cas de valeur scalaire, le
// nœud de la valeur est préféré à celui de la clé.
func fuzzyPath(paths map[string]*yaml.Node, path string) *yaml.Node {
	var keyNode *yaml.Node
	for p, n := range paths {
		key := strings.HasSuffix(p, "#key")
		bare := reIndex.ReplaceAllString(strings.TrimSuffix(p, "#key"), "")
		if bare != path {
			continue
		}
		if !key {
			return n
		}
		keyNode = n
	}
	return keyNode
}

func nearestParent(paths map[string]*yaml.Node, path string) *yaml.Node {
	for p := path; p != ""; {
		i := strings.LastIndexAny(p, ".[")
		if i < 0 {
			return nil
		}
		p = p[:i]
		if n := paths[p+"#key"]; n != nil {
			return n
		}
	}
	return nil
}

// findFile retrouve un fichier relu par le chemin cité dans le message de kubectl.
func findFile(files []*File, name string) *File {
	for _, f := range files {
		if f.Path == name || filepath.Clean(f.Path) == filepath.Clean(name) {
			return f
		}
	}
	for _, f := range files {
		if filepath.Base(f.Path) == filepath.Base(name) {
			return f
		}
	}
	if len(files) == 1 {
		return files[0]
	}
	return nil
}
