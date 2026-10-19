# Contrat — Artefacts d'une Release GitHub

URL de base : `https://github.com/aboigues/telemach-plugin-kubectl/releases/download/v<version>/`

## Fichiers

| Fichier | Description |
|---|---|
| `kubectl-telemach_<version>_linux_amd64` | binaire statique Linux x86-64 (voie manuelle) |
| `kubectl-telemach_<version>_linux_arm64` | binaire statique Linux ARM64 |
| `kubectl-telemach_<version>_darwin_amd64` | binaire macOS Intel |
| `kubectl-telemach_<version>_darwin_arm64` | binaire macOS Apple Silicon |
| `kubectl-telemach_<version>_<os>_<arch>.tar.gz` | archive (voie Krew) : `kubectl-telemach`, `LICENSE`, `NOTICE` à la racine ; le binaire est octet pour octet celui de la ligne ci-dessus |
| `kubectl-telemach_<version>_<os>_<arch>.sbom.json` | SBOM SPDX JSON de chaque binaire |
| `checksums.txt` | une ligne `<sha256>  <fichier>` par binaire, archive et SBOM (format `sha256sum`) |
| `checksums.txt.sigstore.json` | bundle cosign de `checksums.txt` |

Ces noms sont stables : le README, les notes de Release, le manifeste Krew et les scripts des
utilisateurs en dépendent. Les changer est un changement incompatible (version MAJOR).

## Garanties

1. `./kubectl-telemach_<version>_<os>_<arch> version` affiche
   `TELEMACH PLUGIN KUBECTL <version>` en première ligne ; installé sous le nom
   `kubectl-telemach` dans un dossier du `PATH`, `kubectl telemach version` affiche la même
   ligne et `kubectl plugin list` cite le plugin.
2. `sha256sum --check --ignore-missing checksums.txt` (Linux) ou
   `shasum -a 256 --check --ignore-missing checksums.txt` (macOS) réussit pour tout fichier
   téléchargé.
3. Signature :

   ```bash
   cosign verify-blob \
     --bundle checksums.txt.sigstore.json \
     --certificate-identity-regexp '^https://github\.com/aboigues/telemach-plugin-kubectl/\.github/workflows/release\.yml@refs/tags/v' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   ```

4. Provenance : `gh attestation verify <fichier> --repo aboigues/telemach-plugin-kubectl`
   réussit pour chaque binaire et chaque archive, et désigne `release.yml` et le commit du tag.
5. Reconstruire le binaire depuis le tag (procédure du [quickstart](../quickstart.md)) donne la
   même somme SHA-256.
6. La Release est immuable : aucun fichier n'est ajouté, modifié ou supprimé après publication.

## Notes de Release (structure)

```text
## Installer TELEMACH <version>
<Linux / macOS : téléchargement, vérification de somme, installation dans ~/.local/bin, version>
<Krew : kubectl krew index add … puis kubectl krew install telemach/telemach>
<Image : telemachlearning/telemach-kubectl:<version> install>
<Prochaine étape : kubectl-telemach shell-init>

## Nouveautés
<commits feat>
## Corrections
<commits fix>
## Autres changements
<autres commits, hors docs/ci/test>

## Vérifier l'authenticité
<commandes cosign et gh attestation>
```
