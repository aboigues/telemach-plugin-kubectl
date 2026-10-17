package schema

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sync"
)

//go:embed data/*.json.gz
var data embed.FS

// Schema est un schéma OpenAPI v3 réduit aux mots-clés utiles (scripts/gen-schemas).
type Schema struct {
	Type                 string             `json:"type"`
	Format               string             `json:"format"`
	Properties           map[string]*Schema `json:"properties"`
	Required             []string           `json:"required"`
	Items                *Schema            `json:"items"`
	AdditionalProperties *Schema            `json:"additionalProperties"`
	Ref                  string             `json:"$ref"`
	AllOf                []*Schema          `json:"allOf"`
	Enum                 []any              `json:"enum"`
	IntOrString          bool               `json:"x-kubernetes-int-or-string"`
	PreserveUnknown      bool               `json:"x-kubernetes-preserve-unknown-fields"`
}

// SchemaSet regroupe les règles d'une version mineure de Kubernetes (data-model.md).
type SchemaSet struct {
	Version string             `json:"version"` // « 1.37 »
	Source  string             `json:"source"`  // tag Kubernetes d'origine
	Kinds   map[string]string  `json:"kinds"`   // « apps/v1/Deployment » → nom du schéma
	Schemas map[string]*Schema `json:"schemas"`
}

var (
	loadOnce sync.Once
	sets     map[string]*SchemaSet
	loadErr  error
)

// load décompresse les règles embarquées, une seule fois.
func load() {
	sets = map[string]*SchemaSet{}
	for _, v := range Supported {
		raw, err := data.ReadFile("data/v" + v + ".json.gz")
		if err != nil {
			loadErr = err
			return
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			loadErr = err
			return
		}
		content, err := io.ReadAll(zr)
		if err != nil {
			loadErr = err
			return
		}
		var s SchemaSet
		if err := json.Unmarshal(content, &s); err != nil {
			loadErr = fmt.Errorf("règles %s : %w", v, err)
			return
		}
		sets[v] = &s
	}
}

var minorVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// Select choisit les règles correspondant à la version mineure du client kubectl
// (FR-014a) ; si elle est inconnue ou hors de la plage embarquée, la plus récente est
// utilisée et fallback vaut true.
func Select(kubectlVersion string) (set *SchemaSet, fallback bool, err error) {
	loadOnce.Do(load)
	if loadErr != nil {
		return nil, false, loadErr
	}
	if m := minorVersion.FindStringSubmatch(kubectlVersion); m != nil {
		if s, ok := sets[m[1]+"."+m[2]]; ok {
			return s, false, nil
		}
	}
	return sets[Latest()], true, nil
}

// Lookup renvoie le schéma racine d'une ressource (apiVersion « apps/v1 », kind
// « Deployment »).
func (s *SchemaSet) Lookup(apiVersion, kind string) (*Schema, bool) {
	name, ok := s.Kinds[apiVersion+"/"+kind]
	if !ok {
		return nil, false
	}
	sch, ok := s.Schemas[name]
	return sch, ok
}

// KnownKind indique si un kind est couvert, quelle que soit l'apiVersion, et renvoie
// l'apiVersion attendue.
func (s *SchemaSet) KnownKind(kind string) (apiVersion string, ok bool) {
	for gvk := range s.Kinds {
		i := len(gvk) - len(kind) - 1
		if i > 0 && gvk[i:] == "/"+kind {
			return gvk[:i], true
		}
	}
	return "", false
}

// KindNames renvoie les kinds couverts.
func (s *SchemaSet) KindNames() []string {
	var out []string
	for gvk := range s.Kinds {
		for i := len(gvk) - 1; i >= 0; i-- {
			if gvk[i] == '/' {
				out = append(out, gvk[i+1:])
				break
			}
		}
	}
	return out
}

// resolve suit les références ($ref, allOf à un seul élément) jusqu'au schéma effectif.
func (s *SchemaSet) resolve(sch *Schema) *Schema {
	for i := 0; sch != nil && i < 20; i++ {
		switch {
		case sch.Ref != "":
			sch = s.Schemas[sch.Ref]
		case len(sch.AllOf) == 1 && sch.Type == "" && len(sch.Properties) == 0:
			sch = sch.AllOf[0]
		default:
			return sch
		}
	}
	return sch
}
