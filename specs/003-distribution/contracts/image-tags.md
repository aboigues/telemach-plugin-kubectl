# Contrat — Tags de l'image `telemachlearning/telemach-kubectl`

| Tag | Mis à jour par | Mobile ? |
|---|---|---|
| `X.Y.Z` (ex. `0.1.0`) | chaque version, y compris pré-version (`0.2.0-rc.1`) | **non** : jamais réécrit (vérifié avant publication) |
| `X.Y` (ex. `0.1`) | versions stables uniquement | oui : suit le dernier correctif de la branche |
| `latest` | versions stables uniquement | oui |

Règles :

- Chaque tag désigne un index multi-architecture `linux/amd64` + `linux/arm64`.
- Ordre de publication : `X.Y.Z` est poussé, signé et attesté ; ensuite seulement, pour une
  version stable, `X.Y` et `latest` sont déplacés sur **le même digest**. Si la signature ou
  l'attestation échoue, `X.Y` et `latest` restent sur la version précédente, signée.
- La documentation, les notes de Release et les messages de l'image utilisent **toujours**
  `X.Y.Z` (principe III : tag explicite). `latest` existe pour la commodité, avec un rappel
  pédagogique dans le README.
- Les binaires embarqués ont exactement les sommes de `checksums.txt` de la Release `vX.Y.Z`.
- Signature vérifiable :

  ```bash
  cosign verify telemachlearning/telemach-kubectl:X.Y.Z \
    --certificate-identity-regexp '^https://github\.com/aboigues/telemach-plugin-kubectl/\.github/workflows/release\.yml@refs/tags/v' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com
  ```

- Provenance :
  `gh attestation verify oci://docker.io/telemachlearning/telemach-kubectl:X.Y.Z --repo aboigues/telemach-plugin-kubectl`
  (`docker.io/telemachlearning/telemach-kubectl` est exactement le `subject-name` utilisé par
  `actions/attest`). La même valeur est reprise dans le README, `docs/dockerhub.md` et le
  quickstart.
