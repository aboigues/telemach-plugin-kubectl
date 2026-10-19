---

description: "Liste des tâches pour la feature 003-distribution"
---

# Tasks: Distribution du plugin sans Go

**Input**: Design documents from `/specs/003-distribution/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: inclus. Le principe IV exige des gates bloquantes, FR-025 exige que chaque procédure
documentée soit testée de bout en bout (LL-004, D-LL-006), et les contrats de `contracts/`
(noms, sommes, refus de `install`, manifeste Krew) se vérifient par des tests. Dans chaque
histoire, les tests sont écrits **avant** l'implémentation et doivent d'abord échouer.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 à US5 de spec.md)
- Include exact file paths in descriptions

## Conventions du projet (à respecter dans chaque tâche)

- Code et identifiants en anglais ; commentaires, messages, README, notes de Release, manifeste
  Krew et `CONTRIBUTING.md` en français avec accents (principe II). Relire `LESSON-LEARNED.md`
  avant chaque phase (principe V).
- **Chaîne de référence** : celle du plugin TELEMACH Docker (`../telemach-plugin-docker`,
  feature `002-distribution`, publiée en v0.2.0). Reprendre ses fichiers en les adaptant aux
  noms de ce dépôt ; ne jamais recopier une version ou un SHA sans le revérifier
  (research.md § Versions, vérifiées le 2026-10-19).
- Actions épinglées par SHA de commit complet ; `permissions` minimales par job ; **aucune
  interpolation `${{ }}` dans un script `run:`** (variables passées par `env:`).
- **Les secrets ne sont jamais manipulés par Claude Code** : `DOCKERHUB_TOKEN`,
  `KREW_INDEX_DEPLOY_KEY` et la variable `DOCKERHUB_USERNAME` sont créés par le mainteneur.
- Toute compilation de vérification écrit dans un dossier temporaire (`-o` vers le scratchpad
  ou `/dev/null`), jamais à la racine du dépôt (LL-007) ; relire `git status` avant `git add`.
- Validation et commit enchaînés par `&&` (D-LL-008) ; lint local par
  `GOTOOLCHAIN=go1.27.2 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 fmt ./...`
  puis `… run ./...` ; `scripts/secrets` avant chaque commit ; après chaque push, relire
  `gh pr checks` (LL-002).
- Les attentes des tests de l'image et des procédures viennent de sorties réellement observées
  (snapshot GoReleaser, image construite), pas de messages supposés (LL-008).
- **Tout push, ouverture de PR, création de tag ou publication est soumis à la confirmation
  de l'utilisateur.**

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: outils locaux et squelettes communs aux trois voies de distribution.

- [X] T001 Installer en local, dans le scratchpad, GoReleaser v2.18.3 et `validate-krew-manifest` (Krew v0.5.0), compilés avec la chaîne d'outils du projet (D-LL-001) : `GOBIN=<scratchpad>/bin GOTOOLCHAIN=go1.27.2 go install github.com/goreleaser/goreleaser/v2@v2.18.3` et `GOBIN=<scratchpad>/bin GOTOOLCHAIN=go1.27.2 go install sigs.k8s.io/krew/cmd/validate-krew-manifest@v0.5.0` ; vérifier `goreleaser --version` et `validate-krew-manifest -h` ; **vérifier aussi** (analyse A1, LL-008) si `validate-krew-manifest` et `kubectl krew install` acceptent une `uri` de fichier local (`file://`) ; sinon, T020 et T026 servent `dist/` par un serveur HTTP local le temps de la validation ; consigner le résultat dans `specs/003-distribution/research.md` § R9
- [X] T002 [P] Créer `cmd/telemach-image/doc.go` : commentaire de paquet en français (« point d'entrée de l'image telemachlearning/telemach-kubectl : installe le plugin sur la machine hôte, vérifie des manifestes sans installation, ou exécute le plugin linux embarqué »)
- [X] T003 [P] Créer `.dockerignore` à la racine : tout exclure (`*`) sauf ce que le `Dockerfile` copie (le contexte réel est préparé par GoReleaser)
- [X] T004 [P] Ajouter l'étiquette de build `dist` à `run.build-tags` de `.golangci.yml` (D-LL-007), en conservant les étiquettes existantes

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: configuration GoReleaser, Dockerfile, point d'entrée minimal de l'image et gates de
CI, dont dépendent les trois voies (Release, Krew, image).

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 [P] Écrire `cmd/telemach-image/main_test.go` (adapté de `../telemach-plugin-docker/cmd/telemach-image/main_test.go`) : aide (aucun argument, `--help`, `-h`, `help`) en français, code 0, contenant les commandes canoniques de `specs/003-distribution/contracts/image-cli.md` avec la version injectée ; toute autre commande (`version`, `explain …`, `check -f …`) transmise telle quelle au binaire `/opt/telemach/linux/kubectl-telemach` (exécuteur injectable dans les tests, pas de `syscall.Exec` réel) ; argument inconnu de `install` → code 2
- [X] T006 Implémenter `cmd/telemach-image/main.go` et `cmd/telemach-image/messages.go` (adaptés du plugin Docker) : variable `version` injectée par `-X main.version=…` (défaut `dev`) ; aiguillage `help` / `install` / `check` / autre ; exécution du plugin par `syscall.Exec` de `/opt/telemach/linux/kubectl-telemach` ; couleurs et symboles par `internal/render` (mêmes règles que le plugin : `NO_COLOR`, terminal) ; `install` et le pré-contrôle de `check` renvoient pour l'instant « non implémenté » (code 2). Faire passer T005
- [X] T007 Créer `Dockerfile` à la racine (research R5) : `FROM scratch`, `ARG TARGETPLATFORM`, `ARG TARGETARCH`, `COPY $TARGETPLATFORM/telemach-image /usr/local/bin/telemach-image`, `COPY $TARGETPLATFORM/kubectl-telemach /opt/telemach/linux/kubectl-telemach`, `COPY dist/kubectl-telemach_darwin_${TARGETARCH}_*/kubectl-telemach /opt/telemach/darwin/kubectl-telemach`, `WORKDIR /manifestes`, `USER 65532:65532`, `ENTRYPOINT ["/usr/local/bin/telemach-image"]` ; aucun `VOLUME`, aucun kubeconfig ; commentaire d'en-tête en français ; hadolint v2.15.1 sans avertissement
- [X] T008 Créer `.goreleaser.yaml` (GoReleaser OSS v2.18.3, `version: 2`, `project_name: telemach-plugin-kubectl`), conforme à `specs/003-distribution/contracts/release-assets.md` et `contracts/image-tags.md` : build `kubectl-telemach` (`./cmd/kubectl-telemach`, linux/darwin × amd64/arm64, `CGO_ENABLED=0`, `-trimpath`, `ldflags: -s -w -buildid= -X main.version={{ .Version }}`, `mod_timestamp: {{ .CommitTimestamp }}`) ; build `telemach-image` (linux seulement, jamais publié) ; **deux** archives : `formats: [binary]` → `kubectl-telemach_{{ .Version }}_{{ .Os }}_{{ .Arch }}` et `formats: [tar.gz]` → même nom `.tar.gz` avec `files: [LICENSE, NOTICE]` à la racine (research R2) ; `checksum` SHA-256 `checksums.txt` ; `sboms` par binaire ; `signs` cosign keyless de `checksums.txt` (`checksums.txt.sigstore.json`) ; `dockers_v2` vers `docker.io/telemachlearning/telemach-kubectl`, plateformes linux/amd64 et linux/arm64, `extra_files` des dossiers darwin de `dist/`, tags `{{ .Version }}` puis `X.Y` et `latest` hors pré-version, labels OCI (titre, description en français, source, licence Apache-2.0, version, révision), `sbom: true` (SBOM de l'image, FR-015) ; `release.draft: true`, `replace_existing_draft: false`, `prerelease: auto`, `name_template: "TELEMACH {{ .Version }}"` ; changelog git groupé Nouveautés / Corrections / Autres changements (exclure `^docs`, `^ci`, `^test`)
- [X] T009 Vérifier avec `goreleaser release --snapshot --clean` (en local si Docker répond, sinon dans la CI via T010) et consigner dans `specs/003-distribution/research.md` § R5 : présence des binaires darwin dans le contexte de l'image (`extra_files`), contenu de l'image (trois fichiers, utilisateur `65532:65532`), **taille compressée par architecture** (SC-007 : < 15 Mo), archives contenant exactement `kubectl-telemach`, `LICENSE`, `NOTICE`, binaire de l'archive identique au binaire brut (même somme)
- [X] T010 Modifier `.github/workflows/ci.yml` : déclencheur `workflow_call` (en plus de `pull_request` et `push` sur `main`) ; job `build` (version issue de `git describe --tags --match 'v[0-9]*' --always --dirty` sans le `v`, compilation `-X main.version=…` dans un dossier temporaire, contrôle que la première ligne de `kubectl-telemach version` est `TELEMACH PLUGIN KUBECTL <version>`) ; job `goreleaser-check` (Syft v1.54.1 via `download-syft` épinglé, `goreleaser check`, `goreleaser release --snapshot --clean --skip=sign,docker`, puis `DIST_DIR=dist go test -tags dist -count=1 ./tests/release/...`) ; job `image-scan` (snapshot avec image, Grype v0.120.1 par `anchore/scan-action` épinglé, `severity-cutoff: high`, `fail-build: true`, image `docker.io/telemachlearning/telemach-kubectl:<version>-amd64`), suivi d'un **contrôle bloquant de la taille compressée** de l'image de chaque architecture (< 15 Mo, SC-007, D-LL-012) et de la **comparaison des sommes** des binaires extraits de l'image avec `dist/checksums.txt` (FR-013) ; job `dockerfile-lint` (hadolint v2.15.1 téléchargé, somme SHA-256 vérifiée, fichiers de `git ls-files -- '*Dockerfile'` hors `testdata/`) ; SHA et versions de research.md § Versions
- [ ] T011 Pousser la branche `003-distribution` et ouvrir une PR **en brouillon** pour valider les nouvelles gates avant d'écrire le reste (D-LL-005). **Demander confirmation avant le push.** Attendu : `build`, `goreleaser-check` (sans tests de contrat encore), `image-scan` et `dockerfile-lint` verts ; consigner l'écart éventuel

**Checkpoint**: snapshot complet (binaires, archives, SBOM, image) reproductible localement et en CI ; nouvelles gates vertes.

---

## Phase 3: User Story 1 - Installer le plugin depuis une Release, sans Go (Priority: P1) 🎯 MVP

**Goal**: une Release `vX.Y.Z` immuable, vérifiable, dont les binaires s'installent dans un
dossier du `PATH` et répondent `kubectl telemach version` avec la version publiée.

**Independent Test**: quickstart S1 sur un snapshot, puis R1 à R4 sur les vrais fichiers après
publication ; `kubectl plugin list` cite le plugin.

### Tests for User Story 1 ⚠️

- [X] T012 [P] [US1] Écrire `tests/release/dist_test.go` (étiquette de build `dist`, dossier lu dans `DIST_DIR`, `t.Skip` si absent ; adapté du plugin Docker), conforme à `contracts/release-assets.md` : présence des 4 binaires `kubectl-telemach_<version>_<os>_<arch>`, des 4 archives `.tar.gz`, des 4 SBOM, de `checksums.txt` ; chaque ligne de `checksums.txt` correspond au fichier ; chaque archive contient exactement `kubectl-telemach`, `LICENSE`, `NOTICE` et son `kubectl-telemach` a la même somme que le binaire brut ; module Go de chaque binaire `github.com/aboigues/telemach-plugin-kubectl` (`debug/buildinfo`) ; le binaire de la plateforme courante, exécuté, affiche `TELEMACH PLUGIN KUBECTL <version>` en première ligne de `version`
- [X] T013 [P] [US1] Écrire `tests/release/reproducible_test.go` (étiquette `dist`) : reconstruire `kubectl-telemach` linux/amd64 avec exactement les options de `.goreleaser.yaml` dans un dossier temporaire et comparer sa somme à la ligne de `checksums.txt` (FR-005)

### Implementation for User Story 1

- [X] T014 [US1] Brancher T012 et T013 dans le job `goreleaser-check` de `.github/workflows/ci.yml` et les faire passer (en local sur le snapshot de T009, puis en CI)
- [X] T015 [US1] Créer `.github/workflows/release.yml` (adapté du plugin Docker) : déclencheurs `push` de tags `v*` et `workflow_dispatch` sans entrée (lancé **sur le tag**) ; `permissions: {}` ; `concurrency` par référence sans annulation ; job `verify` (lecture seule) : format du tag `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$` lu dans `env:`, tag ancêtre de `origin/main` (`git merge-base --is-ancestor`), Release absente même en brouillon (`gh release view`) ; job `gates` (`uses: ./.github/workflows/ci.yml`) ; job `release` (`environment: release`, `contents: write`, `id-token: write`, `attestations: write`) : cosign v3.1.3, Syft v1.54.1, GoReleaser `release --clean`, attestation SLSA par `actions/attest` sur `subject-checksums: dist/checksums.txt`, puis `gh release edit "$TAG" --draft=false`. La partie image est ajoutée en US3 : dans cette tâche, lancer GoReleaser avec `--skip=docker`
- [X] T016 [US1] Rédiger `release.header` et `release.footer` dans `.goreleaser.yaml` (research R11, structure de `contracts/release-assets.md`) : installation Linux/WSL2 et macOS dans `~/.local/bin` (téléchargement, `sha256sum` ou `shasum -a 256 --check --ignore-missing`, `install -m 0755`, `kubectl telemach version`, `kubectl plugin list`), prochaine étape `kubectl-telemach shell-init`, vérification `cosign verify-blob` et `gh attestation verify` ; les lignes Krew et image sont ajoutées en US2 et US3 ; commandes **identiques** à celles du README (T017)
- [X] T017 [US1] Réécrire la section « Installation » de `README.md` dans l'ordre de FR-024 ; rédiger ici « Installer depuis une Release (recommandé) » : `VERSION=0.1.0`, détection de l'architecture, téléchargement, vérification de somme et de signature (ce que fait chaque commande, que faire si elle échoue : ne pas installer), installation dans `~/.local/bin`, ajout au `PATH` si besoin, `kubectl plugin list`, cas d'un binaire téléchargé par navigateur sous macOS ; placer la procédure actuelle « Compiler depuis les sources » en dernier
- [X] T018 [US1] Mettre à jour `specs/003-distribution/quickstart.md` § Reproductibilité avec les options exactes de `.goreleaser.yaml`

**Checkpoint**: une Release complète et vérifiable peut être produite ; la voie manuelle est documentée et testée sur snapshot.

---

## Phase 4: User Story 2 - Installer et mettre à jour le plugin avec Krew (Priority: P2)

**Goal**: après chaque version stable, l'index Krew du projet propose `telemach` à la bonne
version ; `kubectl krew install telemach/telemach` puis `kubectl krew upgrade telemach`
fonctionnent.

**Independent Test**: quickstart S2 (manifeste d'essai validé), puis K1 et K2 sur un index de
répétition et, après la première version, sur l'index réel.

### Tests for User Story 2 ⚠️

- [X] T019 [P] [US2] Écrire `scripts/krew-manifest/main_test.go` : à partir d'un `checksums.txt` d'essai, le manifeste généré contient `metadata.name: telemach`, `spec.version: v<version>`, exactement 4 plateformes (`matchLabels` os/arch), pour chacune `uri` = `https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v<version>/kubectl-telemach_<version>_<os>_<arch>.tar.gz` et `sha256` = la somme de cette archive dans `checksums.txt`, `bin: kubectl-telemach`, `files` binaire + `LICENSE` + `NOTICE` ; textes en français (`shortDescription`, `description`, `caveats` contenant `kubectl-telemach shell-init`) ; échec explicite si une archive manque, si la version n'a pas le format `X.Y.Z` ou si la version est une pré-version (aucun manifeste écrit)
- [X] T020 [P] [US2] Ajouter un job `krew-manifest` à `.github/workflows/ci.yml` : snapshot GoReleaser (`--skip=sign,docker,sbom`), génération d'un manifeste d'essai par `go run ./scripts/krew-manifest -checksums dist/checksums.txt -version <version du snapshot>` puis `validate-krew-manifest -skip-install` v0.5.0 (structure) et, pour chacune des quatre plateformes, `KREW_ROOT=<temporaire> KREW_OS=<os> KREW_ARCH=<arch> krew install --manifest=<manifeste> --archive=dist/<archive>` avec le binaire `krew` v0.5.0 (outils compilés avec la chaîne d'outils du projet), et présence de `LICENSE` installé (research R9, FR-011)

### Implementation for User Story 2

- [X] T021 [US2] Créer `packaging/krew/telemach.yaml.tmpl` (format de `contracts/krew-manifest.md`) : `shortDescription` et `description` en français, `homepage`, `caveats` en français (activer l'intégration shell avec `eval "$(kubectl-telemach shell-init zsh)"` ou `bash`, relancer le terminal), une entrée par plateforme
- [X] T022 [US2] Implémenter `scripts/krew-manifest/main.go` (bibliothèque standard, `text/template`) : options `-checksums`, `-version`, `-out`, `-base-url` (défaut : URL de la Release ; remplacée par une URL de fichiers locaux dans le job de T020) ; lecture de `checksums.txt`, contrôle des 4 archives, rendu du modèle de T021, refus d'une pré-version. Faire passer T019 et T020
- [X] T023 [US2] Créer `.github/workflows/krew-index.yml` (research R9) : déclencheurs `workflow_call` et `workflow_dispatch` lancé **sur le tag** ; `permissions: contents: read` ; `environment: release` ; vérification du format du tag (lu dans `env:`) et arrêt sans erreur pour une pré-version ; `gh release download "$TAG" --pattern checksums.txt` (Release publiée) ; génération par `scripts/krew-manifest` ; validation par `validate-krew-manifest` v0.5.0 ; clone de `git@github.com:aboigues/telemach-krew-index.git` avec la clé `KREW_INDEX_DEPLOY_KEY` (agent SSH, `known_hosts` de github.com renseigné à partir des empreintes publiées par GitHub, jamais `StrictHostKeyChecking=no`) ; écriture de `plugins/telemach.yaml` ; commit « Index : telemach vX.Y.Z » et push **sans `--force`** seulement si le fichier a changé (relance sans effet sinon, quickstart P3)
- [X] T024 [US2] Ajouter à `.github/workflows/release.yml` le job `krew-index` (`needs: release`, `uses: ./.github/workflows/krew-index.yml`) : c'est le job appelé qui déclare `environment: release` et lit `KREW_INDEX_DEPLOY_KEY` ; l'appel ne transmet aucun secret du dépôt
- [X] T025 [US2] Compléter `release.header` dans `.goreleaser.yaml` et rédiger dans `README.md` la section « Installer avec Krew » : commandes **identiques** à `contracts/krew-manifest.md` (`kubectl krew index add telemach https://github.com/aboigues/telemach-krew-index.git`, `kubectl krew install telemach/telemach`, `kubectl telemach version`, `kubectl krew upgrade telemach`), lien vers l'installation de Krew, encadré sur les deux installations concurrentes (`kubectl plugin list` signale un plugin masqué ; n'en garder qu'une)
- [X] T026 [US2] Valider l'installation Krew sur un **index de répétition** (D-LL-015) : dépôt git local contenant `plugins/telemach.yaml` généré depuis le snapshot avec des `uri` locales, `kubectl krew index add repetition <chemin>`, `kubectl krew install repetition/telemach`, `kubectl telemach version`, puis désinstallation et retrait de l'index ; consigner le résultat dans `specs/003-distribution/research.md` § R9

**Checkpoint**: manifeste généré et validé à chaque PR ; publication de l'index prête, relançable seule.

---

## Phase 5: User Story 5 - Publier une version en toute confiance (Priority: P2)

**Goal**: un tag poussé sur `main` publie les trois voies sans autre action ; un tag invalide
ou déjà publié ne publie rien.

**Independent Test**: quickstart P1 (pré-version), P2 (tag hors `main`), P3 (relance de
l'index seul) ; aucune exécution n'écrase un artefact.

### Implementation for User Story 5

- [X] T027 [US5] Compléter le job `verify` de `.github/workflows/release.yml` : contrôle « tag d'image absent » par l'API publique de Docker Hub (`https://hub.docker.com/v2/repositories/telemachlearning/telemach-kubectl/tags/$VERSION/` → 404 attendu, 200 → échec « déjà publiée », autre → échec explicite) ; messages d'erreur en français indiquant la marche à suivre (supprimer un brouillon échoué, publier une nouvelle version)
- [X] T028 [US5] Vérifier les permissions de chaque job de `ci.yml`, `release.yml` et `krew-index.yml` (lecture seule partout sauf `release`) et l'absence de toute expression `${{ … }}` dans un script `run:` (`grep` ajouté au test `tests/manifests/hygiene_test.go`, qui échoue sur une expression `${{` dans une ligne de script d'un workflow)
- [X] T029 [US5] Ajouter à `CONTRIBUTING.md` une section « Publier une version » : prérequis faits par le mainteneur (releases immuables du dépôt ; dépôt Docker Hub `telemachlearning/telemach-kubectl` et jeton Read & Write limité à ce dépôt, avec sa date d'expiration ; dépôt public `aboigues/telemach-krew-index` avec `plugins/` et README, clé de déploiement en écriture dont la clé privée devient `KREW_INDEX_DEPLOY_KEY` ; environnement `release` restreint aux tags `v*` avec ces secrets et la variable `DOCKERHUB_USERNAME`) ; publier (`git tag -a vX.Y.Z -m … && git push origin vX.Y.Z`) ; relancer la publication sur le tag (`gh workflow run release.yml --ref vX.Y.Z`) ; relancer la seule mise à jour de l'index (`gh workflow run krew-index.yml --ref vX.Y.Z`) ; que faire en cas d'échec (brouillon à supprimer, version suivante) ; revue trimestrielle des versions d'outils non suivies par Dependabot
- [ ] T030 [US5] Mettre à jour la protection de `main` : ajouter `build`, `goreleaser-check`, `image-scan`, `dockerfile-lint` et `krew-manifest` aux checks obligatoires en conservant les existants. **Proposer la commande à l'utilisateur** (réglage du dépôt)
- [ ] T031 [US5] Après fusion et **avec confirmation de l'utilisateur**, tester le refus d'un tag hors `main` (quickstart P2) : `v0.0.0-test.1` sur un commit d'une branche jetable ; attendu : `verify` échoue, `gates`, `release` et `krew-index` ne s'exécutent pas ; supprimer ensuite le tag et la branche

**Checkpoint**: chaîne de publication complète et sûre, documentée pour le mainteneur.

---

## Phase 6: User Story 3 - Installer le plugin avec Docker seulement (Priority: P3)

**Goal**: `docker run … telemachlearning/telemach-kubectl:X.Y.Z install` copie le bon binaire
sous le nom `kubectl-telemach` dans `~/.local/bin`, ou refuse avec une explication.

**Independent Test**: quickstart D1, D2 et D4 ; `kubectl telemach version` après installation ;
fichier `0755` appartenant à l'utilisateur.

### Tests for User Story 3 ⚠️

- [ ] T032 [P] [US3] Écrire `cmd/telemach-image/install_test.go` (adapté du plugin Docker), une table couvrant **toutes** les lignes « Installation par l'image » de `data-model.md` avec accès système injectables (racine des binaires, `/sortie`, table des montages, identité du processus) : succès linux et darwin (« `HOST_OS` : `linux` (défaut) ou `darwin` ; autre valeur refusée ») ; remplacement d'un `kubectl-telemach` dont le module Go est `github.com/aboigues/telemach-plugin-kubectl` (message avec ancienne et nouvelle version) ; refus, sans rien écrire : `/sortie` non monté, non accessible en écriture, appartenant à root alors que le processus ne l'est pas, fichier existant d'un autre module, **lien symbolique** (message qui évoque Krew et `kubectl krew list`), `HOST_OS=windows` (renvoi vers WSL2, code 2) ; écriture atomique `0755` ; message de succès conforme à `contracts/image-cli.md` (rappel du `PATH`, `shell-init`)
- [ ] T033 [P] [US3] Écrire `tests/integration/image_test.go` (étiquette `integration`, `t.Skip` sans démon Docker) qui reproduit **à la lettre** les commandes du README (LL-004) sur une image construite depuis un snapshot : installation Linux dans un dossier temporaire, `--user` de l'utilisateur, fichier `0755`, `kubectl-telemach version` du fichier installé égal à la version de l'image ; installation chronométrée, image déjà téléchargée, sous 60 s (SC-003, D-LL-012) ; refus sans montage et sans `--user`

### Implementation for User Story 3

- [ ] T034 [US3] Implémenter `cmd/telemach-image/install.go` selon research R7 et `contracts/image-cli.md` : détection du montage par `/proc/self/mountinfo` ; contrôles propriétaire root, écriture, fichier existant (`os.Lstat` : lien symbolique refusé ; `debug/buildinfo.ReadFile` : module `github.com/aboigues/telemach-plugin-kubectl`, ancienne version lue dans `Main.Version`) ; écriture atomique (fichier temporaire dans `/sortie`, `0755`, renommage) ; codes 0, 1, 2. Faire passer T032
- [ ] T035 [US3] Rédiger les messages de `install` dans `cmd/telemach-image/messages.go` : succès et chaque refus exactement selon `contracts/image-cli.md` (✔/✖, `📘 Concept`, `💡 Correction` avec la commande complète et la version de l'image) ; vocabulaire pédagogique : montage, identité du processus, `PATH`, plugin kubectl trouvé par son nom
- [ ] T036 [US3] Activer la partie image dans `.github/workflows/release.yml`, job `release` : `docker/setup-buildx-action` et `docker/login-action` (SHA de research), `username: ${{ vars.DOCKERHUB_USERNAME }}`, `password: ${{ secrets.DOCKERHUB_TOKEN }}` ; GoReleaser sans `--skip=docker`
- [ ] T037 [US3] Dans `.github/workflows/release.yml`, après GoReleaser : repérer le digest de l'index publié (`docker buildx imagetools inspect … --format '{{json .Manifest}}'`, refus si le digest n'a pas la forme `sha256:<64 hex>`), signer par digest (`cosign sign --yes docker.io/telemachlearning/telemach-kubectl@<digest>`), attester la provenance (`actions/attest`, `subject-name: docker.io/telemachlearning/telemach-kubectl`, `push-to-registry: true`)
- [ ] T038 [US3] Compléter `release.header` dans `.goreleaser.yaml` et rédiger dans `README.md` la section « Installer avec Docker seulement » : commandes Linux/WSL2 et macOS **identiques** à `contracts/image-cli.md` (`mkdir -p ~/.local/bin` d'abord, `--user`, tag `0.1.0` explicite) ; rôle de `-v`, de `--user`, de `HOST_OS` ; rappel du `PATH` ; pourquoi un tag explicite plutôt que `latest`
- [ ] T039 [P] [US3] Créer `docs/dockerhub.md` : texte en français de la page Docker Hub (présentation, deux usages, vérification `cosign verify` et `gh attestation verify` de `contracts/image-tags.md`, lien vers le dépôt et https://www.telemach-learning.fr), à coller par le mainteneur

**Checkpoint**: l'image installe le plugin ou explique pourquoi elle ne peut pas ; publication signée et attestée.

---

## Phase 7: User Story 4 - Essayer TELEMACH sans rien installer (Priority: P4)

**Goal**: `docker run --rm -t -v "$PWD":/manifestes:ro … check` donne la même vérification que
le plugin installé, sans écrire ni contacter de cluster.

**Independent Test**: quickstart D3 sur un cas fautif et un dossier vide.

### Tests for User Story 4 ⚠️

- [ ] T040 [P] [US4] Ajouter à `cmd/telemach-image/main_test.go` : `check` sans `-f` transmis comme `check -f /manifestes -R` ; `check -f …` transmis tel quel ; dossier visé inexistant ou sans aucun fichier `.yaml`/`.yml` (sous-dossiers compris) → message de l'image en français (aucun manifeste trouvé, concept de montage, commande d'essai complète avec la version), code 2, plugin non exécuté
- [ ] T041 [P] [US4] Ajouter à `tests/integration/image_test.go` : pour un cas valide et un cas fautif de `testdata/manifests/`, comparer la sortie de la commande d'essai du README (dossier monté en `:ro`) à celle de `kubectl-telemach check -f <dossier> -R` du binaire du snapshot (même texte, même code) ; aucun fichier créé dans le dossier ; dossier vide → message de l'image, code 2

### Implementation for User Story 4

- [ ] T042 [US4] Implémenter le pré-contrôle et la réécriture de `check` dans `cmd/telemach-image/check.go` (research R6) et les messages dans `messages.go`. Faire passer T040
- [ ] T043 [US4] Vérifier que `check` fonctionne dans l'image `scratch` sans `HOME`, sans `/tmp` inscriptible et sans kubeconfig (aucune écriture, aucun accès à un dossier d'état, aucune tentative de connexion), que la première ligne annonce les règles appliquées et que les couleurs s'activent avec `-t` ; corriger le plugin s'il tente d'écrire. Faire passer T041
- [ ] T044 [US4] Rédiger dans `README.md` la section « Essayer sans installer » : commande canonique de `contracts/image-cli.md`, rôle de `:ro` et de `-t`, absence de kubectl et de cluster dans l'image (règles de la version la plus récente, annoncées en première ligne)

**Checkpoint**: les trois voies et l'essai sont documentés, testés et publiables.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T045 [P] `go test -race ./...`, golangci-lint (Go 1.27.2), `goreleaser check`, hadolint sur `Dockerfile`, `validate-krew-manifest` sur un manifeste d'essai, `scripts/couverture 80`, `scripts/secrets` : tout au vert
- [ ] T046 [P] Relire `README.md`, `CONTRIBUTING.md`, `docs/dockerhub.md`, le modèle Krew et les notes de Release : français correct et accentué, chaque commande exécutable telle quelle avec un tag explicite, ordre de FR-024 respecté, liens valides
- [ ] T047 Passer la PR de brouillon à prête, attendre toutes les gates vertes ; l'utilisateur relit et fusionne
- [ ] T048 Préparer la première publication **avec l'utilisateur** : vérifier que les prérequis de `CONTRIBUTING.md` sont en place (réglages faits par lui), puis, avec sa confirmation, publier une pré-version de répétition `v0.1.0-rc.1` (quickstart P1 : Release pré-version, image `0.1.0-rc.1` seule, index Krew inchangé, relance refusée)
- [ ] T049 Première version : **avec confirmation de l'utilisateur**, poser et pousser `v0.1.0` sur `main` ; suivre `release.yml` jusqu'au bout et vérifier SC-005 (< 30 min) ; vérifier la Release (immuable, 4 binaires, 4 archives, 4 SBOM, sommes, signature, provenance), l'image (tags `0.1.0`, `0.1`, `latest`, taille, utilisateur, signature, provenance, binaires aux sommes de `checksums.txt`) et l'index Krew (commit « Index : telemach v0.1.0 ») ; consigner les résultats dans `specs/003-distribution/research.md` § « Résultats de la première publication »
- [ ] T050 Rejouer le quickstart sur les vrais fichiers : R1 à R4, K1, D1 à D4, Reproductibilité (Linux amd64 / WSL2) ; Linux arm64 si disponible ; macOS arm64 **par l'utilisateur** (SC-008) ; K2 et P3 à la version suivante
- [ ] T051 Consigner dans `LESSON-LEARNED.md` tout incident survenu pendant la feature (gate rouge, publication ratée, mauvaise manipulation), avec règle préventive et suivi (principe V)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** : aucune dépendance.
- **Foundational (Phase 2)** : dépend de Phase 1 ; bloque toutes les histoires. T009 dépend de
  T006 à T008 ; T011 de T010.
- **US1 (Phase 3)** : dépend de Phase 2. MVP.
- **US2 (Phase 4)** : dépend de US1 (fichiers de la Release, `release.yml`).
- **US5 (Phase 5)** : dépend de US1 ; T031 dépend de la fusion (T047).
- **US3 (Phase 6)** : dépend de Phase 2 (image) et de T015 (`release.yml`).
- **US4 (Phase 7)** : dépend de Phase 2 ; indépendante de US3 sauf `tests/integration/image_test.go`
  (T041 après T033).
- **Polish (Phase 8)** : après toutes les histoires ; T048 à T050 après la fusion.

### Within Each User Story

- Tests écrits avant l'implémentation, et d'abord rouges.
- `.goreleaser.yaml` et `release.yml` sont modifiés par plusieurs tâches successives (T008,
  T015, T016, T024, T025, T036 à T038) : jamais en parallèle.
- `README.md` est rédigé section par section dans l'ordre de FR-024 (T017, T025, T038, T044).

### Parallel Opportunities

- Phase 1 : T002, T003, T004.
- Phase 2 : T005 en parallèle de T007 et T008.
- US1 : T012 et T013.
- US2 : T019 et T020.
- US3 : T032, T033 et T039.
- US4 : T040 et T041 (fichiers différents).
- US3 et US4 peuvent avancer en parallèle après Phase 2 (fichiers distincts : `install.go` /
  `check.go`), sauf `messages.go` et `README.md` (séquentiels).

---

## Parallel Example: User Story 1

```bash
Task: "T012 tests/release/dist_test.go (contrat des artefacts)"
Task: "T013 tests/release/reproducible_test.go (reproductibilité)"
```

## Parallel Example: User Story 3

```bash
Task: "T032 cmd/telemach-image/install_test.go (table des cas de data-model.md)"
Task: "T033 tests/integration/image_test.go (commandes du README à la lettre)"
Task: "T039 docs/dockerhub.md"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 et Phase 2 (snapshot complet, gates vertes en PR brouillon).
2. Phase 3 (US1) : Release vérifiable, procédure manuelle documentée et testée sur snapshot.
3. **STOP and VALIDATE** : quickstart S1 ; à ce stade, une version pourrait déjà être publiée
   par la seule voie manuelle (`--skip=docker`, sans index Krew).

### Incremental Delivery

1. US1 → Release.
2. US2 → index Krew (voie habituelle des utilisateurs de kubectl).
3. US5 → garde-fous de publication et procédure du mainteneur.
4. US3 puis US4 → image (installation, puis essai).
5. Polish → pré-version de répétition, première version `v0.1.0`, quickstart sur les vrais
   fichiers.

---

## Notes

- [P] = fichiers différents, pas de dépendance sur une tâche inachevée.
- Les tâches qui publient, poussent ou modifient un réglage du dépôt demandent toujours la
  confirmation de l'utilisateur ; les secrets restent hors de portée de Claude Code.
- Relire `LESSON-LEARNED.md` avant chaque phase ; consigner tout incident dans la PR qui le
  corrige.
