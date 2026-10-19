# Recherche — 003 Distribution du plugin sans Go

La chaîne de publication reprend celle du plugin TELEMACH Docker
([`002-distribution`](../../../telemach-plugin-docker/specs/002-distribution/research.md), publiée
avec succès en v0.2.0 le 2026-10-15). Ce document ne détaille que ce qui change pour kubectl ;
une décision reprise telle quelle renvoie à sa section d'origine (« Docker R… »).

Toutes les versions ci-dessous ont été **revérifiées le 2026-10-19** via l'API GitHub
(`gh api repos/<owner>/<repo>/releases/latest`), conformément à la règle globale et à D-LL-004 ;
les SHA de commit des actions ont été résolus à partir des tags (déréférencement des tags
annotés), pas recopiés du projet Docker.

## Versions retenues

| Outil / action | Version | Épinglage |
|---|---|---|
| GoReleaser (OSS) | v2.18.3 (2026-10-09) | `version: v2.18.3` dans l'action |
| `goreleaser/goreleaser-action` | v7.3.0 | `a4f02ab5355a589de60d917a659ce617ed19b3de` |
| cosign | v3.1.3 (2026-08-06) | `cosign-release: v3.1.3` |
| `sigstore/cosign-installer` | v4.1.2 | `6f9f17788090df1f26f669e9d70d6ae9567deba6` |
| `actions/attest` | v4.2.2 | `1e69f48acb82d1966a394da916b4c1698aa569d6` |
| `docker/setup-buildx-action` | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| `docker/login-action` | v4.6.0 | `dbcb813823bdd20940b903addbd779551569679f` |
| `anchore/scan-action` (Grype) | v7.4.2 | `27805bf3b4e84b4a5c980df22ed233c00390a439`, `grype-version: v0.120.1` (2026-10-06) |
| Syft (SBOM) | v1.54.1 (2026-10-06) | `anchore/sbom-action/download-syft` v0.24.3 `66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c`, `syft-version: v1.54.1` |
| hadolint | v2.15.1 (2026-07-31) | binaire téléchargé, somme SHA-256 vérifiée (comme le job Docker) |
| Krew (`validate-krew-manifest`) | v0.5.0 (2026-02-26) | `go install sigs.k8s.io/krew/cmd/validate-krew-manifest@v0.5.0`, compilé avec la chaîne d'outils du projet (D-LL-001) |

Les actions déjà présentes (`actions/checkout`, `actions/setup-go`, `golangci-lint-action`)
gardent l'épinglage de `ci.yml`. Les versions d'outils fixées dans les actions ne sont pas
suivies par Dependabot : elles sont revues chaque trimestre (CONTRIBUTING).

### Licence et maintenance (constitution, « Chaîne d'approvisionnement »)

Outils de CI uniquement : aucun n'est embarqué dans le plugin ni dans l'image. GoReleaser,
cosign, Syft, Grype et les actions : voir le tableau Docker (licences MIT et Apache-2.0,
projets actifs). Ajout propre à cette feature :

| Projet | Licence | Maintenance |
|---|---|---|
| kubernetes-sigs/krew | Apache-2.0 | actif (push 2026-10-19), ~7 k étoiles, projet SIG CLI |

Aucune nouvelle dépendance Go dans `go.mod` : le point d'entrée de l'image n'utilise que la
bibliothèque standard et `internal/render` ; `validate-krew-manifest` n'est installé qu'en CI.

---

## R1 — Outil de construction et de publication

- **Décision** : GoReleaser OSS v2.18.3, comme le plugin Docker (Docker R1). Deux builds :
  `kubectl-telemach` (`./cmd/kubectl-telemach`, linux/darwin × amd64/arm64) et
  `telemach-image` (`./cmd/telemach-image`, linux seulement, jamais publié dans la Release).
  Options de compilation : `CGO_ENABLED=0`, `-trimpath`, `-s -w -buildid= -X main.version=…`,
  `mod_timestamp: {{ .CommitTimestamp }}` (FR-005, Docker R11).

## R2 — Binaires bruts **et** archives (FR-003)

- **Constat** : Krew n'installe que des archives (`.tar.gz` ou `.zip`) ; un `uri` qui désigne
  un binaire brut est refusé par `validate-krew-manifest`.
- **Décision** : deux entrées `archives` GoReleaser sur le même build :
  - `formats: [binary]` → `kubectl-telemach_X.Y.Z_<os>_<arch>` (voie manuelle, même usage que
    le plugin Docker : un seul fichier à télécharger et vérifier) ;
  - `formats: [tar.gz]` → `kubectl-telemach_X.Y.Z_<os>_<arch>.tar.gz` contenant
    `kubectl-telemach`, `LICENSE` et `NOTICE` à la racine (voie Krew).
  `checksums.txt` couvre les huit fichiers ; le SBOM porte sur les quatre binaires.
- **Alternatives** : archives seulement (la procédure manuelle doit alors décompresser : une
  étape de plus pour un débutant) ; zip (inutile, aucune plateforme Windows).

## R3 — Signature et provenance (keyless)

- **Décision** : reprise de Docker R3 : signature cosign keyless du fichier de sommes
  (`checksums.txt.sigstore.json`), attestation SLSA par `actions/attest` sur chaque ligne de
  `checksums.txt` (binaires et archives), image signée par digest et attestée.

## R4 — Releases immuables (FR-006)

- **Décision** : reprise de Docker R4 : réglage « releases immuables » du dépôt, Release
  créée en brouillon par GoReleaser, attestations, puis publication ; le job `verify` refuse un
  tag dont la Release (même brouillon) ou le tag d'image existe déjà.
- **Action mainteneur** : activer le réglage (Settings › General › Releases).

## R5 — Image : base, contenu, point d'entrée

- **Décision** : reprise de Docker R5 avec les noms de ce dépôt :
  - `FROM scratch`, `USER 65532:65532`, aucun `VOLUME`, labels OCI ;
  - contenu : `/opt/telemach/linux/kubectl-telemach`, `/opt/telemach/darwin/kubectl-telemach`
    (architecture de l'image) et `/usr/local/bin/telemach-image` ;
  - `WORKDIR /manifestes` (dossier monté pour l'essai) ;
  - aucun kubeconfig, aucune variable `KUBECONFIG` : le plugin ne peut contacter aucun cluster
    depuis l'image (FR-014, FR-016).
- **Point d'entrée** `cmd/telemach-image` (nouveau programme, adapté de celui du plugin
  Docker, même auteur, même licence) : `install` → R7 ; `--help` ou aucun argument → aide
  (FR-019) ; `check` → R6 ; toute autre commande (`version`…) → remplacement du processus par le
  binaire linux (`syscall.Exec`).
- **Taille** : mesuré le 2026-10-19, le binaire linux/amd64 compilé avec les options de R1
  pèse 9,3 Mo (3,6 Mo compressé). L'image (plugin linux et darwin, point d'entrée) devrait donc
  faire environ 10 Mo compressés par architecture, sous SC-007 (15 Mo) ; la taille réelle est
  vérifiée sur le premier snapshot d'image.
- **Résultats de T009 (2026-10-19, GoReleaser v2.18.3, snapshot local sans image)** : 4
  binaires et 4 archives produits ; chaque archive contient exactement `LICENSE`, `NOTICE` et
  `kubectl-telemach`, dont la somme est celle du binaire brut (vérifié pour linux/amd64 et
  darwin/arm64) ; `version` affiche `TELEMACH PLUGIN KUBECTL 0.0.0-SNAPSHOT-5e5822d`. Tailles
  compressées : plugin linux/amd64 3,6 Mo, darwin/amd64 3,7 Mo, point d'entrée 2,5 Mo, soit
  ~9,8 Mo pour l'image amd64. Docker ne répondant pas sur la machine de développement, le
  contenu de l'image (binaires darwin présents, utilisateur, taille réelle) est vérifié par
  le job `image-scan` de la CI.

## R6 — Essai `check` dans l'image (FR-016)

- **Constat** : `kubectl telemach check` exige `-f` (Kubernetes n'a pas de fichier par
  défaut) et n'exécute jamais kubectl : il lit seulement `TELEMACH_KUBECTL_VERSION`, absente
  dans l'image, et applique alors les règles de la version la plus récente en le disant en
  première ligne. Aucune connexion au cluster n'est possible ni tentée.
- **Décision** : dans l'image, `check` sans `-f` devient `check -f /manifestes -R` (dossier
  monté, sous-dossiers compris) ; avec `-f`, les arguments sont transmis tels quels. Avant de
  lancer le plugin, l'image vérifie que le dossier visé contient au moins un fichier `.yaml`
  ou `.yml` ; sinon elle explique le montage (bind mount) et donne la commande complète
  (scénario US4-3), comme le fait l'image Docker pour les fichiers Compose.
- **Alternatives** : laisser le plugin répondre « aucun fichier » (il ne sait pas qu'il tourne
  dans un conteneur et ne peut pas parler du montage).

## R7 — Comportement de `install` (FR-017, FR-018)

- **Décision** : reprise de Docker R7, avec trois différences :
  1. **Nom installé** : `kubectl-telemach` (kubectl cherche les plugins `kubectl-*` dans le
     `PATH`) ;
  2. **Destination documentée** : `~/.local/bin`, montée sur `/sortie` ; le message final
     rappelle que le dossier doit être dans le `PATH` et donne la ligne à ajouter au fichier de
     configuration du shell. L'image ne peut pas lire le `PATH` de l'hôte : le rappel est
     systématique, sans affirmer que le dossier en est absent ;
  3. **Fichier existant** : remplacé seulement s'il s'agit d'un fichier ordinaire dont les
     informations de build Go désignent le module `github.com/aboigues/telemach-plugin-kubectl`.
     Un **lien symbolique** est refusé : c'est notamment ce que crée Krew dans
     `~/.krew/bin` ; le message explique qu'une installation Krew existe et comment n'en
     garder qu'une.
- Inchangé : `HOST_OS` = `linux` (défaut) ou `darwin`, `windows` → renvoi vers WSL2 ; refus si
  `/sortie` n'est pas monté, n'est pas accessible en écriture, ou appartient à root alors que
  le processus ne l'est pas ; écriture atomique (fichier temporaire, `0755`, renommage) ;
  ancienne version lue dans `Main.Version` des informations de build.

## R8 — Gates de l'image et du Dockerfile (FR-020)

- **Décision** : reprise de Docker R8 : job `image-scan` (Grype, sévérité `high` bloquante)
  sur l'image snapshot amd64 de chaque PR, et job `dockerfile-lint` (hadolint) **ajouté** à
  `ci.yml` : la constitution (principe IV) l'exige dès qu'un Dockerfile existe, ce qui sera le
  cas avec cette feature. Les deux deviennent des checks obligatoires de `main`.
- Trivy reste écarté (compromission de `trivy-action` en mars 2026, Docker R8).

## R9 — Index Krew du projet (FR-008 à FR-011)

- **Constat** : un index Krew propre est un dépôt git public contenant un dossier `plugins/` à
  sa racine, avec un manifeste par plugin
  ([guide Krew, « Hosting Custom Plugin Indexes »](https://krew.sigs.k8s.io/docs/developer-guide/custom-indexes/)).
  Krew clone le dépôt et suit sa **branche par défaut**.
- **Décision — hébergement** : dépôt **dédié** `aboigues/telemach-krew-index` (créé par le
  mainteneur), branche par défaut `main`, contenant `plugins/telemach.yaml` et un README en
  français. Installation documentée :
  `kubectl krew index add telemach https://github.com/aboigues/telemach-krew-index.git` puis
  `kubectl krew install telemach/telemach`.
- **Décision — écriture** : une **clé de déploiement SSH** en écriture, limitée par nature à ce
  seul dépôt, créée par le mainteneur ; sa clé privée est le secret `KREW_INDEX_DEPLOY_KEY` de
  l'environnement « release » (FR-021). Aucune autre autorisation.
- **Décision — moment** : la mise à jour de l'index est un workflow séparé
  `.github/workflows/krew-index.yml`, appelé par `release.yml` **après** la publication de la
  Release (les adresses d'un brouillon ne sont pas téléchargeables) et lançable seul sur un tag
  (`gh workflow run krew-index.yml --ref vX.Y.Z`) pour une relance sans republier (FR-010). Il
  ne fait rien pour une pré-version (FR-008).
- **Décision — génération** : petit programme Go `scripts/krew-manifest` (bibliothèque
  standard) qui lit `checksums.txt` de la Release publiée (`gh release download`), vérifie que
  les quatre archives y figurent, et écrit le manifeste à partir d'un modèle versionné
  `packaging/krew/telemach.yaml.tmpl` (textes en français, `caveats` avec la prochaine étape
  `kubectl-telemach shell-init`). Le manifeste est ensuite validé par `validate-krew-manifest`
  avant le commit dans l'index (« Index : telemach vX.Y.Z »), poussé par la clé de déploiement.
- **Gate de PR (FR-011)** : job `krew-manifest` de `ci.yml` : génère un manifeste d'essai à
  partir du `checksums.txt` du snapshot GoReleaser et le passe à `validate-krew-manifest`
  (qui vérifie aussi, sur les archives locales, que `bin` et `files` existent bien).
- **Validation sans réseau (vérifiée le 2026-10-19 dans le code de Krew v0.5.0, analyse A1)** :
  l'étape d'installation de `validate-krew-manifest` lance `kubectl krew install --manifest`,
  qui télécharge l'`uri` par HTTP (`http://` accepté, `file://` non) ; `krew install
  --manifest=… --archive=<fichier>` installe en revanche depuis une archive locale. Le job
  `krew-manifest` fait donc `validate-krew-manifest -skip-install` (structure), puis, pour
  chacune des quatre plateformes, `KREW_ROOT=<temporaire> KREW_OS=<os> KREW_ARCH=<arch> krew
  install --manifest=… --archive=dist/<archive>` (binaire `krew` v0.5.0 compilé avec la chaîne
  d'outils du projet, sans kubectl ni réseau) et vérifie la présence de `LICENSE` installé.
  L'index de répétition (T026), qui doit vraiment télécharger, sert `dist/` par un serveur
  HTTP local (`http://127.0.0.1:<port>/`).
- **Alternatives** :
  - **Même dépôt** (dossier `plugins/` sur `main`) : `main` est protégée et ne reçoit que des PR
    à gates vertes ; la publication devrait ouvrir et fusionner sa propre PR, ou contourner la
    protection (contraire au principe IV).
  - **`krews` de GoReleaser** : il écrit le manifeste pendant `goreleaser release`, donc
    **avant** la publication du brouillon (adresses encore privées) et sans relance séparée ;
    il demande en outre un jeton d'accès personnel au lieu d'une clé limitée à un dépôt.
  - **Jeton d'accès personnel à grain fin** : lié au compte du mainteneur, expiration à gérer ;
    une clé de déploiement est plus étroite (un seul dépôt, rien d'autre).
  - **Index officiel `krew-index`** : hors périmètre (spec) ; le même manifeste pourra y être
    proposé plus tard.

## R10 — Enchaînement de la publication (FR-001, FR-002, FR-021, FR-022)

- **Décision** : reprise de Docker R9 :
  - `release.yml` : `push` de tags `v*` et `workflow_dispatch` lancé **sur le tag** ;
    `permissions: {}` au niveau du workflow ; job `verify` (lecture seule : format du tag lu
    dans une variable d'environnement, tag ancêtre de `origin/main`, absence de Release et de
    tag d'image `telemachlearning/telemach-kubectl:X.Y.Z`) ; job `gates` (appel de `ci.yml`,
    rendu réutilisable par `workflow_call`) ; job `release` (`environment: release`, seul à
    écrire : `contents`, `id-token`, `attestations`) ; puis job `krew-index` (appel de
    `krew-index.yml`, `environment: release`, `contents: read` seulement : l'écriture passe
    par la clé de déploiement, pas par le jeton GitHub du dépôt).
  - Environnement « release » : restreint aux tags `v*`, secrets `DOCKERHUB_TOKEN` et
    `KREW_INDEX_DEPLOY_KEY`, variable `DOCKERHUB_USERNAME`, aucune approbation.

## R11 — Notes de Release en français (FR-007)

- **Décision** : reprise de Docker R10 (changelog git groupé feat / fix / autres, `header` et
  `footer` en français) ; le `header` donne les trois voies : Release (avec la commande de
  vérification), Krew, image.

## R12 — Version et contrôle de la compilation

- **Constat** : `main.version` existe déjà (`"dev"` par défaut) et `kubectl telemach version`
  affiche `TELEMACH PLUGIN KUBECTL <version>` en première ligne ; la CI ne vérifie pas encore
  l'injection.
- **Décision** : job `build` ajouté à `ci.yml` (comme Docker) : version issue de
  `git describe --tags --match 'v[0-9]*'`, compilation avec `-X main.version=…`, et contrôle de
  la première ligne de `kubectl-telemach version`. Le test de contrat des artefacts
  (`tests/release`, étiquette de build `dist`) vérifie en plus les noms, les sommes, le module
  et la version de chaque binaire et de chaque archive, et la reproductibilité (FR-005).
- **D-LL-007** : l'étiquette `dist` est ajoutée à `run.build-tags` de `.golangci.yml`.

## R13 — Documentation (FR-024 à FR-026)

- **Décision** : README réorganisé dans l'ordre de FR-024, chaque commande avec un tag de
  version explicite ; `docs/dockerhub.md` (texte de la page Docker Hub, collé par le
  mainteneur) ; `CONTRIBUTING.md` : publication, relance de la Release, relance de la seule
  mise à jour de l'index Krew, prérequis (releases immuables, environnement « release »,
  secrets, dépôt de l'index et sa clé de déploiement, dépôt Docker Hub).
- **Test des procédures (FR-025, D-LL-006, LL-004)** : avant la v0.1.0, les commandes du
  README sont rejouées sur un snapshot (Release simulée par les fichiers de `dist/`), puis,
  après publication, sur les vrais fichiers (quickstart). L'installation Krew se valide sur un
  index de répétition (dépôt temporaire ou clone local) avant la première vraie mise à jour.

## R14 — Leçons appliquées (principe V)

- **D-LL-001** : `validate-krew-manifest` compilé avec la chaîne d'outils du projet.
- **D-LL-004** : versions vérifiées ce jour (tableau ci-dessus) ; image de base `scratch`,
  sans tag à choisir.
- **D-LL-005** : prérequis du dépôt, de l'index et de Docker Hub documentés dans
  `CONTRIBUTING.md` et vérifiés par une PR de test avant la première version.
- **D-LL-006 / LL-004** : procédures du README rejouées telles qu'écrites, de bout en bout.
- **D-LL-007** : étiquette de build `dist` déclarée au lint.
- **D-LL-008** : validations enchaînées par `&&` avant chaque commit.
- **D-LL-015** : les essais de publication visent un tag de pré-version ou un index de
  répétition, jamais l'index réel ni une version déjà publiée.
- **LL-007** : la compilation de vérification écrit dans un dossier temporaire ; aucun binaire
  dans le dépôt (`TestNoBinaryCommitted`).
- **LL-008** : les attentes des tests de l'image s'appuient sur des sorties réellement
  observées (snapshot GoReleaser), pas sur des messages supposés.
