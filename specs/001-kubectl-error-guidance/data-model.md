# Modèle de données (Phase 1) : Guidage pédagogique à partir des erreurs kubectl

**Feature** : `001-kubectl-error-guidance` | **Date** : 2026-10-16

Les noms d'entités et de champs sont en anglais (code, principe II) ; les descriptions sont en
français. Aucune base de données : les entités vivent en mémoire, sauf le catalogue et les
schémas (embarqués dans le binaire), le dernier diagnostic et les signaux en attente (fichiers
d'état locaux).

---

## CapturedError — erreur kubectl capturée

Ce que l'intégration shell transmet à `kubectl-telemach explain`.

| Champ | Type | Description | Validation |
|-------|------|-------------|------------|
| `Args` | `[]string` | Arguments, sans le `kubectl` initial | Non vide ; **masqués** (R9) avant tout usage hors mémoire |
| `InvokedAs` | `string` | `kubectl` ou `k` (R4) | Défaut `kubectl` ; utilisé pour écrire les corrections |
| `ExitCode` | `int` | Code de sortie | 1 à 255 |
| `Stderr` | `string` | Texte d'erreur capturé | 1 Mio max. (on garde la fin) ; séquences ANSI neutralisées ; vide si `Interactive` |
| `Interactive` | `bool` | Commande interactive ou longue (R3) | — |
| `CapturedAt` | `time.Time` | Horodatage | — |
| `KubectlVersion` | `string` | Version du client (FR-001b) | Optionnel ; `^v?[0-9]+\.[0-9]+\.[0-9]+([-+.][0-9A-Za-z.-]+)?$`, ignorée si invalide |

## Command — commande analysée

Dérivée de `CapturedError.Args` (`internal/command`).

| Champ | Type | Description |
|-------|------|-------------|
| `Subcommand` | `[]string` | `get`, `apply`, `config use-context`, `rollout status`, `set image`… (FR-003) |
| `ResourceType` | `string` | Type tel que tapé (`pdos`, `deploy`, `pod/web` → `pod`) |
| `ResourceNames` | `[]string` | Noms de ressources visés |
| `Namespace`, `Context`, `Kubeconfig` | `string` | Options globales, **où qu'elles soient placées** (R2) ; vides si absentes |
| `Files` | `[]string` | Valeurs de `-f`/`--filename` (fichiers, dossiers ; `-` et URL marqués non relisibles) |
| `Recursive`, `DryRun` | `bool` | `-R`, `--dry-run=client|server` |
| `Flags` | `map[string]string` | Autres options, valeurs sensibles masquées |
| `Interactive` | `bool` | Voir R3 |

## CatalogEntry — entrée du catalogue d'erreurs

Fichiers YAML embarqués (`internal/catalog/data/*.yaml`), même structure que le projet Docker.

| Champ | Type | Description | Validation |
|-------|------|-------------|------------|
| `ID` | `string` | ex. `cmdline.unknown-resource-type`, `pod.image-pull-backoff` | Unique ; `^[a-z]+(\.[a-z0-9-]+)+$` |
| `Category` | `enum` | `cmdline`, `connection`, `auth`, `notfound`, `namespace`, `manifest`, `schema`, `conflict`, `workload`, `runtime` | Obligatoire |
| `Applies` | `[]string` | Sous-commandes concernées (vide = toutes) | — |
| `Patterns` | `[]string` | RE2 avec groupes nommés | ≥ 1 sauf entrée `Fallback` ou `PodReason` |
| `PodReasons` | `[]string` | Raisons de statut couvertes (`CrashLoopBackOff`, `OOMKilled`…) pour `diagnose` | Optionnel |
| `ExitCodes` | `[]int` | Codes de conteneur couverts (126, 127, 137…) | Optionnel |
| `Fallback` | `string` | `application`, `manifest-issue` | Optionnel |
| `Concept` | `ConceptRef` | Concept Kubernetes enseigné | Obligatoire |
| `Summary`, `Cause` | `string` (fr) | Résumé (≤ 160 car.) et cause probable | Obligatoires |
| `Fixes` | `[]Fix` | Corrections | ≥ 1 (FR-005) |
| `AntiPattern` | `*AntiPatternNote` | ex. « s'attribuer cluster-admin », `privileged: true` (FR-010) | Optionnel |
| `Deepen` | `Deepening` | `Explanation`, `Example`, `Exercise` (namespace d'exercice créé par l'apprenant) | Obligatoire |
| `DocURL` | `string` | `https://kubernetes.io/docs/…` | Obligatoire |
| `Tests` | `[]string` | Cas de `testdata/` qui doivent correspondre | ≥ 1 |

**Fix** : `Description` (fr), `Command` (gabarit, ex. `{{.kubectl}} get {{.suggested_type}}
{{.rest}}` — `{{.kubectl}}` vaut `kubectl` ou `k`), `Manifest` (extrait YAML facultatif),
`Safe` (`bool`). Les exemples de manifeste respectent les règles de `safety_test` (R13).

## ManifestFile et ManifestDocument — manifeste relu

| Champ | Type | Description |
|-------|------|-------------|
| `Path` | `string` | Chemin tel que passé ou trouvé dans le dossier |
| `Documents` | `[]ManifestDocument` | Documents séparés par `---` |
| `Readable` | `bool` + `ReadError` | Introuvable, droits, non UTF-8, > 1 Mio |

`ManifestDocument` : `Index` (1-based), `StartLine`, `APIVersion`, `Kind`, `Name`,
`Namespace`, `Root` (`*yaml.Node`), `Paths` (`map[string]*yaml.Node`, chemin de champ →
nœud), `SensitiveValues` (positions des valeurs masquées).

## ManifestIssue — problème localisé

| Champ | Type | Description |
|-------|------|-------------|
| `Kind` | `enum` | `yaml-syntax`, `tab-indent`, `missing-api-version`, `missing-kind`, `unknown-kind`, `removed-api-version`, `unknown-field`, `wrong-level`, `wrong-type`, `required-missing`, `invalid-enum`, `selector-mismatch`, `invalid-name`, `plaintext-secret`, `unknown-crd`, `file-not-found`, `file-unreadable` |
| `File`, `Document`, `Line`, `Column` | `string`, `int`… | Position dans le **fichier** (1-based ; 0 si inconnue) |
| `Resource` | `string` | `Deployment/web` |
| `FieldPath` | `string` | `spec.template.spec.containers[0].imagePullPolicyy` |
| `Suggestion` | `string` | Champ, niveau ou `apiVersion` proposé |
| `RelatedLines` | `[]int` | Autres lignes concernées (ex. labels du template pour `selector-mismatch`) |
| `Excerpt`, `Fixed` | `[]Line` | Extrait masqué et version corrigée (FR-016) |
| `Blocking` | `bool` | Erreur de syntaxe qui empêche le reste de l'analyse |
| `RulesVersion` | `string` | Version de règles appliquée (`1.37`), indiquée par `check` (FR-014a) |

Règles : tri par fichier, document, ligne ; erreurs bloquantes en premier.

## SchemaSet — règles embarquées (R7)

| Champ | Type | Description |
|-------|------|-------------|
| `Version` | `string` | `1.35`, `1.36`, `1.37` |
| `Source` | `string` | Tag Kubernetes et sommes SHA-256 des documents d'origine |
| `Kinds` | `map[GVK]SchemaRef` | Ressources couvertes (FR-014) |
| `RemovedAPIs` | `map[GVK]GVK` | `apiVersion` retirées → remplaçantes |

Sélection : mineure de `KubectlVersion` si dans la plage, sinon la plus récente
(`RulesFallback = true`, mentionné à l'apprenant). Ne pas confondre `RulesVersion` (version
des règles Kubernetes) avec `LastDiagnosis.SchemaVersion` (version du format de `last.json`).

## PodSnapshot — ressource diagnostiquée (FR-022 à FR-026)

Construit à partir des lectures de `internal/kuberead` (R8).

| Champ | Type | Description |
|-------|------|-------------|
| `Context`, `Namespace`, `Name`, `Owner` | `string` | `Owner` = `Deployment/web` si applicable |
| `CreatedAt` | `time.Time` | Tri du plus récent |
| `Phase`, `Ready` | `string`, `bool` | — |
| `Containers` | `[]ContainerState` | Nom, image, `Reason` (attente ou fin), `Message`, `ExitCode`, `RestartCount`, noms des variables (**jamais les valeurs**), références `Secret`/`ConfigMap` |
| `Conditions` | `[]Condition` | `PodScheduled`, `Ready`… |
| `Events` | `[]Event` | Raison, message masqué, nombre, dernière occurrence |
| `LogTail` | `[]string` | ≤ 20 lignes, masquées et neutralisées |
| `DeniedReads` | `[]string` | Lectures refusées par RBAC → diagnostic partiel |

**États** : `healthy` → ignoré ; `starting` (`ContainerCreating`, `Pending` sans événement
d'échec) → « en cours de création » ; `failing` (raisons de FR-025) → diagnostiqué.

## Diagnosis — résultat de l'analyse

| Champ | Type | Description |
|-------|------|-------------|
| `Source` | `enum` | `command` (erreur captée), `check`, `pod` |
| `Confidence` | `enum` | `recognized`, `partial`, `unknown` (FR-008, FR-026) |
| `Origin` | `enum` | `kubectl`, `cluster`, `application`, `unknown` (FR-004) |
| `Entry` | `*CatalogEntry` | — |
| `Captures` | `map[string]string` | Valeurs masquées |
| `ManifestIssues` | `[]ManifestIssue` | — |
| `Pod` | `*PodSnapshot` | Pour `diagnose` |
| `OtherFailingPods` | `[]PodSummary` | Nom, état — pour la liste courte (FR-022) |
| `Fingerprint` | `string` | SHA-256 de la commande et de l'erreur normalisées |

**Transitions** : `CapturedError` → `Command` → (catalogue, manifestes, schémas) `Diagnosis` →
`Explanation` ; `diagnose` : lectures → `PodSnapshot` → `Diagnosis` → `Explanation`.

## Explanation — explication rendue

`Summary`, `Concept`, `Cause`, `Fixes`, `AntiPattern`, `OriginalError` (masqué et neutralisé,
atténué avec fragments surlignés, FR-007), `Repeated` (rendu court, FR-009b),
`DeepenAvailable`, `KubectlVersion`, `RulesVersion`, `Partial` (lectures refusées).

## LastDiagnosis — état persistant

`$XDG_STATE_HOME/telemach-kubectl/last.json` (dossier `0700`, fichier `0600`), écrasé à
chaque diagnostic. Champs : `SchemaVersion` (`1`), `Fingerprint`, `EntryID`, `Captures`,
`RecordedAt`, `Source`, `Origin`, `Confidence`, `ExitCode`, `Interactive`, `InvokedAs`,
`KubectlVersion`, `Stderr` (≤ 64 Kio, déjà masqué), `Subcommand`, `Files` (manifestes à
réanalyser par `last`), `Pod` (`PodSnapshot` déjà masqué : `last` ne relit pas le cluster).

Invariant : aucune donnée non masquée n'est écrite sur disque ; un fichier illisible ou d'un
autre schéma est ignoré et réécrit sans message.

## PendingSignal — signal de la vérification différée (FR-028 à FR-031)

Fichier `$XDG_RUNTIME_DIR/telemach-kubectl/pending-<pid du shell>` (dossier `0700`, fichier
`0600`), une ligne par pod en échec, ajout atomique ; lu puis supprimé par le crochet d'invite.

| Champ | Description |
|-------|-------------|
| `Pod`, `Namespace`, `Context` | Pod signalé |
| `Reason` | État (`ImagePullBackOff`…) |
| `Command` | `kubectl telemach diagnose pod/<nom> [-n …] [--context …]` à copier |

Règle : écrit seulement si le shell `<pid>` est vivant ; fichiers de shells disparus supprimés
au chargement de `shell-init`.
