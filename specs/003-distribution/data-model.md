# Modèle de données — 003 Distribution du plugin sans Go

Cette feature ne stocke rien chez l'apprenant : ses « données » sont les artefacts publiés et
les réglages de publication. Détails des formats : [contracts/](contracts/).

## Version

| Champ | Type | Règle |
|-------|------|-------|
| `tag` | texte | `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`, posé sur un commit de `main` |
| `version` | texte | `tag` sans le `v` ; injectée par `-X main.version` ; affichée par `kubectl telemach version` |
| `prerelease` | booléen | vrai si un suffixe `-…` existe ; ne met à jour ni `X.Y`, ni `latest`, ni l'index Krew |

## Release

| Champ | Règle |
|-------|-------|
| `binaires` | 4 : `kubectl-telemach_<version>_<os>_<arch>`, os ∈ {linux, darwin}, arch ∈ {amd64, arm64} |
| `archives` | 4 : `<binaire>.tar.gz`, contenant `kubectl-telemach` (même octets que le binaire), `LICENSE`, `NOTICE` |
| `sboms` | 4 : un SPDX JSON par binaire |
| `checksums.txt` | SHA-256 de chaque binaire, archive et SBOM |
| `signature` | `checksums.txt.sigstore.json` (cosign keyless, identité `release.yml@refs/tags/v…`) |
| `provenance` | attestation SLSA par ligne de `checksums.txt` |
| `état` | brouillon → publiée (immuable) ; une Release existante, même brouillon, bloque une nouvelle publication du même tag |

## Manifeste Krew (`plugins/telemach.yaml`)

| Champ | Règle |
|-------|-------|
| `metadata.name` | `telemach` |
| `spec.version` | `tag` de la Release (stable uniquement) |
| `spec.shortDescription`, `description`, `caveats` | français ; `caveats` donne la prochaine étape (`shell-init`) |
| `spec.platforms[]` | 4 entrées, `selector.matchLabels` {os, arch}, `uri` de l'archive de la Release publiée, `sha256` = ligne de `checksums.txt`, `bin: kubectl-telemach`, `files` : binaire, `LICENSE`, `NOTICE` |

Validation : `validate-krew-manifest` (v0.5.0) avant tout commit dans l'index.

## Index Krew

| Champ | Règle |
|-------|-------|
| `dépôt` | `aboigues/telemach-krew-index`, public, branche `main` |
| `contenu` | `plugins/telemach.yaml`, README |
| `écriture` | clé de déploiement (secret `KREW_INDEX_DEPLOY_KEY`, environnement « release ») ; jamais de `push --force` |

## Image `telemachlearning/telemach-kubectl`

| Champ | Règle |
|-------|-------|
| `tags` | `X.Y.Z` (immuable) ; `X.Y` et `latest` pour une version stable ([image-tags](contracts/image-tags.md)) |
| `plateformes` | `linux/amd64`, `linux/arm64` |
| `contenu` | `/usr/local/bin/telemach-image`, `/opt/telemach/linux/kubectl-telemach`, `/opt/telemach/darwin/kubectl-telemach` (sommes de `checksums.txt`) |
| `base`, `utilisateur` | `scratch`, `65532:65532` ; aucun shell, aucun kubeconfig |
| `preuves` | signature cosign par digest, SBOM, attestation SLSA |

## Installation par l'image (`install`)

| Entrée | Règle |
|--------|-------|
| `/sortie` | point de montage obligatoire, accessible en écriture, n'appartenant pas à root si le processus n'est pas root |
| `HOST_OS` | `linux` (défaut) ou `darwin` ; autre valeur refusée |
| fichier existant `/sortie/kubectl-telemach` | remplacé seulement s'il est un fichier ordinaire dont le module Go est `github.com/aboigues/telemach-plugin-kubectl` ; lien symbolique refusé |

Transitions : `absent` → `installé (version)` ; `installé (v1)` → `installé (v2)` (message avec
les deux versions) ; tout refus laisse l'état inchangé.

## Environnement « release » (GitHub)

| Élément | Règle |
|---------|-------|
| règle de déploiement | tags `v*` uniquement |
| secrets | `DOCKERHUB_TOKEN` (dépôt `telemach-kubectl`, lecture et écriture), `KREW_INDEX_DEPLOY_KEY` (clé privée de la clé de déploiement de l'index) |
| variable | `DOCKERHUB_USERNAME` |
| approbation | aucune (le push du tag vaut décision) |
