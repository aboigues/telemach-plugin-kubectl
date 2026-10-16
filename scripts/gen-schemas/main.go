// Commande gen-schemas : outil du mainteneur (jamais appelé par le plugin) qui produit les
// règles Kubernetes embarquées (research.md R7, T057).
//
// Pour chaque version, il télécharge les documents OpenAPI v3 publiés dans le dépôt
// kubernetes/kubernetes au tag exact, vérifie leur somme SHA-256 consignée dans sources.txt,
// garde la fermeture transitive des types des ressources couvertes (FR-014), retire les
// descriptions et écrit internal/schema/data/v1.XX.json.gz.
//
//	go run ./scripts/gen-schemas                 # vérifie les sommes et régénère
//	go run ./scripts/gen-schemas -record         # première fois : consigne les sommes
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// versions associe chaque version mineure couverte au dernier correctif publié
// (dl.k8s.io/release/stable-1.XX.txt, vérifié le 2026-10-16).
var versions = map[string]string{
	"1.35": "v1.35.9",
	"1.36": "v1.36.5",
	"1.37": "v1.37.1",
}

// documents OpenAPI v3 nécessaires aux ressources couvertes.
var documents = []string{
	"api__v1_openapi.json",
	"apis__apps__v1_openapi.json",
	"apis__batch__v1_openapi.json",
	"apis__networking.k8s.io__v1_openapi.json",
}

// kinds est la liste des ressources de base couvertes (FR-014), par groupe/version.
var kinds = map[string]string{ //nolint:gosec // noms de schémas OpenAPI, aucun secret
	"v1/Pod":                       "io.k8s.api.core.v1.Pod",
	"v1/Service":                   "io.k8s.api.core.v1.Service",
	"v1/ConfigMap":                 "io.k8s.api.core.v1.ConfigMap",
	"v1/Secret":                    "io.k8s.api.core.v1.Secret",
	"v1/Namespace":                 "io.k8s.api.core.v1.Namespace",
	"v1/PersistentVolumeClaim":     "io.k8s.api.core.v1.PersistentVolumeClaim",
	"apps/v1/Deployment":           "io.k8s.api.apps.v1.Deployment",
	"apps/v1/StatefulSet":          "io.k8s.api.apps.v1.StatefulSet",
	"apps/v1/DaemonSet":            "io.k8s.api.apps.v1.DaemonSet",
	"batch/v1/Job":                 "io.k8s.api.batch.v1.Job",
	"batch/v1/CronJob":             "io.k8s.api.batch.v1.CronJob",
	"networking.k8s.io/v1/Ingress": "io.k8s.api.networking.v1.Ingress",
}

// keptKeys sont les mots-clés OpenAPI utiles au validateur ; tout le reste (descriptions,
// valeurs par défaut, extensions de fusion) est retiré.
var keptKeys = map[string]bool{
	"type": true, "properties": true, "required": true, "items": true,
	"additionalProperties": true, "$ref": true, "allOf": true, "enum": true, "format": true,
	"x-kubernetes-int-or-string": true, "x-kubernetes-preserve-unknown-fields": true,
}

// Output est le format des fichiers embarqués.
type Output struct {
	Version string                     `json:"version"`
	Source  string                     `json:"source"`
	Kinds   map[string]string          `json:"kinds"`
	Schemas map[string]json.RawMessage `json:"schemas"`
}

func main() {
	record := flag.Bool("record", false, "consigne les sommes SHA-256 téléchargées dans sources.txt")
	dir := flag.String("dir", "scripts/gen-schemas", "dossier de sources.txt")
	out := flag.String("out", "internal/schema/data", "dossier de sortie")
	flag.Parse()
	if err := run(*dir, *out, *record); err != nil {
		fmt.Fprintln(os.Stderr, "gen-schemas :", err)
		os.Exit(1)
	}
}

func run(dir, out string, record bool) error {
	sums, err := readSums(filepath.Join(dir, "sources.txt"))
	if err != nil && !record {
		return err
	}
	if sums == nil {
		sums = map[string]string{}
	}
	var minors []string
	for m := range versions {
		minors = append(minors, m)
	}
	sort.Strings(minors)
	total := 0
	for _, minor := range minors {
		tag := versions[minor]
		all := map[string]json.RawMessage{}
		for _, doc := range documents {
			key := tag + "/" + doc
			data, err := download("https://raw.githubusercontent.com/kubernetes/kubernetes/" + tag + "/api/openapi-spec/v3/" + doc)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			if record {
				sums[key] = got
			} else if sums[key] != got {
				return fmt.Errorf("%s : somme SHA-256 %s différente de celle consignée (%s)", key, got, sums[key])
			}
			var parsed struct {
				Components struct {
					Schemas map[string]json.RawMessage `json:"schemas"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				return fmt.Errorf("%s : %w", key, err)
			}
			for name, s := range parsed.Components.Schemas {
				all[name] = s
			}
		}
		kept, err := closure(all)
		if err != nil {
			return fmt.Errorf("%s : %w", tag, err)
		}
		o := Output{Version: minor, Source: "kubernetes/kubernetes " + tag + " api/openapi-spec/v3", Kinds: kinds, Schemas: kept}
		n, err := write(filepath.Join(out, "v"+minor+".json.gz"), o)
		if err != nil {
			return err
		}
		total += n
		fmt.Printf("v%s (%s) : %d types, %d octets compressés\n", minor, tag, len(kept), n)
	}
	fmt.Printf("total : %d octets compressés (objectif < 2 Mio)\n", total)
	if record {
		return writeSums(filepath.Join(dir, "sources.txt"), sums)
	}
	return nil
}

// closure garde les schémas atteignables depuis les ressources couvertes, nettoyés.
func closure(all map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	kept := map[string]json.RawMessage{}
	var queue []string
	for _, name := range kinds {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if _, done := kept[name]; done {
			continue
		}
		raw, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("schéma %s introuvable", name)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		cleaned := clean(v, &queue)
		data, err := json.Marshal(cleaned)
		if err != nil {
			return nil, err
		}
		kept[name] = data
	}
	return kept, nil
}

// clean retire les mots-clés inutiles et ajoute à queue les types référencés.
func clean(v any, queue *[]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			if !keptKeys[k] {
				continue
			}
			switch k {
			case "$ref":
				ref, _ := val.(string)
				name := strings.TrimPrefix(ref, "#/components/schemas/")
				*queue = append(*queue, name)
				out[k] = name
			case "properties":
				props := map[string]any{}
				if m, ok := val.(map[string]any); ok {
					for pk, pv := range m {
						props[pk] = clean(pv, queue)
					}
				}
				out[k] = props
			default:
				out[k] = clean(val, queue)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = clean(x, queue)
		}
		return out
	default:
		return v
	}
}

func write(path string, o Output) (int, error) {
	data, err := json.Marshal(o)
	if err != nil {
		return 0, err
	}
	var b bytes.Buffer
	zw, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	zw.ModTime = time.Time{} // fichier reproductible
	if _, err := zw.Write(data); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, err
	}
	return b.Len(), os.WriteFile(path, b.Bytes(), 0o600)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url) //nolint:gosec,noctx // URL construite à partir de constantes
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s : HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func readSums(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // fichier du dépôt
	if err != nil {
		return nil, fmt.Errorf("sources.txt illisible (lancer avec -record la première fois) : %w", err)
	}
	defer f.Close() //nolint:errcheck
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			sums[fields[1]] = fields[0]
		}
	}
	return sums, sc.Err()
}

func writeSums(path string, sums map[string]string) error {
	var keys []string
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Sommes SHA-256 des documents OpenAPI v3 de Kubernetes utilisés par gen-schemas.\n")
	b.WriteString("# Format : somme  tag/document. Régénérer avec « go run ./scripts/gen-schemas -record ».\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s  %s\n", sums[k], k)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
