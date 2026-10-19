# Quickstart — validation de la distribution (003)

Guide de validation de bout en bout. Chaque commande est rejouée telle qu'écrite (LL-004,
D-LL-006). Les essais de publication se font sur un tag de pré-version ou un index de
répétition, jamais sur une version déjà publiée ni sur l'index réel (D-LL-015).

## Prérequis

- Avant la première version : réglages de [CONTRIBUTING](../../CONTRIBUTING.md) faits par le
  mainteneur (releases immuables, environnement « release », secrets `DOCKERHUB_TOKEN` et
  `KREW_INDEX_DEPLOY_KEY`, variable `DOCKERHUB_USERNAME`, dépôts Docker Hub et index Krew).
- Machines : Linux amd64 (ou WSL2), Linux arm64 si disponible, macOS arm64 ; kubectl installé ;
  Docker pour D1 à D3 ; Krew pour K1 et K2 ; Go **absent** pour R1 à R4.

## Snapshot local (avant toute publication)

### S1 — Configuration et artefacts

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=sign,docker
DIST_DIR=dist go test -tags dist -count=1 ./tests/release/...
```

Attendu : 4 binaires, 4 archives, 4 SBOM, `checksums.txt` ; noms, sommes, module, version et
reproductibilité conformes à [release-assets](contracts/release-assets.md).

### S2 — Manifeste Krew d'essai

```bash
go run ./scripts/krew-manifest -checksums dist/checksums.txt -version 0.0.0-snapshot -out /tmp/telemach.yaml
validate-krew-manifest -manifest /tmp/telemach.yaml
```

Attendu : manifeste valide, quatre plateformes, sommes identiques à `dist/checksums.txt`.

### S3 — Image snapshot

```bash
goreleaser release --snapshot --clean --skip=sign,sbom
docker image ls telemachlearning/telemach-kubectl
```

Attendu : image amd64 et arm64 de moins de 15 Mo compressés (SC-007), utilisateur `65532`,
binaires linux et darwin aux sommes de `dist/checksums.txt`.

## Release (après publication de `vX.Y.Z`)

### R1 — Installation Linux et WSL2 (SC-001)

```bash
VERSION=X.Y.Z
ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; esac
FICHIER=kubectl-telemach_${VERSION}_linux_${ARCH}
curl -fsSLO "https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v${VERSION}/${FICHIER}"
curl -fsSLO "https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v${VERSION}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
mkdir -p ~/.local/bin
install -m 0755 "$FICHIER" ~/.local/bin/kubectl-telemach
kubectl telemach version
kubectl plugin list
```

Attendu : vérification `OK`, première ligne `TELEMACH PLUGIN KUBECTL X.Y.Z`, plugin listé, en
moins de 5 minutes.

### R2 — Installation macOS

Même procédure avec `_darwin_` et `shasum -a 256 --check --ignore-missing checksums.txt`.

### R3 — Signature et provenance (SC-004)

Commandes `cosign verify-blob` et `gh attestation verify` de
[release-assets](contracts/release-assets.md) : toutes réussissent.

### R4 — Fichier altéré

```bash
printf 'x' >> "$FICHIER" && sha256sum --check --ignore-missing checksums.txt
```

Attendu : `FAILED` ; le README dit de ne pas installer ce fichier.

## Krew

### K1 — Installation (SC-002)

```bash
kubectl krew index add telemach https://github.com/aboigues/telemach-krew-index.git
kubectl krew install telemach/telemach
kubectl telemach version
```

Attendu : version `X.Y.Z`, message de Krew en français avec la prochaine étape.

### K2 — Mise à jour et pré-version

Après publication d'une version stable plus récente : `kubectl krew upgrade telemach` installe
la nouvelle version. Après une pré-version : l'index n'a pas changé (`git log` du dépôt de
l'index).

## Image

### D1 — Installation Linux et WSL2 (SC-003)

```bash
mkdir -p ~/.local/bin
docker run --rm --user "$(id -u):$(id -g)" \
  -v ~/.local/bin:/sortie \
  telemachlearning/telemach-kubectl:X.Y.Z install
kubectl telemach version
ls -l ~/.local/bin/kubectl-telemach
```

Attendu : message en français (version, système, rappel du `PATH`, `shell-init`), fichier
`0755` appartenant à l'utilisateur, en moins de 1 minute une fois l'image téléchargée.

### D2 — Refus pédagogiques

Sans `-v` ; sans `--user` ; avec `-e HOST_OS=windows` ; avec un lien symbolique
`~/.local/bin/kubectl-telemach` : chaque cas affiche concept et correction, code 1 ou 2, rien
n'est écrit ([image-cli](contracts/image-cli.md)).

### D3 — Essai sans installation

```bash
cd testdata/manifests/<cas fautif>
docker run --rm -it -v "$PWD":/manifestes:ro telemachlearning/telemach-kubectl:X.Y.Z check
```

Attendu : même résultat que `kubectl telemach check -f . -R` installé, première ligne sur les
règles appliquées ; dossier vide → explication du montage, code 2.

### D4 — Signature et provenance de l'image

Commandes `cosign verify` et `gh attestation verify oci://…` de
[image-tags](contracts/image-tags.md) : toutes réussissent.

## Publication (mainteneur)

### P1 — Pré-version de répétition

Tag `vX.Y.Z-rc.1` sur `main` : Release marquée pré-version, image `X.Y.Z-rc.1` seule (ni
`X.Y` ni `latest`), index Krew inchangé ; relance sur le même tag → échec de `verify`, rien de
modifié.

### P2 — Tag refusé

Tag `v0.0.0-test.1` sur un commit hors de `main` : `verify` échoue (« Le tag doit pointer sur
un commit de main »), `gates` et `release` ne s'exécutent pas ; tag supprimé ensuite.

### P3 — Relance de l'index seul

`gh workflow run krew-index.yml --ref vX.Y.Z` sur une version déjà publiée : le manifeste est
regénéré, identique, et aucun commit n'est créé s'il n'a pas changé.

### P4 — Délai (SC-005)

Du push du tag à la disponibilité des trois voies : moins de 30 minutes.

### Reproductibilité (FR-005)

```bash
git checkout vX.Y.Z
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -buildid= -X main.version=X.Y.Z" -o /tmp/kubectl-telemach ./cmd/kubectl-telemach
sha256sum /tmp/kubectl-telemach
```

Attendu : même somme que `kubectl-telemach_X.Y.Z_linux_amd64` dans `checksums.txt`.
