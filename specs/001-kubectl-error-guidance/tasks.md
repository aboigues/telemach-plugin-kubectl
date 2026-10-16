---

description: "Liste des tâches pour la feature 001-kubectl-error-guidance"
---

# Tasks: Guidage pédagogique à partir des erreurs kubectl

**Input**: Design documents from `/specs/001-kubectl-error-guidance/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli.md, quickstart.md

**Tests**: inclus. Les critères SC-001, SC-002, SC-006, SC-008, SC-009 et SC-010 exigent des
corpus et des mesures, et le principe IV de la constitution exige des tests bloquants en CI.
Dans chaque histoire, les tests sont écrits **avant** l'implémentation et doivent d'abord
échouer.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Conventions du projet (à respecter dans chaque tâche)

- Projet Go unique : `cmd/kubectl-telemach/`, `internal/…`, `scripts/`, `testdata/`, `tests/`
  (voir plan.md). Le projet `../telemach-plugin-docker` sert de modèle : relire le fichier
  équivalent avant d'écrire un paquet repris (`sanitize`, `redact`, `textdist`, `render`,
  `state`, `catalog`, `diagnose`, `shellinit`).
- **Code et identifiants en anglais ; commentaires, messages, catalogue et documentation en
  français**, accents compris (principe II).
- Relire `LESSON-LEARNED.md` avant chaque phase (principe V).
- Toute dépendance, action GitHub ou image ajoutée : vérifier la **dernière version stable
  réelle** au moment de l'ajout (versions de research.md vérifiées le 2026-10-16).
- Toute chaîne affichée ou écrite sur disque passe par `sanitize` puis `redact`.
- **Aucun test ne touche le contexte kubectl courant de la machine** : tests d'intégration sur
  un cluster kind jetable uniquement (LL-015, principe III).
- Toute interaction avec un cluster passe par `internal/kuberead` (liste fermée, R8).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: initialiser le dépôt et poser les gates CI **avant** tout code (principe IV)

- [X] T001 Initialiser le dépôt git (`git init -b main`, premier commit sur `main` avec la documentation existante : `.specify/`, `.claude/`, `specs/`), puis créer la branche `001-kubectl-error-guidance`. Créer `.gitignore` à la racine (binaire `kubectl-telemach`, `dist/`, `*.out`, `.idea/`, `.vscode/`, `kubeconfig*` hors `testdata/`)
- [X] T002 Créer `LESSON-LEARNED.md` à la racine (français), avec la même structure que `../telemach-plugin-docker/LESSON-LEARNED.md` (règles d'usage, modèle d'entrée : date, catégorie, contexte, cause racine, impact, correctif, règle préventive). Section « Leçons héritées du projet Docker » : une ligne de règle préventive par leçon de research.md R14 (LL-001, 002, 003, 005, 006, 007, 008, 009, 012, 013, 014, 015), avec le lien vers l'entrée d'origine. Aucun incident propre au projet pour l'instant
- [X] T003 Créer `go.mod` : `module github.com/aboigues/telemach-plugin-kubectl`, `go 1.27`, `toolchain go1.27.2`. Dépendances directes (research.md R12, R13) : `github.com/spf13/cobra` v1.10.2, `go.yaml.in/yaml/v3` v3.0.5, `charm.land/lipgloss/v2` v2.0.6, `github.com/charmbracelet/colorprofile` v0.4.3, `github.com/alecthomas/chroma/v2` v2.27.0, `golang.org/x/term` v0.47.0, `github.com/google/go-cmp` v0.7.0. **Aucune** dépendance `k8s.io/*` ni `client-go`
- [X] T004 Créer l'arborescence du plan : `cmd/kubectl-telemach/`, `internal/{shellinit/scripts,command,kubeterms,catalog/data,diagnose,manifest,schema/data,kuberead,workload,redact,sanitize,textdist,render,state}/`, `scripts/gen-schemas/`, `testdata/{errors,manifests,pods,secrets,golden}/`, `tests/{integration,shell,manifests}/`, `scripts/collect-errors/`, avec un `doc.go` par paquet (commentaire de paquet en français)
- [X] T005 [P] Créer `.golangci.yml` (format v2, golangci-lint v2.15.0) : `govet`, `staticcheck`, `errcheck`, `gosec`, `ineffassign`, `unused`, `revive`, `gocritic`, `bodyclose`, `misspell` (identifiants seulement), formateur `gofumpt` ; `run.build-tags: [integration]` pour linter aussi les tests d'intégration (LL-007). Vérifier que le binaire v2.15.0 est construit avec Go ≥ 1.27 (LL-001) ; sinon l'installer avec `go install` dans la CI
- [X] T006 [P] Créer `.github/workflows/ci.yml` (`pull_request` et `push` sur `main`, `permissions: contents: read`). Jobs : `lint` (golangci-lint v2.15.0), `test` (`go test -race -cover ./...`, `go-version-file: go.mod`, bash et zsh installés), `vuln` (`govulncheck`, `golang.org/x/vuln` v1.9.0), `secrets` (gitleaks v8.30.1), `dependency-review` (sur `pull_request`), `integration` (kind v0.33.0, matrice des trois images `kindest/node` de research.md R13 **épinglées par digest complet**, `go test -tags integration ./tests/integration/...`), `manifest-lint` (`go test ./tests/manifests/...`, voir T020 ; réussit s'il n'y a encore aucun manifeste, principe IV). Chaque action épinglée par **SHA de commit complet** avec la version en commentaire (rechercher les SHA au moment de l'écriture) ; `pull_request_target` interdit
- [X] T007 [P] Créer `.github/dependabot.yml` : écosystèmes `gomod` et `github-actions`, hebdomadaire, mises à jour de sécurité groupées
- [X] T008 [P] Créer `.gitleaks.toml` qui étend la configuration par défaut et n'autorise que `testdata/secrets/` et `testdata/pods/` (faux secrets volontaires)
- [X] T009 [P] Créer `SECURITY.md` (français) : signalement responsable, versions supportées, délai de réponse ; préciser que le plugin lit le cluster avec les droits de l'utilisateur et la liste exacte des lectures (contracts/cli.md)
- [X] T010 [P] Créer `CONTRIBUTING.md` (français) : règles de langue, relecture de `LESSON-LEARNED.md`, gates requises (`lint`, `test`, `vuln`, `secrets`, `dependency-review`, `integration`, `manifest-lint`), interdiction de tester sur un cluster autre que kind jetable, règles de la protection de `main` (T011)
- [X] T011 Créer le dépôt GitHub **public** `aboigues/telemach-plugin-kubectl` (il n'existe pas au 2026-10-16) et protéger `main`, sur le modèle de T010 du projet Docker. **Action visible de l'extérieur : demander confirmation à l'utilisateur avant chaque étape.** Dépend de T001 et T006.
  1. `gh repo create aboigues/telemach-plugin-kubectl --public --source . --remote origin`, activer le Dependency graph (LL-005), pousser `main`.
  2. Créer `.github/branch-protection.json` : `required_status_checks` (`strict: true`, contextes `lint`, `test`, `vuln`, `secrets`, `dependency-review`, `integration`, `manifest-lint`), `enforce_admins: true`, `required_pull_request_reviews.required_approving_review_count: 0`, `required_linear_history: true`, `required_conversation_resolution: true`, `allow_force_pushes: false`, `allow_deletions: false`, `restrictions: null`.
  3. Appliquer par `gh api -X PUT repos/aboigues/telemach-plugin-kubectl/branches/main/protection --input .github/branch-protection.json`, vérifier par `gh api … /protection`, puis vérifier qu'un `git push origin main` direct est refusé.
  4. Workflow de fusion : Claude Code ouvre la PR, l'utilisateur la relit et la fusionne (validation humaine). Le consigner dans `CONTRIBUTING.md`.

**Checkpoint**: `go build ./...` passe sur le squelette ; le workflow est valide ; `main` est protégée et refuse un push direct.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: chaîne complète « capture → diagnostic → rendu » avec repli « erreur non reconnue », partagée par toutes les histoires

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Tests fondamentaux (écrire d'abord)

- [X] T012 [P] Écrire `internal/sanitize/sanitize_test.go` : retrait des séquences ANSI CSI/OSC et des caractères de contrôle sauf `\n` et `\t`, accents UTF-8 conservés, cible `FuzzSanitize` (aucun `\x1b` en sortie, aucune panique)
- [X] T013 [P] Créer le corpus `testdata/secrets/` (≥ 20 paires `*.txt` / `*.expected.txt`) : `--token=…` et `--token …`, `--password`, `create secret generic x --from-literal=password=…`, `--docker-password`, `Authorization: Bearer …`, JWT `eyJ…`, extrait de kubeconfig (`client-key-data:`, `token:`), bloc PEM, URL `user:pass@`, manifeste `kind: Secret` avec `data` et `stringData`, `env` dont le nom contient `PASSWORD`, `PASSWD`, `SECRET`, `TOKEN`, `KEY`, `CREDENTIAL`, `AUTH`, `CERT` (casse quelconque)
- [X] T014 [P] Écrire `internal/redact/redact_test.go` : chaque cas de `testdata/secrets/` (valeur → `••••••`, nom conservé, chemins de `--client-key` conservés), `RedactArgs`, `RedactManifest` (nœud YAML), cible `FuzzRedact` (idempotent, aucune panique)
- [X] T015 [P] Écrire `internal/command/command_test.go` (table) : `get pods -n demo`, `-n demo get pods`, `--context=kind-x get po web`, `get pod/web`, `apply -f a.yaml -f dir/ -R`, `apply -f -` (non relisible), `apply -f https://…` (non relisible), `apply -k .`, `create deployment web --image=nginxinc/nginx-unprivileged:1.30-alpine`, `run web --image=… --dry-run=client`, `config use-context x`, `rollout status deploy/web`, `set image deploy/web web=…`, `exec -it web -- sh`, `exec -ti`, `logs -f web`, `get pods -w`, `port-forward`, `edit` (`Interactive=true`), `create secret generic s --from-literal=password=x` (valeur masquée). Vérifier `Subcommand`, `ResourceType`, `ResourceNames`, `Namespace`, `Context`, `Kubeconfig`, `Files`, `Recursive`, `DryRun`, `Interactive` (data-model.md § Command)
- [X] T016 [P] Écrire `internal/kubeterms/kubeterms_test.go` : résolution des noms courts et pluriels (`po`, `pods`, `deploy`, `deployments.apps`, `svc`, `cm`, `ns`, `ing`, `pvc`, `sts`, `ds`, `cj`, `job`, `secret`) ; `Closest("deploymnet")` → `deployment` ; `Closest("pdos")` → `pods` ; sous-commandes (`gt` → `get`) ; options globales reconnues (research.md R2)
- [X] T017 [P] Écrire `internal/render/color_test.go` : `--color=auto|always|never`, `NO_COLOR` non vide → pas de couleur, `CLICOLOR_FORCE=1` → `always`, sortie non-TTY → pas de couleur en `auto`
- [X] T018 [P] Écrire `internal/state/state_test.go` : `$XDG_STATE_HOME/telemach-kubectl/last.json` (repli `~/.local/state/telemach-kubectl/`), dossier `0700`, fichier `0600`, `SchemaVersion` = `1`, fichier corrompu ou d'un autre schéma ignoré puis réécrit, `IsRepeat` vrai à moins de 10 min pour la même empreinte, faux au-delà ; aucune chaîne non masquée écrite (test avec un secret du corpus)
- [X] T019 [P] Écrire `tests/shell/shellinit_test.go` : scripts générés exécutés sous `bash` et `zsh` (sinon `t.Skip`) dans un pseudo-terminal, avec un faux `kubectl` et un faux `kubectl-telemach` dans `PATH`. Vérifier les garanties 1 à 8 et 11 de contracts/cli.md § shell-init : code de sortie d'origine ; stdout identique octet pour octet ; fichier temporaire `0600` supprimé, y compris après SIGINT ; `TELEMACH_DISABLE=1` ; binaire absent ; commande interactive sans redirection et avec `--interactive` ; `kubectl version --client -o json` appelé **une seule fois** à l'évaluation (compter les appels) et transmis par `--kubectl-version` ; aucune sortie dupliquée sous zsh (LL-002). Cas d'alias (R4) : `alias kubectl='fakewrap kubectl'` → la commande réelle exécutée est `fakewrap kubectl …` et l'erreur est expliquée ; `alias k=kubectl` → couvert avec `--invoked-as k` ; `k` absent → `alias k` toujours absent après évaluation ; `alias k=ls` → inchangé ; rechargement du script sans doublon
- [X] T020 [P] Écrire `internal/catalog/safety_test.go` : pour chaque extrait YAML (`Fix.Manifest`, `Deepen.Example`) du catalogue, vérifier `securityContext.runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `resources.requests` et `resources.limits` déclarés, absence de `privileged: true`, `hostPath`, `hostNetwork`, `hostPID` (échec), et image sans tag ou en `latest` → **avertissement non bloquant** (`t.Log`, constitution v1.0.0 principe III). Une entrée marquée `Safe: false` est exemptée mais doit avoir un `AntiPattern`. Placer ces règles dans un paquet de test partagé `tests/manifests/rules.go` et écrire `tests/manifests/lint_test.go` (contrôle CI `manifest-lint`, constitution principe IV) qui les applique aussi à tous les manifestes de `testdata/manifests/valid/` et `testdata/pods/scenarios/` ; un scénario ne peut s'en écarter que pour les défauts listés dans le champ `allowed_defects` de son `expected.yaml` ; les autres dossiers de `testdata/manifests/` (volontairement faux) sont exclus ; le test réussit s'il ne trouve aucun manifeste

### Implémentation fondamentale

- [X] T021 [P] Implémenter `internal/sanitize/sanitize.go` (`func Clean(s string) string`) en reprenant le projet Docker. Faire passer T012
- [X] T022 [P] Implémenter `internal/redact/redact.go` (`Redact`, `RedactArgs`, `RedactManifest(*yaml.Node)`) selon research.md R9, motifs RE2, marqueur `••••••`. Faire passer T014
- [X] T023 [P] Implémenter `internal/textdist/levenshtein.go` (`Distance`, `Closest(word string, candidates []string, maxDist int) (string, bool)`) avec son test, repris du projet Docker
- [X] T024 [P] Implémenter `internal/kubeterms/kubeterms.go` : tables embarquées des types de ressources de base (nom, pluriel, noms courts, groupe, namespacé ou non), des sous-commandes kubectl de FR-003 et de leurs options, des options globales (research.md R2) ; `ResolveType`, `ClosestType`, `ClosestSubcommand`, `ClosestFlag`. Faire passer T016 (dépend de T023)
- [X] T025 Implémenter `internal/command/command.go` : type `Command` (data-model.md) et `func Parse(args []string) Command` ; options globales reconnues **avant ou après** la sous-commande, avec ou sans `=` ; `-f -` et URL marqués non relisibles ; détection `Interactive` (research.md R3). Faire passer T015 (dépend de T022, T024)
- [X] T026 [P] Implémenter `internal/render/color.go` (`ResolveColor(flag string, env func(string) string, isTTY bool) bool`). Faire passer T017
- [X] T027 [P] Implémenter `internal/render/theme.go` : styles lipgloss v2 adaptatifs, un par rôle, avec symbole et libellé (`✖`, `📘 Concept`, `🔎 Cause probable`, `💡 Corrections`, `⚠ Anti-pattern`, `✔`, `➜ Pour aller plus loin`, `── Message kubectl d'origine ──`), rendu sans couleur
- [X] T028 [P] Implémenter `internal/render/highlight.go` : `Highlight(code, lexer string, color bool)` (chroma v2, lexers `bash`, `yaml`, `json`) et `HighlightLines(lines []Line, focusLine, focusCol int, related []int, color bool)` (ligne fautive marquée `➤`, marqueur `^` sous la colonne, lignes liées marquées) avec `internal/render/highlight_test.go`
- [X] T029 Implémenter `internal/catalog/schema.go` et `internal/catalog/loader.go` : types `CatalogEntry`, `Fix`, `ConceptRef`, `Deepening`, `AntiPatternNote` (data-model.md) ; `go:embed data/*.yaml`, `yaml.v3` en `KnownFields(true)` ; validation : `ID` unique conforme à `^[a-z]+(\.[a-z0-9-]+)+$` ; `Category` ∈ {`cmdline`, `connection`, `auth`, `notfound`, `namespace`, `manifest`, `schema`, `conflict`, `workload`, `runtime`} ; `Patterns` ≥ 1 sauf `Fallback`, `PodReasons` ou `ExitCodes` non vides, toutes compilées ; `Summary` obligatoire et ≤ 160 caractères ; `Cause` obligatoire ; `Fixes` ≥ 1 ; `Deepen` obligatoire ; `DocURL` préfixé par `https://kubernetes.io/docs/` ; `Tests` ≥ 1
- [X] T030 Écrire `internal/catalog/catalog_test.go` : charge le catalogue embarqué, vérifie les règles de T029, que chaque fichier de `Tests` existe dans `testdata/` et correspond à un motif de l'entrée, et le contrôle de langue SC-005 (mot-outil français dans `Summary`, `Cause`, `Concept.Definition`) (dépend de T029)
- [X] T031 Implémenter `internal/diagnose/diagnose.go` : type `Diagnosis` (`Source` ∈ {`command`, `check`, `pod`}, `Confidence` ∈ {`recognized`, `partial`, `unknown`}, `Origin` ∈ {`kubectl`, `cluster`, `application`, `unknown`}, `Entry`, `Captures`, `ManifestIssues`, `Pod`, `OtherFailingPods`, `Fingerprint`) ; `func Diagnose(cmd command.Command, exitCode int, stderr string, cat *catalog.Catalog) Diagnosis` : filtre `Applies`, premier motif correspondant, captures masquées ; repli `unknown` sans cause (FR-008) ; `Fingerprint` = SHA-256 commande + erreur normalisées et masquées. Tests dans `internal/diagnose/diagnose_test.go` (dépend de T025, T029)
- [X] T032 [P] Implémenter `internal/state/state.go` : `LastDiagnosis` (champs de data-model.md), `Load`, `Save` (écriture atomique, `0600`), `IsRepeat(fp string, now time.Time) bool` (10 min). Faire passer T018
- [X] T033 Implémenter `internal/render/explanation.go` : `RenderExplanation(w io.Writer, d diagnose.Diagnosis, original string, opts Options)` ; blocs dans l'ordre de l'exemple de contracts/cli.md § explain ; message d'origine **atténué avec les captures surlignées** (FR-007) ; bloc « Erreur non reconnue » sans cause, avec version de kubectl et lien (FR-008) ; rendu court si `Repeated` (FR-009b) avec renvoi à `kubectl telemach last` ; gabarit `{{.kubectl}}` remplacé par `kubectl` ou `k` (dépend de T027, T028)
- [X] T034 [P] Écrire `internal/shellinit/scripts/bash.sh` et `internal/shellinit/scripts/zsh.sh` (commentaires en français) selon research.md R3 et R4 : détermination de la commande kubectl réelle (`__telemach_kubectl_cmd`, alias découpé par le shell, **sans `eval`**), redéfinition de l'alias `kubectl` s'il existe, sinon fonction `kubectl` ; prise en charge de `k` uniquement s'il est déjà un alias de kubectl (`__telemach_kubectl_as k`) ; fonction `__telemach_kubectl` : `mktemp` dans `${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}` avec `umask 077`, `2> >(tee "$tmp" >&2)` sauf commande interactive, `trap` de nettoyage, appel de `command kubectl-telemach explain --exit-code "$rc" --stderr-file "$tmp" --kubectl-version … --invoked-as … -- "$@"` (code ignoré), renvoi de `$rc` ; court-circuits `TELEMACH_DISABLE=1` et binaire absent ; lecture unique de la version par `"${__telemach_kubectl_cmd[@]}" version --client -o json` ; exports `TELEMACH_KUBECTL`, `TELEMACH_KUBECTL_VERSION`, `TELEMACH_KUBECTL_SHELL=<shell>:<version>`. Implémenter `internal/shellinit/shellinit.go` (`go:embed`, `Script(shell string) (string, error)`, erreur en français si shell inconnu). Faire passer T019
- [X] T035 Implémenter `cmd/kubectl-telemach/main.go` : racine cobra `kubectl-telemach` (`--color`, `--width` ≥ 40), sous-commandes `shell-init` (`cmd/kubectl-telemach/shellinit.go`, stdout, code `2` si shell inconnu) et `version` (`cmd/kubectl-telemach/version.go` : version du plugin et versions de règles embarquées) ; aide et erreurs en français
- [X] T036 Implémenter `cmd/kubectl-telemach/explain.go` selon contracts/cli.md § explain : `--exit-code` (1 à 255), `--stderr-file` (1 Mio max., on garde la fin, jamais modifié ni supprimé), `--interactive`, `--kubectl-version` (format `^v?[0-9]+\.[0-9]+\.[0-9]+([-+.][0-9A-Za-z.-]+)?$`, ignorée si invalide), `--invoked-as kubectl|k`, `-- <args>` ; pipeline `sanitize` → `redact` → `command.Parse` → `diagnose.Diagnose` → `state` → `render` sur **stderr**, couleur résolue sur stderr ; code `0` si explication produite, `2` si usage invalide. Ajouter `cmd/kubectl-telemach/explain_test.go` : quickstart V6 (bloc « non reconnue », aucune cause), `--color=always` (séquences présentes, captures surlignées), `--color=never` et `NO_COLOR=1` (aucun `\x1b`, symboles présents, SC-007)

**Checkpoint**: `go test ./...` passe ; `kubectl-telemach explain` reformule honnêtement toute erreur et l'intégration shell fonctionne de bout en bout sous bash et zsh, alias compris.

---

## Phase 3: User Story 1 - Comprendre l'échec d'une commande kubectl (Priority: P1) 🎯 MVP

**Goal**: expliquer en français les erreurs de commandes kubectl (ligne de commande, connexion, RBAC, ressources, namespaces), avec la commande corrigée qui conserve les arguments (FR-002 à FR-012).

**Independent Test**: quickstart V1 à V6 et V17 sur un cluster kind jetable ; corpus reconnu à ≥ 90 % (SC-001).

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T037 [P] [US1] Écrire `tests/integration/kind_test.go` (étiquette `integration`) : aide commune qui crée un cluster kind jetable à partir de `TELEMACH_KIND_IMAGE` (digest obligatoire), un namespace par test, et **refuse de s'exécuter** si le contexte cible n'est pas `kind-telemach-*` ; nettoyage en `t.Cleanup`
- [X] T038 [P] [US1] Créer `scripts/collect-errors/main.go` (outil du mainteneur, étiquette `integration`) qui rejoue une liste de commandes fautives sur le cluster kind de T037 et écrit `testdata/errors/<id>/{args.txt,stderr.txt,exit-code.txt}` masqués ; constituer le corpus **≥ 30 erreurs réelles** pour 1.35, 1.36 et 1.37, avec **au moins un cas par sous-commande de FR-003** (`get`, `describe`, `apply`, `create`, `delete`, `run`, `expose`, `logs`, `exec`, `scale`, `rollout`, `set image`, `edit`, `diff`, `config use-context`, `config current-context`, `config get-contexts`, `port-forward`), couvrant research.md R5 (ligne de commande, connexion, auth/RBAC, NotFound, namespace, AlreadyExists, Conflict, `flags cannot be placed before plugin name`, application dans `exec`), chacune avec `expected.yaml` (`entry`, `origin`, `suggestion`)
- [X] T039 [P] [US1] Écrire `internal/diagnose/corpus_test.go` : taux de reconnaissance correcte (bonne entrée, bonne origine, bonne suggestion) ≥ 90 % sur `testdata/errors/` (SC-001) ; 0 cause inventée sur les cas marqués `unknown` (SC-005) ; échec si une sous-commande de FR-003 n'a aucun cas dans le corpus
- [X] T040 [P] [US1] Écrire `internal/diagnose/fixes_test.go` (FR-011, FR-012) : `get deploymnet -n demo --context kind-x` → correction `kubectl get deployment -n demo --context kind-x` ; `gt pods` → `get pods` ; `get po --nmespace demo` → `--namespace demo` ; invoqué par `k` → corrections en `k …` ; aucune valeur sensible dans la correction
- [X] T041 [P] [US1] Écrire `internal/render/explanation_golden_test.go` : golden files `testdata/golden/us1/*.{color,nocolor}.txt` pour V1 à V5, rendu court de répétition, accents vérifiés
- [X] T042 [P] [US1] Écrire `tests/integration/us1_test.go` : V1, V2, V4 et V5 du quickstart exécutés via un vrai `kubectl` et l'intégration shell (pas le binaire en direct, LL-014) ; code de sortie préservé ; aucune modification du namespace (comparaison `get all -o json` avant/après)

### Implementation for User Story 1

- [X] T043 [P] [US1] Rédiger `internal/catalog/data/cmdline.yaml` : type de ressource inconnu, sous-commande inconnue (réutiliser `Did you mean this?`), option inconnue et raccourci inconnu, ressource non précisée, nom invalide, `flags cannot be placed before plugin name` (correction réordonnée, research.md R2)
- [X] T044 [P] [US1] Rédiger `internal/catalog/data/connection.yaml` : connexion refusée, serveur injoignable, `current-context is not set`, contexte inexistant, certificat `x509` ; concepts client/serveur API, contexte, kubeconfig ; corrections `kubectl config get-contexts`, `config use-context`, démarrage du cluster local ; **jamais** d'affichage du kubeconfig
- [X] T045 [P] [US1] Rédiger `internal/catalog/data/auth.yaml` : `Unauthorized`, `Forbidden` (captures sujet, verbe, ressource, groupe, namespace) ; correction `kubectl auth can-i <verbe> <ressource> -n <ns>` ; `AntiPattern` « s'attribuer cluster-admin » (FR-010)
- [X] T046 [P] [US1] Rédiger `internal/catalog/data/notfound.yaml` et `internal/catalog/data/namespace.yaml` : ressource introuvable (orientation vers un autre namespace, `get … -A`), namespace inexistant
- [X] T047 [P] [US1] Rédiger `internal/catalog/data/conflict.yaml` (`AlreadyExists`, `Conflict` sur `apply`/`edit`) et `internal/catalog/data/runtime.yaml` (entrée `Fallback: application` : erreur de la commande du conteneur dans `exec`/`run -it`, orientation vers `kubectl logs`, codes 126, 127, 137, 143)
- [X] T048 [US1] Implémenter `internal/diagnose/origin.go` (FR-004, research.md R5) et `internal/diagnose/suggest.go` : suggestion par `kubeterms` et reconstruction de la commande corrigée qui conserve tous les autres arguments, le contexte et le namespace, avec `{{.kubectl}}` (FR-011, FR-012). Faire passer T039 et T040 (dépend de T043 à T047)
- [X] T049 [US1] Implémenter `cmd/kubectl-telemach/last.go` (`kubectl telemach last`, FR-009a) : rendu complet de `LastDiagnosis`, jamais court ; message pédagogique si aucun diagnostic ; avertissement `⚠` si `TELEMACH_KUBECTL_SHELL` est absent ou d'une autre version (LL-006, LL-013)
- [X] T050 [US1] Écrire `internal/diagnose/bench_test.go` : `explain` de bout en bout (chargement du catalogue compris) sous 200 ms au p95 sur le corpus (SC-003, LL-012) ; valider V1 à V6 et V17 du quickstart

**Checkpoint**: US1 est utilisable seule (MVP) : toute erreur kubectl courante est expliquée, SC-001 ≥ 90 %.

---

## Phase 4: User Story 2 - Localiser une erreur dans un manifeste YAML, à la ligne près (Priority: P2)

**Goal**: relire les manifestes de `-f`, localiser fichier, document, ligne, colonne, ressource et champ, proposer la correction, et vérifier avant application avec `check`, sans cluster (FR-013 à FR-017a).

**Independent Test**: quickstart V7 à V11 ; corpus de manifestes localisé à ≥ 90 % (SC-002).

### Tests for User Story 2 ⚠️

- [X] T051 [P] [US2] Constituer `testdata/manifests/` : **≥ 20 dossiers** erronés (un problème par dossier) + `valid/`, chacun avec `expected.yaml` (`file`, `document`, `line`, `column`, `resource`, `field_path`, `kind` de `ManifestIssue`, `suggestion`) et, si l'erreur passe par kubectl, le `stderr.txt` réel relevé sur kind. Couvrir : tabulation, YAML mal formé, `---` mal placé, `apiVersion`/`kind` manquants, `kind` inconnu, `extensions/v1beta1` Deployment, champ inconnu (`imagePullPolicyy`), mauvais niveau (`containerPort` sous `containers[0]`), mauvais type (`replicas: "3"`), champ obligatoire manquant (`containers`, `selector`), enum invalide, selector/labels incohérents, nom DNS-1123 invalide, multi-documents (erreur dans le 3e), CRD inconnue, `Secret` en clair, `env` sensible, fichier non UTF-8, fichier introuvable
- [X] T052 [P] [US2] Écrire `internal/manifest/corpus_test.go` : fichier + ligne + champ corrects dans ≥ 90 % des cas, via `check` **et** via `explain` avec le `stderr.txt` (SC-002) ; extraits sans valeur sensible (SC-006)
- [X] T053 [P] [US2] Écrire `internal/manifest/locate_test.go` et la cible `FuzzLocate` : conversion `yaml: line N` relative au document → ligne du fichier ; chemin `spec.template.spec.containers[0].image` → nœud ; document retrouvé par `kind`/`metadata.name` du message ; chemins inexistants sans panique
- [X] T054 [P] [US2] Écrire `internal/schema/validate_test.go` : sur les trois versions, `PodSpec.containers` obligatoire, `DeploymentSpec.selector` et `template` obligatoires, type `string` de `imagePullPolicy`, `x-kubernetes-int-or-string` accepté pour `targetPort`, `x-kubernetes-preserve-unknown-fields` respecté ; suggestion du champ proche au même niveau puis au niveau voisin ; sélection de version (`v1.36.5` → 1.36, `v1.32.3` → 1.37 + `Fallback`, vide → 1.37)
- [X] T055 [P] [US2] Écrire `internal/render/manifest_golden_test.go` : golden files `testdata/golden/us2/*` pour V7, V8, V9, V11 (en-tête « Règles Kubernetes 1.37 », extrait, `➤`, `^`, version corrigée, lignes liées)
- [X] T056 [P] [US2] Écrire `cmd/kubectl-telemach/check_test.go` : codes `0`/`1`/`2` ; **sans `-f`** → aucun fichier ouvert (vérifier par un dossier courant contenant un manifeste piégé), message FR-017a, code `2` ; `-R` ; lecture seule (dates de modification inchangées) ; aucun accès réseau ni exécution (faux `kubectl` qui échoue s'il est appelé)

### Implementation for User Story 2

- [X] T057 [P] [US2] Créer `scripts/gen-schemas/main.go` (outil du mainteneur, jamais appelé à l'exécution) : télécharge `api__v1`, `apis__apps__v1`, `apis__batch__v1`, `apis__networking.k8s.io__v1` (OpenAPI v3) aux tags **v1.35.9, v1.36.5, v1.37.1** (vérifier au moment de l'exécution que ce sont toujours les derniers patchs de `dl.k8s.io/release/stable-1.3X.txt`), vérifie leurs SHA-256 consignées dans `scripts/gen-schemas/sources.txt`, garde la fermeture transitive des 12 types de FR-014, retire les descriptions, écrit `internal/schema/data/v1.3{5,6,7}.json.gz` ; mesurer la taille totale (objectif < 2 Mio, plan.md) et la noter dans research.md R7
- [X] T058 [US2] Implémenter `internal/schema/schema.go` et `internal/schema/validate.go` : `SchemaSet` (data-model.md), `Select(kubectlVersion string) (SchemaSet, fallback bool)`, validateur sur `*yaml.Node` (`type`, `properties`, `required`, `items`, `additionalProperties`, `$ref`, `allOf`, `enum`, extensions `x-kubernetes-*`), suggestions via `textdist`. Faire passer T054. Activer dans `tests/manifests/lint_test.go` la validation par les règles des trois versions (contrôle `manifest-lint`) (dépend de T057, T020)
- [X] T059 [P] [US2] Implémenter `internal/manifest/read.go` : `ManifestFile`, `ManifestDocument` (data-model.md), lecture seule des `-f` (fichiers, dossiers, `-R`, extensions `.yaml`, `.yml`, `.json`, 1 Mio max. par fichier, UTF-8 obligatoire), documents multiples avec `StartLine`, index chemin → nœud
- [X] T060 [US2] Implémenter `internal/manifest/syntax.go` (tabulations, YAML mal formé, erreur bloquante) et `internal/manifest/locate.go` (traduction des messages kubectl en position, research.md R6). Faire passer T053 (dépend de T059)
- [X] T061 [US2] Implémenter `internal/manifest/semantic.go` : selector/labels avec `RelatedLines`, table des `apiVersion` retirées (research.md R7), noms DNS-1123, CRD inconnue (structure commune seulement), `Secret`/`env` en clair (`plaintext-secret`, valeurs masquées) ; et `internal/manifest/analyze.go` qui assemble syntaxe, schéma et sémantique en `[]ManifestIssue` triés (fichier, document, ligne ; bloquants d'abord) avec `Excerpt` masqué et `Fixed` (dépend de T058, T060)
- [X] T062 [P] [US2] Rédiger `internal/catalog/data/manifest.yaml` et `internal/catalog/data/schema.yaml` : messages kubectl de manifestes (research.md R5) et pédagogie de chaque `ManifestIssue.Kind` (`Fallback: manifest-issue`), exemples conformes à `safety_test`
- [X] T063 [US2] Implémenter `internal/render/manifest.go` : rendu des `ManifestIssue` (« Problème i/n — fichier:ligne:colonne (document d, Kind/nom) », extrait coloré, version corrigée, mention de la version de règles et du repli). Faire passer T055 (dépend de T061)
- [X] T064 [US2] Brancher l'analyse dans `cmd/kubectl-telemach/explain.go` : si la sous-commande lit des manifestes locaux (`apply`, `create`, `diff`, `replace`, `delete -f`), relire les `-f` et localiser ; `-f -`, URL, `-k` → mention « ligne non localisable » ; `LastDiagnosis.Files` renseigné pour que `last` réanalyse (dépend de T061, T063)
- [X] T065 [US2] Implémenter `cmd/kubectl-telemach/check.go` selon contracts/cli.md § check (FR-017, FR-017a, FR-014a) : version de règles d'après `TELEMACH_KUBECTL_VERSION`, en-tête « Règles Kubernetes 1.3X », « ✔ Aucun problème détecté dans … (n documents) », codes `0`/`1`/`2`. Faire passer T052 et T056 (dépend de T061, T063)

**Checkpoint**: US2 fonctionne seule sur la base de la Phase 2 ; V7 à V11 passent ; SC-002 ≥ 90 %.

---

## Phase 5: User Story 3 - Approfondir un concept à la demande (Priority: P3)

**Goal**: `kubectl telemach deepen [--exercise]` approfondit le dernier diagnostic (FR-009).

**Independent Test**: quickstart V12 après une explication de US1 ou US2.

- [X] T066 [P] [US3] Écrire `cmd/kubectl-telemach/deepen_test.go` et `internal/render/deepen_golden_test.go` : contenu de `Deepen` du dernier diagnostic, exemple coloré, `--exercise`, message si aucun diagnostic ; l'exercice nomme un namespace d'exercice à créer par l'apprenant et ne cible jamais `default` ni `kube-system`
- [X] T067 [US3] Implémenter `internal/render/deepen.go` et `cmd/kubectl-telemach/deepen.go` (lecture de `LastDiagnosis`, entrée du catalogue, avertissement d'intégration inactive comme `last`). Faire passer T066
- [X] T068 [US3] Relire toutes les entrées `Deepen` du catalogue : concept détaillé, exemple exécutable sur kind, exercice court et sans risque ; ajouter à `internal/catalog/catalog_test.go` la vérification que chaque `Deepen.Exercise` mentionne `kubectl create namespace`

**Checkpoint**: US3 est démontrable avec une entrée de US1 ou de US2.

---

## Phase 6: User Story 4 - Diagnostiquer une ressource qui ne fonctionne pas (Priority: P4)

**Goal**: `kubectl telemach diagnose` explique les pods en échec par des lectures seules bornées (FR-022 à FR-027) ; la vérification différée (`watch`) signale un pod en échec après une création réussie (FR-028 à FR-031).

**Independent Test**: quickstart V13 et V14 sur kind ; SC-008, SC-009, SC-010.

### Tests for User Story 4 ⚠️

- [ ] T069 [P] [US4] Écrire `internal/kuberead/allowlist_test.go` : faux `kubectl` (script qui journalise argv) ; chaque fonction publique de `kuberead` produit **exactement** une commande du tableau de research.md R8, suivie de `--context`, `-n`, `--kubeconfig` transmis et `--request-timeout=10s` ; aucun autre verbe que `get` et `logs`, jamais `secret`/`secrets`, jamais `-A` implicite ; `TELEMACH_KUBECTL="fakewrap kubectl"` → argv `fakewrap kubectl …` sans shell ; un nom de pod contenant `;` ou `$(…)` reste un argument unique (SC-008, FR-023)
- [ ] T070 [P] [US4] Créer `testdata/pods/` : JSON réels relevés sur kind (pods, events, journaux) pour `ImagePullBackOff`, `ErrImagePull`, `CrashLoopBackOff` (code 1), `CreateContainerConfigError` (Secret et ConfigMap manquants), `OOMKilled`, `Error` code 127, `Pending` (ressources insuffisantes, PVC non lié, nodeSelector impossible), sonde readiness et liveness en échec, pod sain, pod `ContainerCreating` ; chaque cas avec `expected.yaml` ; JSON contenant volontairement des `env[].value` et une annotation `last-applied-configuration` avec un secret ; et `testdata/pods/scenarios/*.yaml` pour reproduire chaque état : manifestes conformes aux règles de `manifest-lint` (T020) sauf le défaut voulu, déclaré dans `allowed_defects` de `testdata/pods/scenarios/expected.yaml`
- [ ] T071 [P] [US4] Écrire `internal/workload/diagnose_test.go` : sélection (pods en échec seulement, tri du plus récent, au plus 10 autres listés avec `kubectl telemach diagnose pod/<nom>`), état reconnu et cause correcte pour chaque cas de `testdata/pods/` (≥ 90 %, SC-009), `starting` jamais signalé, lectures refusées → `partial` avec la lecture nommée, aucune valeur d'`env` ni d'annotation dans `PodSnapshot`, le rendu ou `last.json` (SC-006)
- [ ] T072 [P] [US4] Écrire `internal/workload/targets_test.go` : cibles de `watch` sans lecture du cluster — `apply -f deploy.yaml` (selector du Deployment), pod nommé, multi-documents, `create deployment web` → `app=web`, `run web` → pod `web` ; `--dry-run`, `-f -`, URL, `-k`, `get` → aucune cible
- [ ] T073 [P] [US4] Écrire `tests/shell/watch_test.go` (pseudo-terminal, bash et zsh) : après une création réussie, `watch` lancé en arrière-plan sans message de tâche ; signal affiché à l'invite suivante, jamais pendant une saisie ; fichier `pending-<pid>` `0600` lu puis supprimé ; aucun processus lancé par le crochet en l'absence de fichier ; `TELEMACH_WATCH=0` et `TELEMACH_DISABLE=1` → aucun appel ; shell non interactif → aucun appel ; invite retardée de moins de 50 ms au p95 (SC-010)
- [ ] T074 [P] [US4] Écrire `tests/integration/us4_test.go` : V13 et V14 sur kind ; état du namespace (`get all,events -o json`, hors champs volatils) identique avant et après `diagnose` et `watch` (SC-008) ; `diagnose` < 5 s par pod (SC-009) ; lecture refusée via `--as=system:serviceaccount:<ns>:default` → diagnostic partiel ; signal présent pour un pod en échec, absent pour un pod sain (SC-010)

### Implementation for User Story 4

- [ ] T075 [US4] Implémenter `internal/kuberead/read.go` et `internal/kuberead/types.go` : seules les lectures de research.md R8, `os/exec` avec liste d'arguments, commande issue de `TELEMACH_KUBECTL` découpée en mots sans shell (sinon `kubectl` du `PATH`), délai de 10 s ; types JSON dédiés qui **ne décodent pas** `env[].value`, `valueFrom` détaillé ni `metadata.annotations` (seuls les noms de variables et les références `secretKeyRef`/`configMapKeyRef` sont gardés) ; erreurs `Forbidden` typées (`ErrDenied`) ; journaux et messages passés par `sanitize` et `redact`. Faire passer T069
- [ ] T076 [US4] Implémenter `internal/workload/select.go` et `internal/workload/diagnose.go` : `PodSnapshot` (data-model.md), états `healthy`/`starting`/`failing`, sources de cause dans l'ordre de research.md R10, résolution `deployment/<nom>` par son selector, `OtherFailingPods`, `DeniedReads`. Faire passer T071 (dépend de T075)
- [ ] T077 [P] [US4] Rédiger `internal/catalog/data/workload.yaml` : une entrée par état de FR-025 (`PodReasons`), motifs d'événements (`FailedScheduling … Insufficient memory`, `Unhealthy: Readiness probe failed`, `manifest unknown`, `pull access denied`), concepts (scheduler, téléchargement d'image par le nœud, politique de redémarrage, sondes, limites de mémoire) ; compléter `internal/catalog/data/runtime.yaml` avec les motifs de journal (variable obligatoire absente, permission refusée, adresse déjà utilisée) ; corrections sans `privileged` ni root
- [ ] T078 [US4] Implémenter `internal/render/pod.go` : en-tête (pod, namespace, contexte, image, état, redémarrages), journal ≤ 20 lignes atténué, explication, mention « diagnostic partiel » et lectures refusées, liste courte des autres pods (dépend de T076, T077)
- [ ] T079 [US4] Implémenter `cmd/kubectl-telemach/diagnose.go` selon contracts/cli.md § diagnose : arguments `pod/<nom>`, `deployment/<nom>`, nom de pod ; `-n`, `--context`, `--kubeconfig` ; « ✔ Aucun pod en difficulté dans le namespace … » ; ressource introuvable → noms proches, code `2` ; cluster injoignable → explication `connection.*`, code `1` ; enregistrement dans `LastDiagnosis.Pod` ; `last` réaffiche sans relire le cluster (mettre à jour `cmd/kubectl-telemach/last.go`)
- [ ] T080 [US4] Implémenter `internal/workload/targets.go` et `internal/state/pending.go` (`PendingSignal`, `$XDG_RUNTIME_DIR/telemach-kubectl/pending-<pid>`, dossier `0700`, fichier `0600`, ajout atomique, écriture seulement si `kill(pid, 0)` réussit, purge des fichiers de shells disparus). Faire passer T072 (dépend de T059 pour lire les manifestes)
- [ ] T081 [US4] Implémenter `cmd/kubectl-telemach/watch.go` selon contracts/cli.md § watch : sortie immédiate si aucune cible, attente de 10 s (constante), lecture du **seul statut** des pods ciblés par `kuberead`, une ligne `PendingSignal` par pod `failing`, aucune sortie terminal, code `0` (dépend de T075, T080)
- [ ] T082 [US4] Compléter `internal/shellinit/scripts/bash.sh` et `zsh.sh` (garanties 9 et 10) : après un succès d'`apply`/`create`/`run` dans un shell interactif, si `TELEMACH_WATCH` ≠ `0`, `command kubectl-telemach watch --shell-pid $$ -- "$@" </dev/null >/dev/null 2>&1 &` depuis la fonction ; crochet d'invite (`add-zsh-hook precmd`, `PROMPT_COMMAND` chaîne ou tableau, ajout idempotent) qui affiche puis supprime le fichier, sans processus si absent ; purge à l'évaluation. Faire passer T073 (dépend de T081)
- [ ] T083 [US4] Mettre à jour `SECURITY.md` et `contracts/cli.md` si l'implémentation a dû ajuster la liste des lectures ; valider V13 et V14 du quickstart et faire passer T074

**Checkpoint**: US4 fonctionne sur kind, sans aucune écriture dans le cluster ; SC-008, SC-009, SC-010 mesurés.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T084 [P] Rédiger `README.md` (français) sur le modèle de `../telemach-plugin-docker/README.md` : concept (une erreur = une leçon), exemple de sortie, installation (`go build`, `PATH`), `shell-init`, cas des alias (`microk8s kubectl`, `k`), règle « pas d'option avant `telemach` », sous-commandes, garanties de sécurité et liste des lectures du cluster, variables `TELEMACH_DISABLE`, `TELEMACH_WATCH`, `NO_COLOR`
- [ ] T085 [P] Créer `scripts/couverture` (sur le modèle du projet Docker) et viser ≥ 80 % de couverture sur `internal/` ; ajouter le résultat au job `test`
- [ ] T086 [P] Ajouter les cibles de fuzzing (`FuzzSanitize`, `FuzzRedact`, `FuzzLocate`, `FuzzMatch` dans `internal/diagnose/fuzz_test.go`) à un job CI hebdomadaire `fuzz` de courte durée (60 s par cible)
- [ ] T087 Exécuter le quickstart complet (V1 à V17) sur kind avec chacune des trois images, sous bash et zsh, et consigner tout écart dans `LESSON-LEARNED.md`
- [ ] T088 Vérifier SC-005 et SC-006 sur l'ensemble des golden files et de `last.json` (aucun texte anglais hors termes Kubernetes et message d'origine ; aucun secret du corpus en clair) avec `internal/render/language_test.go`
- [ ] T089 Relire `LESSON-LEARNED.md`, transformer les leçons automatisables en tests ou gates (principe V), et préparer la PR de la feature (description, liste des gates vertes)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** : aucune dépendance ; T011 en dernier (après T001 et T006).
- **Foundational (Phase 2)** : dépend de la Phase 1 ; bloque toutes les histoires.
- **US1 (Phase 3)** : dépend de la Phase 2 uniquement.
- **US2 (Phase 4)** : dépend de la Phase 2 uniquement (pas de US1).
- **US3 (Phase 5)** : dépend de la Phase 2 ; démontrable dès qu'une entrée de US1 ou US2 existe.
- **US4 (Phase 6)** : dépend de la Phase 2 ; T080 réutilise `internal/manifest/read.go` (T059, US2) pour les cibles de `watch` : dépendance acceptée et laissée en US2 (choix de l'analyse du 2026-10-16), car `diagnose` seul n'en dépend pas et US2 précède US4 dans l'ordre de priorité.
- **Polish (Phase 7)** : dépend des histoires livrées.

### Dépendances fines notables

- T023 → T024 → T025 ; T022 + T024 → T025 ; T025 + T029 → T031 ; T029 → T030
- T027 + T028 → T033 ; T031, T032, T033, T034, T035 → T036
- T038 → T039, T043 à T047 (les `Tests` du catalogue référencent le corpus) → T048
- T057 → T058 ; T059 → T060 → T061 ; T058 → T061 → T063 → T064, T065
- T075 → T076 → T078 → T079 ; T059 + T075 → T080 → T081 → T082

### Within Each User Story

- Tests écrits d'abord, et qui échouent
- Données (corpus, catalogue, schémas) → analyse → rendu → branchement CLI
- Histoire validée par son checkpoint avant la priorité suivante

### Parallel Opportunities

- Phase 1 : T005 à T010 en parallèle après T001 à T004
- Phase 2 : tests T012 à T020 en parallèle ; puis T021 à T024, T026 à T028, T032, T034 en parallèle
- US1 : T037 à T042 en parallèle ; puis T043 à T047 (un fichier de catalogue chacun)
- US2 : T051 à T056 en parallèle ; T057, T059, T062 en parallèle
- US4 : T069 à T074 en parallèle ; T077 en parallèle de T075/T076
- Après la Phase 2, US1, US2 et US4 (`diagnose`) peuvent avancer en parallèle

---

## Parallel Example: User Story 1

```bash
Task: "Constituer testdata/errors/ (≥ 30 erreurs réelles relevées sur kind)"
Task: "Écrire internal/diagnose/fixes_test.go (FR-011, FR-012)"
Task: "Écrire internal/render/explanation_golden_test.go"
Task: "Rédiger internal/catalog/data/cmdline.yaml"
Task: "Rédiger internal/catalog/data/connection.yaml"
Task: "Rédiger internal/catalog/data/auth.yaml"
```

## Parallel Example: User Story 2

```bash
Task: "Constituer testdata/manifests/ (≥ 20 cas)"
Task: "Créer scripts/gen-schemas/main.go"
Task: "Implémenter internal/manifest/read.go"
Task: "Rédiger internal/catalog/data/manifest.yaml et schema.yaml"
```

## Parallel Example: User Story 4

```bash
Task: "Écrire internal/kuberead/allowlist_test.go"
Task: "Créer testdata/pods/ (JSON réels + scénarios)"
Task: "Écrire tests/shell/watch_test.go"
Task: "Rédiger internal/catalog/data/workload.yaml"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 : Setup, gates CI actives et `main` protégée (T011)
2. Phase 2 : Foundational (capture shell avec alias, pipeline, repli « non reconnue »)
3. Phase 3 : US1
4. **STOP and VALIDATE** : quickstart V1 à V6, V15 à V17 ; SC-001 ≥ 90 %
5. Démontrer à des apprenants (mesure de SC-004)

### Incremental Delivery

1. Setup + Foundational → toute erreur kubectl est au moins reformulée honnêtement
2. + US1 → MVP des commandes
3. + US2 → manifestes localisés à la ligne et `check`
4. + US3 → approfondissement
5. + US4 → `diagnose` et vérification différée (première lecture du cluster : revue de sécurité dédiée dans la PR)
6. Chaque incrément passe toutes les gates avant fusion

---

## Notes

- [P] = fichiers différents, pas de dépendance sur une tâche inachevée
- [USx] relie chaque tâche à son histoire (traçabilité avec spec.md)
- Commiter après chaque tâche ou groupe logique, via une PR soumise aux gates
- Tout incident (CI rouge inattendue, faille, régression) → entrée dans `LESSON-LEARNED.md`
- Hors périmètre : Windows natif, Kustomize, Helm, CRD (au-delà de la structure commune), StatefulSet/DaemonSet/Job dans `diagnose`, distribution (Krew, Releases : feature ultérieure)
