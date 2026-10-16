# Implementation Plan: Guidage pédagogique à partir des erreurs kubectl

**Branch**: `001-kubectl-error-guidance` | **Date**: 2026-10-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-kubectl-error-guidance/spec.md`

## Summary

TELEMACH PLUGIN KUBECTL capte automatiquement les erreurs des commandes `kubectl` tapées par
l'apprenant et les explique en français : concept Kubernetes, cause probable, corrections
prêtes à copier et approfondissement à la demande. Il localise à la ligne près les erreurs des
manifestes YAML, après un échec ou avant application (`check`, sans cluster). Enfin, il
diagnostique en lecture seule les pods qui ne démarrent pas (`diagnose`) et prévient
l'apprenant, à l'invite suivante, quand un pod qu'il vient de créer est en échec.

Choix techniques (voir [research.md](research.md)), repris du projet TELEMACH PLUGIN DOCKER et
adaptés :
- **Forme** : plugin kubectl en Go, binaire unique `kubectl-telemach` (R1) ;
- **Capture** : intégration shell bash/zsh qui duplique stderr de `kubectl`, y compris quand
  `kubectl` ou `k` est un alias (R3, R4) ;
- **Reconnaissance** : catalogue d'erreurs déclaratif en français, embarqué (R5) ;
- **Manifestes** : yaml.v3 pour les positions, règles OpenAPI v3 de Kubernetes 1.35 à 1.37
  embarquées et un validateur interne qui garde la ligne et la colonne (R6, R7) ;
- **Cluster** : lectures d'une liste fermée, faites par la propre commande kubectl de
  l'apprenant, sans `client-go` (R8, R10, R11) ;
- **Affichage** : sortie colorée accessible (lipgloss, chroma), comme le projet Docker (R12).

## Technical Context

**Language/Version**: Go 1.27.2 (`go.mod` : `go 1.27`, `toolchain go1.27.2`)

**Primary Dependencies** (versions vérifiées le 2026-10-16) :
- `github.com/spf13/cobra` v1.10.2 : sous-commandes ;
- `go.yaml.in/yaml/v3` v3.0.5 : nœuds YAML avec positions ;
- `charm.land/lipgloss/v2` v2.0.6 et `github.com/charmbracelet/colorprofile` v0.4.3 : mise en
  page colorée ;
- `github.com/alecthomas/chroma/v2` v2.27.0 : coloration syntaxique shell et YAML ;
- `golang.org/x/term` v0.47.0 : détection du terminal.

Aucune dépendance Kubernetes (`client-go`, `k8s.io/api`) : les règles viennent des documents
OpenAPI v3 officiels (v1.35.9, v1.36.5, v1.37.1) réduits par un générateur, et le cluster est lu
par la commande kubectl de l'apprenant.

**Storage**: `$XDG_STATE_HOME/telemach-kubectl/last.json` (`0600`, masqué) et
`$XDG_RUNTIME_DIR/telemach-kubectl/pending-<pid>` (`0600`). Catalogue et schémas embarqués
(`go:embed`). Pas de base de données.

**Testing**: `go test` + `go-cmp` v0.7.0 et golden files ; corpus `testdata/` (SC-001, SC-002,
SC-006, SC-009) ; fuzzing (matcher, masquage, chemin → ligne) ; intégration réelle sur kind
v0.33.0 (images `kindest/node` 1.35.8, 1.36.4, 1.37.0 épinglées par digest, étiquette
`integration`) ; tests bash et zsh dans un pseudo-terminal.

**Target Platform**: Linux et macOS (bash, zsh), WSL2 compris. Windows natif hors périmètre.

**Project Type**: CLI, sous forme de plugin kubectl.

**Performance Goals**: explication < 2 s au p95 (SC-003 ; objectif interne < 200 ms) ;
`diagnose` < 5 s (SC-009) ; invite retardée de moins de 50 ms au p95 par l'intégration
(SC-010) ; chacun mesuré par un test (LL-012).

**Constraints**:
- `explain`, `check`, `last`, `deepen` : 100 % hors ligne ;
- exécutions limitées à `kubectl version --client` (une fois, par le script) et aux lectures de
  R8 (`get`, `logs`), via la commande kubectl de l'apprenant, avec ses droits ;
- aucun fichier de l'apprenant ni aucune ressource de cluster modifié ;
- code de sortie, stdin et stdout de `kubectl` intacts ; stderr capté limité à 1 Mio ;
- binaire statique (`CGO_ENABLED=0`) ; schémas embarqués < 2 Mio compressés (à mesurer).

**Scale/Scope**: catalogue initial ≥ 30 erreurs (FR-003) + entrées de pods (FR-025) ; ≥ 20
manifestes erronés ; 12 types de ressources couverts par les règles ; un utilisateur local.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principe | Vérification | Statut |
|----------|--------------|--------|
| **I. Vocation pédagogique** | Chaque entrée du catalogue doit fournir concept, cause, ≥ 1 correction, approfondissement et lien officiel (test de schéma). Erreurs inconnues et causes de pod non trouvées traitées honnêtement (FR-008, FR-026). Anti-patterns signalés (`cluster-admin`, `privileged`). Exemples exécutables vérifiés sur kind (quickstart). | ✅ |
| **II. Langue** | Code et identifiants en anglais ; catalogue, messages, commentaires et documentation en français ; termes Kubernetes tels quels ; golden files vérifiant les accents. | ✅ |
| **III. Sécurité** | Masquage systématique (R9), y compris `Secret`, kubeconfig, `--from-literal`, `env`. Lecture du cluster restreinte à une liste fermée testée (R8), avec les droits de l'apprenant, sans lecture de `Secret`, sans écriture ; tests sur cluster jetable uniquement (LL-015). Pas d'injection : `os/exec` avec liste d'arguments, alias découpé par le shell sans `eval`. RE2. Fichiers `0600`. Versions vérifiées le 2026-10-16 ; images kind épinglées par digest. Manifestes d'exemple contrôlés par `safety_test` (non-root, `resources`, pas de `privileged`/`hostPath`, avertissement non bloquant pour `latest`). | ✅ |
| **IV. Gates GitHub Actions** | Le dépôt n'est pas encore un dépôt git et la CI n'existe pas. `tasks.md` commence par : `git init`, dépôt GitHub, `ci.yml` (`golangci-lint` v2.15.0 + `gosec`, `go test -race`, `govulncheck` v1.9.0, `gitleaks` v8.30.1, revue des dépendances, intégration kind), actions épinglées par SHA, `permissions` minimales, puis protection de `main`. Pas de Dockerfile dans cette feature. Manifestes : exemples du catalogue vérifiés par le contrôle obligatoire `test` (`internal/catalog/safety_test.go`) ; `testdata/manifests/valid/` et `testdata/pods/scenarios/` vérifiés par le contrôle obligatoire `manifest-lint` (validation par les règles embarquées + règles de sécurité) ; seuls les défauts déclarés dans `expected.yaml` sont tolérés ; les manifestes d'erreur de `testdata/manifests/` (hors `valid/`) sont exclus. | ⚠️ à faire en premier |
| **V. LESSON-LEARNED** | Le fichier n'existe pas encore ; les leçons transposables du projet Docker ont été relues et appliquées (R14) ; la première tâche crée `LESSON-LEARNED.md` avec ces règles. | ⚠️ à créer en premier |

**Re-check post-design** : le modèle de données ne persiste que des données masquées ; le
contrat de `shell-init` garantit code de sortie, stdout intact, nettoyage, alias non créés ; le
contrat de `diagnose`/`watch` limite les lectures à la liste de R8 ; `explain` interdit toute
cause inventée. Les deux ⚠️ sont des tâches d'amorçage, pas des violations.

## Project Structure

### Documentation (this feature)

```text
specs/001-kubectl-error-guidance/
├── plan.md              # Ce fichier
├── research.md          # Phase 0 — décisions R1 à R14
├── data-model.md        # Phase 1 — entités
├── quickstart.md        # Phase 1 — scénarios de validation V1 à V17
├── contracts/
│   └── cli.md           # Phase 1 — contrat des sous-commandes
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — pas encore créé)
```

### Source Code (repository root)

```text
cmd/
└── kubectl-telemach/
    └── main.go                 # Point d'entrée, câblage cobra

internal/
├── shellinit/                  # Scripts bash/zsh embarqués (R3, R4, R11)
│   └── scripts/{bash,zsh}.sh
├── command/                    # Analyse des arguments kubectl → Command (options globales partout, R2)
├── kubeterms/                  # Types de ressources, noms courts, sous-commandes, options (FR-011)
├── catalog/                    # Chargement + validation du catalogue
│   └── data/*.yaml             # Catalogue en français, un fichier par catégorie
├── diagnose/                   # Matcher, origine (R5) → Diagnosis
├── manifest/                   # Lecture YAML multi-documents, positions, chemin → ligne,
│                               # règles hors schéma (selector/labels, apiVersion retirées)
├── schema/                     # Validateur OpenAPI v3 sur nœuds YAML, choix de version (R7)
│   └── data/v1.3{5,6,7}.json.gz
├── kuberead/                   # SEULES lectures du cluster (liste fermée, R8)
├── workload/                   # Sélection et diagnostic des pods, cibles de watch (R10, R11)
├── redact/                     # Masquage (R9)
├── sanitize/                   # Neutralisation des séquences de contrôle
├── textdist/                   # Distance d'édition
├── render/                     # lipgloss + chroma, couleur/NO_COLOR
└── state/                      # LastDiagnosis, empreinte, PendingSignal

scripts/
├── gen-schemas/                # Générateur des règles (mainteneur, sommes SHA-256 vérifiées)
└── collect-errors/             # Relevé du corpus d'erreurs réelles sur kind (mainteneur)

testdata/
├── errors/                     # ≥ 30 erreurs kubectl réelles (1.35–1.37) + attendu
├── manifests/                  # ≥ 20 dossiers de manifestes erronés + valid/
├── pods/                       # JSON de pods/événements réels masqués + scénarios kind
├── secrets/                    # Corpus de fuite (SC-006)
└── golden/                     # Sorties de référence (couleur / sans couleur)

tests/
├── manifests/                  # Contrôle manifest-lint (principe IV)
├── integration/                # Étiquette `integration`, cluster kind jetable
└── shell/                      # bash/zsh : capture, alias, crochet d'invite, kubectl telemach

.github/workflows/
└── ci.yml                      # Gates (principe IV)

go.mod
.golangci.yml
LESSON-LEARNED.md
SECURITY.md
```

**Structure Decision**: un seul projet Go `cmd/` + `internal/`, calqué sur
`../telemach-plugin-docker` (mêmes paquets `catalog`, `diagnose`, `redact`, `sanitize`,
`render`, `state`, `shellinit`, `textdist`, `command`). Les spécificités Kubernetes sont isolées
dans `kubeterms`, `manifest`, `schema`, `kuberead` et `workload`. Toute interaction avec le
cluster passe par `kuberead`, ce qui rend la liste fermée de FR-023/FR-029 vérifiable par un
seul test. Le paquet `plugin` du projet Docker n'a pas d'équivalent (kubectl n'a pas de
protocole de métadonnées).

## Complexity Tracking

| Écart | Pourquoi c'est nécessaire | Alternative plus simple écartée parce que |
|-------|---------------------------|--------------------------------------------|
| Validateur OpenAPI interne + générateur de schémas | Trois versions de règles hors ligne avec ligne et colonne (FR-014a, FR-015) | `k8s.io/api` : une seule version et aucune position ; `kubeconform` : schémas tiers, positions perdues |
| Redéfinition de l'alias `kubectl` (et de `k` s'il existe) | Cas réel de l'utilisateur (`microk8s kubectl`) : une fonction ne peut pas coexister avec l'alias (R4) | Ne couvrir que le binaire `kubectl` : le plugin paraîtrait inactif chez l'utilisateur |
