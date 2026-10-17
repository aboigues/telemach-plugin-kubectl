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
  `integration`, `manifest-lint`.

**Prérequis du dépôt** : le Dependency graph et les alertes Dependabot doivent être activés
(Settings › Code security), sans quoi la gate `dependency-review` échoue (D-LL-005).

**Workflow de fusion** : Claude Code commit, pousse et ouvre la pull request ; le mainteneur
relit la PR puis la fusionne lui-même. Ce clic sur « Merge » vaut validation humaine : aucune
approbation formelle n'est exigée, GitHub interdisant d'approuver sa propre PR.

## Commandes utiles

```bash
go test ./...                                   # tests unitaires et corpus
go test ./tests/manifests/...                   # contrôle manifest-lint
scripts/couverture 80                           # couverture de internal/ (gate test)
scripts/secrets                                 # gate secrets : gitleaks sur l'historique et le dossier (LL-002)
TELEMACH_KIND_IMAGE=kindest/node:v1.37.0@sha256:… \
  go test -tags integration ./tests/integration/...   # cluster kind jetable
```
