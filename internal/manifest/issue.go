package manifest

// Types de problèmes (data-model.md § ManifestIssue).
const (
	KindYAMLSyntax        = "yaml-syntax"
	KindTabIndent         = "tab-indent"
	KindMissingAPIVersion = "missing-api-version"
	KindMissingKind       = "missing-kind"
	KindUnknownKind       = "unknown-kind"
	KindRemovedAPIVersion = "removed-api-version"
	KindUnknownField      = "unknown-field"
	KindWrongLevel        = "wrong-level"
	KindWrongType         = "wrong-type"
	KindRequiredMissing   = "required-missing"
	KindInvalidEnum       = "invalid-enum"
	KindSelectorMismatch  = "selector-mismatch"
	KindInvalidName       = "invalid-name"
	KindPlaintextSecret   = "plaintext-secret"
	KindUnknownCRD        = "unknown-crd"
	KindNotUTF8           = "not-utf8"
	KindFileNotFound      = "file-not-found"
	KindFileUnreadable    = "file-unreadable"
	KindServerRejected    = "server-rejected" // message du serveur rattaché à une ligne, sans règle locale
	KindNoManifestFile    = "no-manifest-file"
)

// Line est une ligne numérotée d'un extrait de fichier ; Number vaut 0 pour une ligne
// ajoutée dans une version corrigée.
type Line struct {
	Number int
	Text   string
}

// Issue est un problème localisé dans un manifeste (data-model.md § ManifestIssue). Toutes
// les valeurs textuelles sont déjà masquées.
type Issue struct {
	Kind         string
	File         string
	Document     int // 1-based ; 0 si sans objet
	Line, Column int // position dans le FICHIER (1-based) ; 0 si inconnue
	Resource     string
	FieldPath    string
	Suggestion   string
	Detail       string // précision en français
	RelatedLines []int
	RelatedPaths []string // autres champs concernés (labels du modèle de pod, par exemple)
	Excerpt      []Line
	Fixed        []Line
	Blocking     bool
}
