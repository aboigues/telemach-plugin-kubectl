package redact

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// reviewedSensitiveFields sont les champs du schéma Kubernetes dont le nom contient un mot
// sensible et dont la valeur reste masquée, après examen : un chemin vers un fichier de
// secret ou une donnée d'identification, que l'on préfère ne pas afficher.
var reviewedSensitiveFields = map[string]string{
	"gmsaCredentialSpec": "spécification d'identifiants Windows (JSON) : masquée par prudence",
	"secretFile":         "chemin d'un fichier de secret (CephFS) : masqué par prudence",
	"secret":             "volume Secret (objet, jamais masqué) ; « secret: valeur » hors schéma reste masqué",
}

// LL-015 (issue #11) : tout champ des schémas embarqués dont le nom contient un mot sensible
// (key, token, auth…) est soit une clé structurelle, jamais masquée, soit examiné et listé
// dans reviewedSensitiveFields. Un nouveau champ apporté par une version de Kubernetes fait
// échouer ce test tant qu'il n'est pas classé.
func TestSchemaSensitiveFieldsReviewed(t *testing.T) {
	files, err := filepath.Glob("../schema/data/*.json.gz")
	if err != nil || len(files) == 0 {
		t.Fatalf("schémas embarqués introuvables : %v", err)
	}
	fields := map[string]bool{}
	for _, f := range files {
		for name := range propertyNames(t, f) {
			fields[name] = true
		}
	}
	var unreviewed []string
	for name := range fields {
		if !IsSensitiveName(name) || isStructuralKey(name+":") {
			continue
		}
		if _, ok := reviewedSensitiveFields[name]; !ok {
			unreviewed = append(unreviewed, name)
		}
	}
	sort.Strings(unreviewed)
	for _, name := range unreviewed {
		t.Errorf("champ %q : nom sensible ; l'ajouter à structuralKeys (valeur non secrète) ou à reviewedSensitiveFields (valeur masquée, avec justification)", name)
	}
}

// propertyNames renvoie les noms des propriétés de tous les schémas d'un fichier embarqué.
func propertyNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // fichier du dépôt
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schemas map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"schemas"`
	}
	if err := json.NewDecoder(gz).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, s := range doc.Schemas {
		for name := range s.Properties {
			out[name] = true
		}
	}
	return out
}
