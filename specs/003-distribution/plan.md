# Implementation Plan: Distribution du plugin sans Go

**Branch**: `003-distribution` | **Date**: 2026-10-19 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-distribution/spec.md`

## Summary

Aujourd'hui, installer TELEMACH pour kubectl exige Go 1.27 et une compilation. Cette feature
publie le plugin par trois voies, sans Go, à partir d'un seul tag `vX.Y.Z` posé sur `main` :
une **Release GitHub** immuable (binaires et archives pour linux et darwin, sommes, SBOM,
signature keyless, provenance SLSA), un **index Krew** propre au projet, mis à jour après
chaque version stable, et une **image Docker Hub** dédiée `telemachlearning/telemach-kubectl`
qui installe le plugin dans un dossier du `PATH` ou vérifie des manifestes sans rien installer.

La chaîne reprend celle du plugin TELEMACH Docker, publiée avec succès en v0.2.0
(`002-distribution`), avec trois adaptations (voir [research.md](research.md)) :
- **archives `.tar.gz`** en plus des binaires bruts, car Krew n'installe que des archives (R2) ;
- **index Krew** dans un dépôt dédié, écrit par une clé de déploiement, par un workflow séparé
  lancé après la publication et relançable seul (R9) ;
- **installation par l'image** dans `~/.local/bin` sous le nom `kubectl-telemach`, avec rappel
  du `PATH` et refus d'un lien symbolique (installation Krew probable) (R7) ; essai `check`
  sur `/manifestes`, sans aucun kubeconfig (R6).

## Technical Context

**Language/Version**: Go 1.27.2 (inchangé, `toolchain` de `go.mod`).

**Primary Dependencies**: aucune nouvelle dépendance Go. Outils de CI seulement (versions
vérifiées le 2026-10-19, research.md) : GoReleaser OSS v2.18.3, cosign v3.1.3, Syft v1.54.1,
Grype v0.120.1, hadolint v2.15.1, `validate-krew-manifest` (Krew v0.5.0).

**Storage**: aucun chez l'apprenant ; artefacts publiés (Release, image, index Krew).

**Testing**: `go test` ; test de contrat des artefacts `tests/release` (étiquette `dist`,
snapshot GoReleaser : noms, sommes, module, version, archives, reproductibilité) ; tests de
`cmd/telemach-image` (refus, messages, transmission au plugin) ; test du générateur
`scripts/krew-manifest` (sommes, plateformes manquantes) ; `validate-krew-manifest` en CI ;
scan Grype et hadolint ; quickstart rejoué sur les vrais fichiers après publication.

**Target Platform**: binaires linux et darwin (amd64, arm64) ; image linux/amd64 et
linux/arm64 ; WSL2 compris ; Windows natif hors périmètre.

**Project Type**: CLI, plugin kubectl ; chaîne de publication (GitHub Actions).

**Performance Goals**: installation par la Release < 5 min (SC-001), par l'image < 1 min une
fois téléchargée (SC-003) ; trois voies disponibles < 30 min après le tag (SC-005).

**Constraints**: image < 15 Mo compressés par architecture (SC-007, ~10 Mo estimés) ; image
`scratch`, non-root, sans shell ni kubeconfig ; jetons et clés uniquement dans l'environnement
« release » ; aucune modification manuelle de l'index Krew.

**Scale/Scope**: 4 plateformes × (binaire + archive + SBOM) ; 1 image multi-architecture ; 1
manifeste Krew ; 3 workflows (`ci.yml` étendu, `release.yml`, `krew-index.yml`) ; 2 nouveaux
programmes Go (`cmd/telemach-image`, `scripts/krew-manifest`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principe | Vérification | Statut |
|----------|--------------|--------|
| **I. Vocation pédagogique** | Chaque message de l'image explique la notion en jeu (montage, identité du processus, `PATH`, plugin kubectl trouvé par son nom) et donne la commande corrigée ; le `caveats` du manifeste Krew et les notes de Release donnent la prochaine étape (`shell-init`) ; le README rappelle pourquoi un tag explicite est préférable. | ✅ |
| **II. Langue** | README, notes de Release, manifeste Krew, messages de l'image, `CONTRIBUTING`, `docs/dockerhub.md` en français ; code et identifiants en anglais. | ✅ |
| **III. Sécurité** | Binaires vérifiables (sommes, signature keyless, provenance), Release immuable, image `scratch` non-root sans shell ni kubeconfig, signée et scannée ; `install` n'écrit qu'un fichier, jamais en root, jamais par-dessus un fichier inconnu ; secrets dans un environnement protégé restreint aux tags, clé d'index limitée à un dépôt ; outils épinglés sur des versions vérifiées ce jour ; aucune installation documentée qui exige `sudo`. | ✅ |
| **IV. Gates GitHub Actions** | `ci.yml` devient réutilisable et gagne `build` (version injectée), `goreleaser-check` (snapshot + contrat), `image-scan` (Grype, bloquant ≥ high), `dockerfile-lint` (hadolint, désormais exigé puisqu'un Dockerfile existe) et `krew-manifest` (validation) ; les nouvelles gates deviennent obligatoires sur `main`. Publication : actions épinglées par SHA, `permissions: {}` au niveau du workflow, écriture limitée au job `release`, aucune interpolation `${{ }}` dans les scripts, mêmes gates rejouées sur le commit du tag. | ✅ |
| **V. LESSON-LEARNED** | Relu avant `/speckit-specify` et `/speckit-plan` ; leçons du projet Docker sur la distribution (LL-001, LL-004 à LL-007 Docker) et du projet (LL-004, LL-007, LL-008) appliquées : research.md R14. | ✅ |

**Re-check post-design** : les contrats n'exposent aucun secret ; `install` et `check`
n'écrivent qu'à l'endroit monté pour cela (ou nulle part) ; le manifeste Krew ne référence que
des fichiers de la Release publiée, aux sommes de `checksums.txt` ; la mise à jour de l'index
n'utilise pas le jeton GitHub du dépôt. Aucun écart à justifier.

## Project Structure

### Documentation (this feature)

```text
specs/003-distribution/
├── plan.md              # Ce fichier
├── research.md          # Phase 0 : versions vérifiées, décisions R1 à R14
├── data-model.md        # Phase 1 : Version, Release, manifeste, index, image, environnement
├── quickstart.md        # Phase 1 : validation de bout en bout (snapshot, Release, Krew, image)
├── contracts/
│   ├── release-assets.md  # fichiers de la Release, vérifications, notes
│   ├── krew-manifest.md   # index et manifeste Krew
│   ├── image-cli.md       # interface de l'image (install, check, aide)
│   └── image-tags.md      # tags de l'image, signature, provenance
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/
├── kubectl-telemach/          # plugin (inchangé, version déjà injectable)
└── telemach-image/            # NOUVEAU : point d'entrée de l'image (install, check, aide)
    ├── main.go                #   aiguillage, aide, transmission au plugin (syscall.Exec)
    ├── install.go             #   copie dans /sortie, refus pédagogiques
    ├── check.go               #   dossier /manifestes, « aucun manifeste »
    ├── messages.go            #   textes en français
    └── *_test.go
scripts/
└── krew-manifest/             # NOUVEAU : génère plugins/telemach.yaml depuis checksums.txt
    ├── main.go
    └── main_test.go
packaging/
└── krew/telemach.yaml.tmpl    # NOUVEAU : modèle du manifeste (textes en français)
tests/
└── release/                   # NOUVEAU : contrat des artefacts (étiquette dist)
    ├── dist_test.go
    └── reproducible_test.go
docs/
└── dockerhub.md               # NOUVEAU : texte de la page Docker Hub
.goreleaser.yaml               # NOUVEAU
Dockerfile                     # NOUVEAU (scratch, copie de binaires)
.dockerignore                  # NOUVEAU
.github/workflows/
├── ci.yml                     # + workflow_call, build, goreleaser-check, image-scan,
│                              #   dockerfile-lint, krew-manifest
├── release.yml                # NOUVEAU : verify → gates → release → krew-index
└── krew-index.yml             # NOUVEAU : génère, valide et pousse le manifeste (relançable)
.golangci.yml                  # + étiquette de build dist (D-LL-007)
README.md, CONTRIBUTING.md     # installation (ordre de FR-024), publication et prérequis
```

**Structure Decision**: projet unique, comme les features précédentes. Le point d'entrée de
l'image est un second programme (`cmd/telemach-image`) plutôt qu'une sous-commande du plugin,
pour ne pas ajouter au plugin une commande `install` qui n'a de sens que dans l'image (Docker
R5). Le générateur du manifeste est un outil de dépôt (`scripts/`), comme `gen-schemas` et les
collecteurs ; il n'est jamais livré.

## Complexity Tracking

Aucun écart à la constitution.

Point d'attention, hors constitution : un second dépôt (`aboigues/telemach-krew-index`) et une
seconde autorisation d'écriture (clé de déploiement) sont nécessaires. C'est le seul moyen de
publier l'index sans contourner la protection de `main` (principe IV) ; l'alternative « même
dépôt » est écartée en research.md R9.
