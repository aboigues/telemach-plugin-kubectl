# Page Docker Hub — `telemachlearning/telemach-kubectl`

Texte à coller par le mainteneur dans la page du dépôt Docker Hub (description courte, puis
description complète). Le garder à jour à chaque changement d'usage de l'image ; il ne cite
aucun numéro de version (les commandes retrouvent la dernière version stable), il n'y a donc
rien à recoller à chaque publication.

## Description courte (100 caractères au plus)

Explique en français les erreurs kubectl ; installe le plugin kubectl telemach ou essaie-le.

## Description complète

**TELEMACH** transforme chaque erreur `kubectl` en leçon, en français : ce qui s'est passé, le
concept Kubernetes en jeu, la cause probable et la commande corrigée. Cette image sert à deux
choses, sans rien installer d'autre que Docker.

### Installer le plugin sur ta machine

Les commandes retrouvent la dernière version stable et l'affichent ; pour une version
précise, remplace les deux lignes `VERSION=` par `VERSION=X.Y.Z`.

Linux et WSL2 :

```bash
VERSION=$(curl -fsSLo /dev/null -w '%{url_effective}' https://github.com/aboigues/telemach-plugin-kubectl/releases/latest)
VERSION=${VERSION##*/v}; echo "Version : $VERSION"
mkdir -p ~/.local/bin
docker run --rm --user "$(id -u):$(id -g)" -v ~/.local/bin:/sortie "telemachlearning/telemach-kubectl:${VERSION}" install
```

macOS (Docker Desktop) : ajoute `-e HOST_OS=darwin` avant `-v`. Le dossier `~/.local/bin`
doit faire partie de ton `PATH` : kubectl y trouve le plugin par son nom (`kubectl-telemach`).

### Vérifier des manifestes sans rien installer

Depuis le dossier de tes manifestes (aucun cluster n'est contacté, rien n'est écrit) :

```bash
VERSION=$(curl -fsSLo /dev/null -w '%{url_effective}' https://github.com/aboigues/telemach-plugin-kubectl/releases/latest)
VERSION=${VERSION##*/v}; echo "Version : $VERSION"
docker run --rm -it -v "$PWD":/manifestes:ro "telemachlearning/telemach-kubectl:${VERSION}" check
```

### Tags

`X.Y.Z` (jamais réécrit), `X.Y` et `latest` (versions stables). Préfère toujours une version
explicite : `latest` change sans prévenir.

### Vérifier l'image

Signature (cosign v3) :

```bash
cosign verify "telemachlearning/telemach-kubectl:${VERSION}" \
  --certificate-identity-regexp '^https://github\.com/aboigues/telemach-plugin-kubectl/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Provenance (SLSA) :

```bash
gh attestation verify "oci://docker.io/telemachlearning/telemach-kubectl:${VERSION}" --repo aboigues/telemach-plugin-kubectl
```

Image `scratch` (aucun shell, aucun gestionnaire de paquets), utilisateur non-root, pour
`linux/amd64` et `linux/arm64`.

Code source, autres voies d'installation (Release, Krew) : https://github.com/aboigues/telemach-plugin-kubectl
— licence Apache 2.0 — https://www.telemach-learning.fr
