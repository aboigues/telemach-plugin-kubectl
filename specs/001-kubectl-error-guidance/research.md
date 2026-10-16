# Recherche (Phase 0) : Guidage pédagogique à partir des erreurs kubectl

**Feature** : `001-kubectl-error-guidance` | **Date** : 2026-10-16

Toutes les versions citées ont été vérifiées le 2026-10-16 sur les sources officielles
(`go.dev/dl`, `proxy.golang.org`, `dl.k8s.io/release/stable-*.txt`, GitHub Releases),
conformément au principe III. Le projet frère `../telemach-plugin-docker` sert de référence :
les décisions qui y ont été validées sont reprises quand elles s'appliquent, et adaptées
quand Kubernetes l'impose.

Environnement de développement constaté : WSL2, bash 5.2.21, zsh 5.9, `kubectl` v1.32.3 dans
`/usr/local/bin`, **et** `alias kubectl='microk8s kubectl'` dans le zsh de l'utilisateur
(microk8s arrêté). `kind` n'est pas installé.

---

## R1. Forme du livrable et langage

- **Decision** : plugin `kubectl` écrit en **Go 1.27.2**, binaire statique unique
  `kubectl-telemach` (`CGO_ENABLED=0`), placé dans un dossier du `PATH` et appelé par
  `kubectl telemach …` (mécanisme standard des plugins kubectl : tout exécutable
  `kubectl-<nom>` du `PATH`), ou directement par l'intégration shell.
- **Rationale** : même choix que le projet Docker (binaire autonome, démarrage immédiat pour
  SC-003, aucun interpréteur chez l'apprenant). Les plugins kubectl n'ont pas de protocole de
  métadonnées à implémenter, contrairement aux plugins Docker CLI.
- **Alternatives considered** : script shell (impossible d'embarquer catalogue et schémas
  proprement, tests difficiles) ; Python (interpréteur requis).
- **Distribution** : hors périmètre (feature ultérieure, comme la 002 du projet Docker ;
  Krew v0.5.0 sera alors évalué).

## R2. Options placées avant le nom du plugin (FR-002, leçon LL-014 du projet Docker)

- **Constat** (lecture de `staging/src/k8s.io/kubectl/pkg/cmd/plugin.go`, v1.37.1,
  `HandlePluginCommand`) : contrairement à Docker, `kubectl` **refuse** toute option avant le
  nom d'un plugin : `kubectl -n demo telemach diagnose` → `Error: flags cannot be placed
  before plugin name: -n`. Les options placées après sont transmises telles quelles au
  binaire.
- **Decision** :
  - `kubectl telemach diagnose -n demo --context kind-dev` est la forme documentée ;
  - l'erreur `flags cannot be placed before plugin name` devient une **entrée du catalogue**
    (elle est captée par l'intégration shell comme toute erreur `kubectl`) : elle explique la
    règle et donne la commande réordonnée ;
  - pour les commandes `kubectl` captées, `internal/command` reconnaît les options globales
    (`-n`, `--namespace`, `--context`, `--kubeconfig`, `--cluster`, `--user`, `-s`/`--server`,
    `--token`, `--as`, `--as-group`, `--request-timeout`, `-v`…) **où qu'elles soient**, avant
    ou après la sous-commande, avec ou sans `=` ;
  - test obligatoire : appel réel de `kubectl telemach …` via un vrai `kubectl` (pas seulement
    le binaire en direct), car c'est précisément ce qui manquait dans LL-014.

## R3. Capture automatique des erreurs (FR-001, FR-001a, FR-021)

- **Decision** : intégration shell générée par `kubectl telemach shell-init bash|zsh`, activée
  par `eval "$(kubectl-telemach shell-init zsh)"` dans le fichier rc (appel direct du binaire :
  fonctionne même quand `kubectl` est un alias confiné comme `microk8s kubectl`, voir R4). Elle
  définit une fonction `__telemach_kubectl` qui :
  1. exécute la commande `kubectl` réelle (R4) sans toucher à stdin/stdout ;
  2. duplique stderr (`2> >(tee "$tmp" >&2)`) vers un fichier `mktemp` en `0600` dans
     `$XDG_RUNTIME_DIR` (ou `$TMPDIR`), supprimé par `trap` même après Ctrl-C (LL-003) ;
  3. si le code de sortie est non nul, appelle
     `kubectl-telemach explain --exit-code "$rc" --stderr-file "$tmp" [--kubectl-version v]
     [--invoked-as k] -- "$@"` ;
  4. après un succès, lance la vérification différée (R11) si la commande est concernée ;
  5. renvoie **toujours** le code de sortie d'origine.
- **Commandes interactives ou longues** (`exec -it`/`-ti`/`-i -t`, `run -it`, `attach`,
  `edit`, `port-forward`, `proxy`, `logs -f`, `get -w`/`--watch`, `debug -it`, `cp` vers un
  TTY) : stderr n'est pas redirigé ; seul le code de sortie est transmis (`--interactive`).
- **Garde-fous** : `TELEMACH_DISABLE=1` ou binaire absent → comportement strictement identique
  à `kubectl` ; aucun `eval` du contenu capté ; stderr capté limité à 1 Mio.
- **Version du client (FR-001b)** : lue **une fois**, à l'évaluation du script, par
  `<kubectl réel> version --client -o json` (purement local, ne contacte pas le serveur API ;
  mesuré : 83 ms). Échec (par exemple microk8s arrêté : « microk8s is not running ») → version
  inconnue, l'analyse continue sans elle.
- **Alternatives considered** : `preexec`/`precmd` seuls (pas d'accès à stderr) ; remplacer
  `kubectl` dans le `PATH` (intrusif, casse les mises à jour et les outils qui appellent
  `kubectl`) ; option `--v`/journal de kubectl (n'existe pas pour ce besoin).

## R4. Quand `kubectl` ou `k` est un alias (FR-001a, clarification Q5)

- **Constat** : chez l'utilisateur, `kubectl` est un alias (`microk8s kubectl`). Définir une
  fonction `kubectl()` alors qu'un alias du même nom existe échoue en zsh (l'alias est
  développé dans la définition) et masquerait l'alias en bash. Les distributions
  d'apprentissage le font souvent (`microk8s kubectl`, `minikube kubectl --`).
- **Decision** : à l'évaluation de `shell-init`, le script détermine la **commande kubectl
  réelle** :
  - si `kubectl` est un alias, son texte est mémorisé dans le tableau
    `__telemach_kubectl_cmd` (découpé en mots par le shell, sans `eval`), puis l'alias est
    redéfini en `kubectl=__telemach_kubectl` : le comportement visible est inchangé (même
    commande exécutée), seule l'interception s'ajoute ;
  - sinon, `__telemach_kubectl_cmd=(command kubectl)` et une fonction `kubectl` appelle
    `__telemach_kubectl`.
  - **Alias `k`** : s'il existe et que son premier mot est `kubectl`, il est couvert sans
    modification (le shell développe la chaîne d'alias, puis la fonction ou l'alias
    `kubectl`) ; pour que les corrections reprennent la forme `k …`, `k` est redéfini en
    `k='__telemach_kubectl_as k'` en conservant ses options. S'il désigne un chemin absolu
    vers un binaire `kubectl`, il est traité de même. S'il n'existe pas ou désigne autre chose,
    il n'est **ni créé ni modifié** (FR-001a).
  - La complétion existante (`complete -F __start_kubectl k`, `compdef`) n'est pas touchée.
- **Plugins et kubectl confiné** : `microk8s kubectl` s'exécute dans un snap et peut ne pas
  voir `kubectl-telemach` dans le `PATH` ; c'est pourquoi l'intégration appelle toujours le
  binaire en direct. La documentation l'indique.
- **Rationale** : couvre le cas réel de l'utilisateur et les alias courants, sans jamais
  changer la commande exécutée.

## R5. Reconnaissance des erreurs : catalogue déclaratif (FR-002, FR-005 à FR-010)

- **Decision** : comme le projet Docker, un **catalogue** YAML en français embarqué
  (`go:embed`), un fichier par catégorie (`cmdline`, `connection`, `auth`, `notfound`,
  `namespace`, `manifest`, `schema`, `conflict`, `workload`, `runtime`), motifs RE2 avec groupes
  nommés, validé par des tests (schéma, unicité, ≥ 1 cas de corpus par entrée, exemples
  conformes aux bonnes pratiques).
- **Familles de messages kubectl à couvrir** (relevées dans les sources v1.37.1 et à confirmer
  sur le corpus réel, R13) :
  - ligne de commande : `error: unknown command "gt" for "kubectl"` + `Did you mean this?`,
    `error: unknown flag: --foo`, `error: unknown shorthand flag`, `error: the server doesn't
    have a resource type "pdos"`, `error: required resource not specified`,
    `flags cannot be placed before plugin name` ;
  - connexion : `The connection to the server … was refused - did you specify the right host
    or port?`, `Unable to connect to the server: dial tcp …`, `error: current-context is not
    set`, `context "x" does not exist`, certificats (`x509: …`) ;
  - authentification et RBAC : `error: You must be logged in to the server (Unauthorized)`,
    `Error from server (Forbidden): … User "u" cannot <verbe> resource "r" … in the namespace
    "n"` ;
  - ressources : `Error from server (NotFound): pods "web" not found`,
    `namespaces "x" not found`, `Error from server (AlreadyExists)`, `(Conflict)` ;
  - manifestes : `error converting YAML to JSON: yaml: line N: …`, `strict decoding error:
    unknown field "spec.template.spec.containers[0].imagePullPolicyy"`, `The Deployment "web"
    is invalid: spec.template.metadata.labels: Invalid value: … selector does not match
    template labels`, `no matches for kind "Deployment" in version "extensions/v1beta1"`,
    `error: error parsing deploy.yaml`, `error validating data`.
- **Suggestions de forme proche (FR-011)** : table des types de ressources de base et de leurs
  noms courts (`po`, `deploy`, `svc`, `cm`, `ns`, `ing`, `pvc`, `sts`, `ds`, `cj`…) et des
  sous-commandes/options de kubectl, embarquée ; distance de Levenshtein (paquet interne
  `textdist`, repris du projet Docker). La suggestion de kubectl lui-même (`Did you mean
  this?`) est réutilisée quand elle existe.
- **Origine (FR-004)** : `kubectl` renvoie 1 pour presque toutes ses erreurs : l'origine se
  déduit du **texte** (préfixes `error:`/`Error from server (…)` = kubectl ou serveur API) et de
  la sous-commande ; pour `exec`/`run --rm -it`, un code ≠ 1 sans message kubectl est le code
  de la commande du conteneur → origine `application`, orientation vers `kubectl logs`.

## R6. Analyse des manifestes et localisation à la ligne (FR-013 à FR-016)

- **Decision** : `go.yaml.in/yaml/v3` **v3.0.5** (API `Node`, ligne et colonne, documents
  multiples via `Decoder`) pour :
  1. **syntaxe** : tabulations, YAML mal formé ; la ligne de l'erreur yaml (`yaml: line N`)
     est rapportée au fichier entier (kubectl la donne relative au **document** : décalage
     ajouté d'après la position du séparateur `---`) ;
  2. **indexation** : pour chaque document, `kind`, `metadata.name`, ligne de début, et une
     table *chemin de champ → nœud* (`spec.template.spec.containers[0].image`) ;
  3. **traduction** d'un chemin de champ issu d'un message kubectl (`strict decoding error`,
     `is invalid: <chemin>`) en ligne et colonne, en retrouvant le document par
     `kind`/`metadata.name` cités dans le message ou, à défaut, par le fichier cité.
- **Fichiers relus** : uniquement ceux passés par `-f` (fichiers, dossiers, `-R`), en lecture
  seule, 1 Mio max. par fichier, extensions `.yaml`, `.yml`, `.json` comme kubectl ; `-f -`,
  URL et `-k` ne sont pas relus (edge cases).
- **Alternatives considered** : `sigs.k8s.io/yaml` (convertit en JSON et perd les positions) ;
  `github.com/goccy/go-yaml` (troisième parseur sans gain décisif).

## R7. Règles de schéma hors ligne pour trois versions (FR-014, FR-014a, clarification Q3)

- **Versions maintenues au 2026-10-16** : `stable-1.37` = **v1.37.1**, `stable-1.36` =
  **v1.36.5**, `stable-1.35` = **v1.35.9** (1.34 est la quatrième, hors plage).
- **Decision** : embarquer, pour chacune, un **sous-ensemble** des documents OpenAPI v3
  publiés dans le dépôt Kubernetes (`api/openapi-spec/v3/api__v1_openapi.json`,
  `apis__apps__v1_…`, `apis__batch__v1_…`, `apis__networking.k8s.io__v1_…`, vérifiés présents
  pour v1.37.1) :
  - un générateur (`scripts/gen-schemas`, exécuté par le mainteneur, jamais à l'exécution)
    télécharge ces fichiers au tag exact, vérifie leur somme SHA-256 consignée, ne garde que
    la fermeture transitive des types des ressources de FR-014, retire les descriptions, et
    écrit `internal/schema/data/v1.3X.json.gz` ;
  - un **validateur interne** parcourt le nœud YAML et le schéma en parallèle : `type`,
    `properties`, `required`, `items`, `additionalProperties`, `$ref`/`allOf`, `enum` ;
    `x-kubernetes-int-or-string` et `x-kubernetes-preserve-unknown-fields` respectés. Champ
    inconnu → suggestion par distance d'édition parmi les propriétés **du même niveau**, puis
    recherche du même nom **un niveau au-dessus ou au-dessous** (« mauvais niveau
    d'indentation »).
  - vérifié sur v1.37.1 : `PodSpec.required = [containers]`, `DeploymentSpec.required =
    [selector, template]`, `Container.imagePullPolicy` de type `string`.
- **Choix de la version** : version mineure de `--kubectl-version` si elle vaut 1.35, 1.36 ou
  1.37 ; sinon (par exemple v1.32.3 sur la machine de développement) la plus récente, avec la
  mention prévue par FR-014a. Le cluster n'est jamais interrogé.
- **Règles hors schéma** (catalogue, `internal/manifest/semantic.go`) : selector/labels d'un
  `Deployment`/`StatefulSet`/`DaemonSet`/`Job`, `apiVersion` retirées (table
  `extensions/v1beta1`, `apps/v1beta1`, `apps/v1beta2` → `apps/v1`, `batch/v1beta1` CronJob →
  `batch/v1`, `networking.k8s.io/v1beta1` Ingress → `networking.k8s.io/v1`…), noms DNS-1123
  invalides, CRD inconnues (structure commune seulement).
- **Rationale** : fidélité aux règles réelles de chaque version, aucune dépendance lourde,
  positions préservées (le schéma est parcouru sur le nœud YAML).
- **Alternatives considered** : `k8s.io/api` + décodage strict (une seule version, celle du
  module ; pas de position) ; `kubeconform` et schémas JSON tiers (dépendance et schémas
  externes, positions perdues) ; `github.com/santhosh-tekuri/jsonschema/v6` sur l'OpenAPI
  converti (positions à reconstruire, conversion fragile des extensions `x-kubernetes-*`).
- **Taille** : à mesurer par le générateur ; objectif < 2 Mio compressés pour les trois
  versions (les documents complets font 0,5 à 2,1 Mo non compressés chacun).

## R8. Lecture du cluster : `diagnose` et vérification différée (FR-022 à FR-029)

- **Decision** : appeler la **commande kubectl réelle de l'apprenant** (R4, transmise par
  `TELEMACH_KUBECTL`, découpée en mots sans shell ; à défaut `kubectl` du `PATH`) via `os/exec`
  avec une liste d'arguments, au travers d'un seul paquet `internal/kuberead` qui n'expose que
  les lectures suivantes (liste fermée vérifiée par un test avec un faux `kubectl` qui
  journalise ses appels) :

  | Lecture | Usage |
  |---|---|
  | `get pods -o json` | candidats de `diagnose` sans argument (namespace courant ou `-n`) |
  | `get pod <nom> -o json` | `diagnose pod/<nom>` |
  | `get deployment <nom> -o json` puis `get pods -l <selector> -o json` | `diagnose deployment/<nom>` |
  | `get events --field-selector involvedObject.kind=Pod,involvedObject.name=<nom> -o json` | causes `Pending`, `ImagePullBackOff`, sondes |
  | `logs <pod> -c <conteneur> [--previous] --tail=20` | journal d'un conteneur en échec |
  | `get pods -l <selector> -o json` / `get pod <nom> -o json` | vérification différée (R11) |

  Toujours suivies des options de l'apprenant `--context`, `-n`, `--kubeconfig` (et
  `--request-timeout=10s`) ; jamais `-A` implicite, jamais `secrets`, jamais d'autre verbe que
  `get` et `logs`.
- **Rationale** : la commande de l'apprenant applique d'elle-même le kubeconfig, le contexte,
  les plugins d'authentification `exec` (cloud) et le TLS : TELEMACH lit exactement le même
  cluster, avec les mêmes droits (FR-024), sans réimplémenter cette logique ni ajouter
  `client-go` (dépendance très lourde, surface d'attaque).
- **Valeurs sensibles** : le JSON d'un pod contient les valeurs de `env[].value` et des
  annotations (dont `kubectl.kubernetes.io/last-applied-configuration`, qui recopie tout le
  manifeste) : le décodage utilise des types dédiés qui **ne lisent pas** ces champs
  (`env` réduit aux noms, annotations ignorées) ; journal et événements passent par le masquage
  (R9) avant tout usage.
- **Erreurs de lecture** : `Forbidden` → lecture marquée refusée, diagnostic partiel
  (FR-026) ; connexion refusée → explication du catalogue (US1), sans retentative.
- **Alternatives considered** : `client-go`/`k8s.io/cli-runtime` (cf. ci-dessus) ; appels HTTP
  directs à l'API (authentification et contexte à refaire).

## R9. Masquage des secrets (FR-018)

- **Decision** : module `redact` repris du projet Docker et étendu à Kubernetes :
  - options de ligne de commande `--token`, `--password`, `--client-key`, `--client-certificate`
    (chemins conservés, contenus jamais lus), `--from-literal=<clé>=<valeur>` (valeur masquée),
    `--docker-password`, `--as` conservé ;
  - manifestes : tout le contenu de `data` et `stringData` d'un `kind: Secret` ; `env[].value`
    dont le nom contient `PASSWORD`, `PASSWD`, `SECRET`, `TOKEN`, `KEY`, `CREDENTIAL`, `AUTH`,
    `CERT` ; blocs PEM (`-----BEGIN … -----END …`) ; chaînes base64 longues dans `data` ;
  - messages : en-têtes `Authorization: Bearer …`, jetons de compte de service (JWT `eyJ…`),
    contenu de kubeconfig (`client-key-data`, `token:`), identifiants dans les URL.
  - valeur remplacée par `••••••` ; appliqué à toute chaîne avant affichage **ou** écriture.
- **Rationale** : défense en profondeur ; corpus dédié `testdata/secrets/` (SC-006) et fuzzing.

## R10. Diagnostic d'un pod (FR-022, FR-025, FR-026)

- **Decision** : sélection sans argument = pods du namespace dont une condition `Ready` est
  fausse **et** qui présentent un état d'échec (raison d'attente `ImagePullBackOff`,
  `ErrImagePull`, `CrashLoopBackOff`, `CreateContainerConfigError`, `InvalidImageName`, raison
  de fin `OOMKilled`/`Error` avec code, `Pending` + condition `PodScheduled=False`, ou sonde en
  échec d'après les événements) ; tri par heure de création décroissante ; le plus récent est
  expliqué, les autres listés (au plus 10) avec `kubectl telemach diagnose pod/<nom>`.
- **Sources de cause**, de la plus précise à la plus générale : raison et message du statut
  (`state.waiting.message`, par exemple « manifest unknown »), événements (`FailedScheduling:
  0/1 nodes are available: 1 Insufficient memory`, `Unhealthy: Readiness probe failed`), fin
  du journal précédent (motifs du catalogue `runtime`), code de sortie (1, 2, 126, 127, 137,
  139, 143) ; aucune cause inventée (FR-026).
- **Pods sains** : un pod `Running`/`Ready` ou `Succeeded` n'est jamais diagnostiqué ;
  `ContainerCreating` et `Pending` sans événement d'échec = « en cours de création ».
- **Performance** : 3 à 4 lectures de ~100 à 300 ms chacune → < 5 s (SC-009), à mesurer sur
  kind.

## R11. Vérification différée après une création réussie (FR-028 à FR-031)

- **Decision** (reprise de R6 de la feature 004 du projet Docker) :
  - après un `apply -f`/`create -f`/`create deployment`/`run` **réussi**, sans `--dry-run`,
    dans un shell **interactif**, si `TELEMACH_WATCH` ≠ `0` et `TELEMACH_DISABLE` ≠ `1` :
    `command kubectl-telemach watch --shell-pid $$ -- "$@" </dev/null >/dev/null 2>&1 &`
    depuis la fonction (sous-shell : aucun message de tâche) ;
  - `watch` (Go) identifie les cibles **sans lire le cluster** : pour `-f`, `kind` `Pod` et
    `Deployment` des manifestes locaux (`spec.selector.matchLabels`) ; pour
    `create deployment <nom>`, le label `app=<nom>` posé par kubectl ; pour `run <nom>`, le pod
    `<nom>` ; puis attend **10 s**, lit **uniquement** le statut de ces pods (R8, dernière
    ligne du tableau), et, pour chaque pod en échec (FR-025), ajoute une ligne à
    `$XDG_RUNTIME_DIR/telemach-kubectl/pending-<pid du shell>` (dossier `0700`, fichier `0600`) si le
    shell est toujours vivant ;
  - un crochet d'invite (`precmd` via `add-zsh-hook`, `PROMPT_COMMAND` en bash, ajout
    idempotent) affiche puis supprime ce fichier ; s'il n'existe pas, aucun processus n'est
    lancé ;
  - les fichiers de shells disparus sont supprimés au chargement suivant de `shell-init`.
- **Délai de 10 s** : un `ImagePullBackOff` apparaît après le premier échec de téléchargement
  (quelques secondes), un `CrashLoopBackOff` après le premier redémarrage ; à confirmer sur
  kind (R13), ajustable par une constante.
- **Rationale** : affichage seulement à l'invite (jamais au milieu d'une saisie) ; lectures
  minimales (pas d'événements ni de journaux, FR-029).

## R12. Interface, rendu et état local

- **CLI** : `github.com/spf13/cobra` **v1.10.2** (bibliothèque de kubectl : ergonomie
  cohérente).
- **Rendu** : `charm.land/lipgloss/v2` **v2.0.6**, `github.com/charmbracelet/colorprofile`
  **v0.4.3**, `github.com/alecthomas/chroma/v2` **v2.27.0** (coloration shell et YAML),
  `golang.org/x/term` **v0.47.0** ; même palette adaptative, mêmes symboles, `--color`,
  `NO_COLOR`, `CLICOLOR_FORCE`, neutralisation des séquences ANSI (paquet `sanitize`) que le
  projet Docker.
- **État** : `$XDG_STATE_HOME/telemach-kubectl/last.json` (dossier `0700`, fichier `0600`,
  contenu déjà masqué) pour `last`, `deepen` et la fenêtre de répétition de 10 minutes
  (empreinte SHA-256 de la commande et de l'erreur normalisées). Dossier distinct de celui du
  plugin Docker pour que les deux plugins coexistent.

## R13. Tests et quality gates (principe IV)

- **Unitaires et golden files** : `testing` + `github.com/google/go-cmp` **v0.7.0** ; rendus
  `--color=never` et `--color=always`.
- **Corpus** : `testdata/errors/` (≥ 30 erreurs réelles relevées sur kind pour 1.35, 1.36 et
  1.37, SC-001), `testdata/manifests/` (≥ 20 manifestes erronés avec fichier, ligne et champ
  attendus, SC-002), `testdata/secrets/` (SC-006), `testdata/pods/` (JSON de pods et
  d'événements réels, masqués, pour SC-009 hors cluster).
- **Fuzzing** : matcher, masquage, traduction chemin → ligne.
- **Intégration** (`//go:build integration`, LL-007 : le lint doit couvrir ces fichiers) :
  cluster **kind v0.33.0** jetable avec les images épinglées par digest (notes de version de
  kind v0.33.0) :
  - `kindest/node:v1.35.8@sha256:07b2536e30b803ed61d1677a79df6115f798ce64c80f9e22f6ed45afd09323c0`
  - `kindest/node:v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed`
  - `kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5`

  Chaque test crée son propre
  namespace et vérifie que l'état du cluster est identique avant et après `diagnose`/`watch`
  (SC-008, LL-015) ; jamais le contexte courant de la machine.
- **Shell** : bash et zsh dans un pseudo-terminal : code de sortie, stdout intact, fichier
  temporaire supprimé (y compris Ctrl-C), alias `kubectl` et `k` (R4), crochet d'invite (R11),
  appel réel via `kubectl telemach` (R2) ; aucune sortie dupliquée (LL-002).
- **Gates GitHub Actions** : `golangci-lint` **v2.15.0** (avec `gosec` ; vérifier qu'il est
  construit avec Go ≥ 1.27, LL-001), `go test -race`, `govulncheck` (`golang.org/x/vuln`
  **v1.9.0**), `gitleaks` **v8.30.1**, revue des dépendances (Dependency graph activé, LL-005),
  tests d'intégration kind, actions épinglées par SHA, `permissions: contents: read`.
- **Contrôle `manifest-lint`** (principe IV : manifestes présents dans le dépôt) : un test
  `tests/manifests/` exécuté par un job CI obligatoire dédié vérifie, sur les exemples du
  catalogue, `testdata/manifests/valid/` et `testdata/pods/scenarios/`, (1) la validation par
  les règles embarquées des trois versions (dès que `internal/schema` existe) et (2) les règles
  de sécurité : `runAsNonRoot`, `allowPrivilegeEscalation: false`, `resources`, pas de
  `privileged`/`hostPath`/`hostNetwork`/`hostPID`, avertissement non bloquant pour un tag
  absent ou `latest`. Un scénario ne peut s'écarter de ces règles que pour le défaut qu'il
  déclare dans son `expected.yaml` (`allowed_defects`). Les manifestes d'erreur de
  `testdata/manifests/` (hors `valid/`), volontairement faux, sont exclus.

## R14. Leçons du projet Docker appliquées (principe V)

`LESSON-LEARNED.md` n'existe pas encore dans ce dépôt ; il sera créé par la première tâche,
avec les leçons transposables du projet Docker : LL-001 (version de Go de golangci-lint),
LL-002 (zsh MULTIOS), LL-003 (fichier temporaire et Ctrl-C), LL-005 (Dependency graph),
LL-006 et LL-013 (intégration shell inactive ou périmée : variable
`TELEMACH_KUBECTL_SHELL=<shell>:<version>` et avertissement dans `last`/`deepen`), LL-007
(lint des tests à étiquette), LL-008 (lint bloquant avant commit), LL-009 (vérification
réellement comparée), LL-012 (objectif de performance mesuré par un test), LL-014 (R2),
LL-015 (aucun test sur les ressources de l'utilisateur).
