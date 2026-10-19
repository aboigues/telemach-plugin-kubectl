# Page Docker Hub — `telemachlearning/telemach-kubectl`

Texte à coller par le mainteneur dans la page du dépôt Docker Hub (description courte, puis
description complète). Le garder à jour à chaque changement d'usage de l'image.

## Description courte (100 caractères au plus)

Explique en français les erreurs kubectl ; installe le plugin kubectl telemach ou essaie-le.

## Description complète

**TELEMACH** transforme chaque erreur `kubectl` en leçon, en français : ce qui s'est passé, le
concept Kubernetes en jeu, la cause probable et la commande corrigée. Cette image sert à deux
choses, sans rien installer d'autre que Docker.

### Installer le plugin sur ta machine

Linux et WSL2 :

```bash
mkdir -p ~/.local/bin
docker run --rm --user "$(id -u):$(id -g)" -v ~/.local/bin:/sortie telemachlearning/telemach-kubectl:0.1.0 install
```

macOS (Docker Desktop) : ajoute `-e HOST_OS=darwin` avant `-v`. Le dossier `~/.local/bin`
doit faire partie de ton `PATH` : kubectl y trouve le plugin par son nom (`kubectl-telemach`).

### Vérifier des manifestes sans rien installer

Depuis le dossier de tes manifestes (aucun cluster n'est contacté, rien n'est écrit) :

```bash
docker run --rm -t -v "$PWD":/manifestes:ro telemachlearning/telemach-kubectl:0.1.0 check
```

### Tags

`X.Y.Z` (jamais réécrit), `X.Y` et `latest` (versions stables). Préfère toujours une version
explicite : `latest` change sans prévenir.

### Vérifier l'image

Signature (cosign v3) :

```bash
cosign verify telemachlearning/telemach-kubectl:0.1.0 \
  --certificate-identity-regexp '^https://github\.com/aboigues/telemach-plugin-kubectl/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Provenance (SLSA) :
`gh attestation verify oci://docker.io/telemachlearning/telemach-kubectl:0.1.0 --repo aboigues/telemach-plugin-kubectl`

Image `scratch` (aucun shell, aucun gestionnaire de paquets), utilisateur non-root, pour
`linux/amd64` et `linux/arm64`.

Code source, autres voies d'installation (Release, Krew) : https://github.com/aboigues/telemach-plugin-kubectl
— licence Apache 2.0 — https://www.telemach-learning.fr
