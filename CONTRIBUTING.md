# Contribuer à TELEMACH PLUGIN KUBECTL

Merci de ton intérêt ! Ce projet est un outil pédagogique : la qualité des explications compte
autant que celle du code. Les règles ci-dessous découlent de la
[constitution](.specify/memory/constitution.md).

## Avant de commencer

- Relis [`LESSON-LEARNED.md`](LESSON-LEARNED.md) : chaque incident passé, y compris ceux du
  projet frère TELEMACH PLUGIN DOCKER, y laisse une règle préventive à appliquer.
- Toute nouvelle fonctionnalité passe par Spec Kit (`/speckit-specify` → `/speckit-plan` →
  `/speckit-tasks` → `/speckit-implement`).

## Langue

- **En français** : spécifications, documentation, commentaires du code, messages affichés par
  le plugin, catalogue d'erreurs, `LESSON-LEARNED.md`. Accents obligatoires.
- **En anglais** : le code (identifiants, noms de fichiers de code), les noms de branches, les
  clés de configuration.
- Les termes, commandes et champs Kubernetes restent dans leur forme d'origine.

## Sécurité

- Ne versionne jamais de secret ni de kubeconfig réel. Les seuls faux secrets autorisés sont
  dans `testdata/secrets/` et `testdata/pods/`.
- Toute nouvelle dépendance est justifiée dans le plan de la feature et épinglée sur la dernière
  version stable vérifiée.
- Toute lecture du cluster passe par `internal/kuberead` (liste fermée testée) ; le plugin ne
  modifie jamais un fichier de l'apprenant ni une ressource d'un cluster.
- **Tests** : uniquement sur un cluster **kind jetable** créé par le test lui-même. Ne lance
  jamais les tests d'intégration contre ton contexte kubectl habituel (D-LL-015).

## Gates et fusion

La branche `main` est protégée :

- pas de push direct, pas de force-push, pas de suppression ;
- fusion uniquement par pull request, historique linéaire, conversations résolues ;
- aucune exception pour les administrateurs (`enforce_admins`) ;
- checks obligatoires : `lint`, `test`, `vuln`, `secrets`, `dependency-review`,
  `integration`, `manifest-lint`, et depuis la feature 003 `build`, `goreleaser-check`,
  `image-scan`, `dockerfile-lint`, `krew-manifest`.

**Prérequis du dépôt** : le Dependency graph et les alertes Dependabot doivent être activés
(Settings › Code security), sans quoi la gate `dependency-review` échoue (D-LL-005).

**Workflow de fusion** : Claude Code commit, pousse et ouvre la pull request ; le mainteneur
relit la PR puis la fusionne lui-même. Ce clic sur « Merge » vaut validation humaine : aucune
approbation formelle n'est exigée, GitHub interdisant d'approuver sa propre PR.

## Publier une version

La publication est automatisée par `.github/workflows/release.yml` : elle produit la Release
GitHub (binaires et archives des quatre plateformes, sommes signées, SBOM, provenance),
l'image Docker Hub `telemachlearning/telemach-kubectl` (signée, multi-architecture), puis met à
jour l'index Krew du projet par `.github/workflows/krew-index.yml`. Détail :
`specs/003-distribution/`.

### Prérequis (une seule fois, par le mainteneur — D-LL-005)

- **Releases immuables** activées (Settings › General › Releases) : une Release publiée ne
  peut plus être modifiée et son tag ne peut plus être déplacé.
- **Docker Hub** : dépôt public `telemachlearning/telemach-kubectl` ; jeton d'accès personnel
  « Read & Write », limité à ce dépôt si le compte le permet. **Noter sa date d'expiration**
  et le renouveler avant.
- **Index Krew** : dépôt public `aboigues/telemach-krew-index`, branche par défaut `main`,
  contenant un dossier `plugins/` (avec un fichier `.gitkeep`) et un README en français.
  Y ajouter une **clé de déploiement** (Settings › Deploy keys) avec l'accès en écriture,
  créée par exemple avec `ssh-keygen -t ed25519 -N "" -C "telemach krew-index" -f krew-index`
  sur une machine de confiance ; la clé publique (`krew-index.pub`) va dans le dépôt de
  l'index, la clé privée (`krew-index`) devient le secret ci-dessous, puis les deux fichiers
  locaux sont supprimés. Cette clé n'ouvre que ce dépôt.
- **GitHub, environnement `release`** (Settings › Environments) : déploiement réservé aux
  tags `v*` ; variable `DOCKERHUB_USERNAME` ; secrets `DOCKERHUB_TOKEN` (le jeton Docker Hub)
  et `KREW_INDEX_DEPLOY_KEY` (la clé privée de l'index). Ces secrets n'existent que dans cet
  environnement : aucun autre workflow, ni aucune branche, ne peut les lire.

Claude Code ne manipule jamais ces secrets.

### Procédure

1. Fusionner sur `main` tout ce qui doit être publié (gates vertes).
2. Poser un tag annoté sur `main` et le pousser :

   ```bash
   git checkout main && git pull
   git tag -a vX.Y.Z -m "TELEMACH X.Y.Z — résumé"
   git push origin vX.Y.Z
   ```

3. Suivre l'exécution (`gh run watch`). Le workflow vérifie le tag (format, commit de `main`,
   Release et image absentes), rejoue toutes les gates, publie la Release et l'image, puis
   met à jour l'index Krew (versions stables seulement).
4. Vérifier la Release, l'image et l'index (quickstart de la feature 003).

Une **pré-version** (`vX.Y.Z-rc.N`) est publiée comme pré-version : elle ne déplace ni la
« dernière version » des Releases, ni les tags `X.Y` et `latest` de l'image, et ne modifie pas
l'index Krew.

### En cas d'échec

- **Avant la publication** (vérifications, gates, construction) : rien n'est publié, ou il
  reste un brouillon de Release. Supprimer le brouillon (`gh release delete vX.Y.Z`), corriger
  sur `main` si nécessaire, puis relancer **sur le tag** :
  `gh workflow run release.yml --ref vX.Y.Z`. Ne jamais lancer depuis une branche :
  l'identité des signatures serait celle de la branche et les vérifications documentées
  échoueraient. Si l'image `X.Y.Z` a déjà été poussée, la relance est refusée : publier une
  nouvelle version. `X.Y` et `latest` ne sont déplacés qu'une fois l'image signée et attestée :
  après un échec, ils désignent toujours la version précédente.
- **Seule la mise à jour de l'index Krew a échoué** (clé absente ou expirée, dépôt
  inaccessible) : la Release et l'image restent valables. Corriger la cause, puis relancer la
  seule mise à jour **sur le tag** : `gh workflow run krew-index.yml --ref vX.Y.Z`. Sans
  changement du manifeste, aucun commit n'est créé.
- **Après la publication** : une Release publiée ne se corrige pas. Publier une nouvelle
  version (par exemple `vX.Y.(Z+1)`).
- Tout échec inattendu est consigné dans `LESSON-LEARNED.md`.

### Revue trimestrielle

Dependabot suit les actions GitHub et les modules Go, mais **pas** les versions d'outils
passées en paramètre des actions ou installées par `go install` : GoReleaser (`version`),
cosign (`cosign-release`), Syft (`syft-version`), Grype (`grype-version`), Krew et
`validate-krew-manifest`, hadolint, golangci-lint et govulncheck. Les vérifier chaque trimestre
(dernière version stable réelle), en même temps que la revue de `LESSON-LEARNED.md` et de la
constitution, ainsi que la date d'expiration du jeton Docker Hub.

## Commandes utiles

```bash
go test ./...                                   # tests unitaires et corpus
go test ./tests/manifests/...                   # contrôle manifest-lint
scripts/couverture 80                           # couverture de internal/ (gate test)
scripts/secrets                                 # gate secrets : gitleaks sur l'historique et le dossier (LL-002)
TELEMACH_KIND_IMAGE=kindest/node:v1.37.0@sha256:… \
  go test -tags integration ./tests/integration/...   # cluster kind jetable à trois nœuds
# Relevés du corpus (cluster jetable à trois nœuds, testdata/kind/three-nodes.yaml) ; -only f004
# ne relève que les cas de la feature 004, -variant 1.35 range une variante de version.
TELEMACH_KIND_IMAGE=… go run -tags integration ./scripts/collect-errors -out testdata/errors
TELEMACH_KIND_IMAGE=… go run -tags integration ./scripts/collect-objects -out testdata/objects
# Relevé d'une formation : rejoue ses pannes (tp*/pannes/*.yaml) et liste ce qui reste non
# reconnu ; prérequis facultatifs d'un TP dans <tp>/telemach-releve.yaml (apply, wait).
TELEMACH_KIND_IMAGE=… go run -tags integration ./scripts/collect-errors -training ../formation -report releve.md
```
