# Contrat — Manifeste Krew et index du projet

## Index

- Dépôt git public `https://github.com/aboigues/telemach-krew-index.git`, branche par défaut
  `main`, fichier `plugins/telemach.yaml` (un seul plugin), README en français.
- Mis à jour **uniquement** par `.github/workflows/krew-index.yml` (clé de déploiement en
  écriture limitée à ce dépôt), un commit par version stable : `Index : telemach vX.Y.Z`.
- Jamais modifié par une pré-version ; jamais réécrit (pas de `push --force`).

## Installation documentée (forme canonique, à reprendre telle quelle)

```bash
kubectl krew index add telemach https://github.com/aboigues/telemach-krew-index.git
kubectl krew install telemach/telemach
kubectl telemach version
```

Mise à jour : `kubectl krew upgrade telemach`.

## Manifeste (`plugins/telemach.yaml`)

```yaml
apiVersion: krew.googlecontainertools.github.com/v1alpha2
kind: Plugin
metadata:
  name: telemach
spec:
  version: vX.Y.Z
  homepage: https://github.com/aboigues/telemach-plugin-kubectl
  shortDescription: Explique en français les erreurs kubectl et ce qui ne marche pas
  description: |
    <description en français : explication automatique des erreurs, check, diagnose,
    vérification différée ; licence Apache 2.0>
  caveats: |
    <prochaine étape en français : activer l'intégration shell avec
    eval "$(kubectl-telemach shell-init zsh)" (ou bash), puis relancer le terminal>
  platforms:
  - selector:
      matchLabels: {os: linux, arch: amd64}
    uri: https://github.com/aboigues/telemach-plugin-kubectl/releases/download/vX.Y.Z/kubectl-telemach_X.Y.Z_linux_amd64.tar.gz
    sha256: <somme de l'archive, lue dans checksums.txt>
    bin: kubectl-telemach
    files:
    - {from: kubectl-telemach, to: .}
    - {from: LICENSE, to: .}
    - {from: NOTICE, to: .}
  # … idem pour linux/arm64, darwin/amd64, darwin/arm64
```

## Garanties

1. Pour chaque plateforme, `uri` désigne l'archive de la Release `vX.Y.Z` publiée et `sha256`
   est **exactement** la somme de cette archive dans `checksums.txt` ; le générateur échoue si
   une des quatre archives manque.
2. Le manifeste passe `validate-krew-manifest` (v0.5.0) avant tout commit dans l'index, et le
   modèle passe la même validation sur un snapshot dans chaque PR qui touche `packaging/krew/`
   ou le générateur.
3. `kubectl telemach version` affiche `TELEMACH PLUGIN KUBECTL X.Y.Z` après
   `kubectl krew install telemach/telemach`.
4. Une archive altérée (somme différente) est refusée par Krew, qui n'installe rien.
