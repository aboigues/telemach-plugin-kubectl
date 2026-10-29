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
	// Issue #12 : fichiers valides qui ne sont pas des objets de l'API (informations).
	KindToolConfig   = "tool-config"   // configuration de kubeadm, du kubelet… (pas pour kubectl apply)
	KindHelmTemplate = "helm-template" // template Helm, pas encore du YAML
	KindHelmValues   = "helm-values"   // fichier de valeurs ou de description d'un chart Helm
	// Feature 004.
	KindInvalidCron   = "invalid-cron"   // spec.schedule d'un CronJob refusé par l'analyseur cron
	KindPodSecurity   = "pod-security"   // securityContext à compléter après un refus PodSecurity (explain seulement)
	KindMissingLimits = "missing-limits" // conteneur sans resources.limits (information)
)

// infoKinds ne sont pas des problèmes : check les signale par ℹ, sans changer son code de
// sortie (issue #12). Une ressource personnalisée est valide ; seule sa vérification est
// partielle.
var infoKinds = map[string]bool{
	KindUnknownCRD: true, KindToolConfig: true, KindHelmTemplate: true, KindHelmValues: true,
	KindMissingLimits: true,
}

// Info indique une simple information, qui n'est pas un problème du manifeste.
func (is Issue) Info() bool { return infoKinds[is.Kind] }

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
	Source       string // template d'origine d'un rendu Helm (feature 004)
	FieldPath    string
	Suggestion   string
	Detail       string // précision en français
	RelatedLines []int
	RelatedPaths []string // autres champs concernés (labels du modèle de pod, par exemple)
	Insert       []string // lignes à ajouter après la ligne désignée (version corrigée)
	Excerpt      []Line
	Fixed        []Line
	Blocking     bool
}
