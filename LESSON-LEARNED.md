# LESSON-LEARNED — TELEMACH PLUGIN KUBECTL

Registre des incidents du projet et des leçons qui en découlent (principe V de la
[constitution](.specify/memory/constitution.md)).

## Règles d'usage

- **Relecture obligatoire** : ce fichier MUST être relu au démarrage de chaque session de
  travail et avant chaque étape Spec Kit (`/speckit-specify`, `/speckit-plan`,
  `/speckit-tasks`, `/speckit-implement`).
- **Quoi consigner** : tout incident — bug en production ou en CI, faille de sécurité,
  régression, gate contournée, erreur de processus, mauvaise manipulation.
- **Quand** : l'entrée est ajoutée dans la pull request qui corrige l'incident.
- **Ordre** : les entrées les plus récentes en haut de la section « Incidents ».
- **Automatisation** : quand une leçon peut être vérifiée mécaniquement, elle SHOULD devenir
  une gate GitHub Actions ou un test de non-régression ; le noter dans le champ « Suivi ».
- **Pas de secret** : aucune entrée ne contient de jeton, mot de passe, contenu de kubeconfig
  ou donnée personnelle.

## Modèle d'entrée

```markdown
### LL-XXX — Titre court de l'incident

- **Date** : AAAA-MM-JJ
- **Catégorie** : sécurité | CI/gates | bug | régression | processus | pédagogie | langue
- **Contexte** : ce qui était en cours au moment de l'incident.
- **Cause racine** : pourquoi c'est arrivé (pas seulement le symptôme).
- **Impact** : conséquences observées ou évitées de justesse.
- **Correctif** : ce qui a été fait (lien vers la PR / le commit).
- **Règle préventive** : la règle à appliquer désormais, formulée de façon actionnable.
- **Suivi** : gate ou test ajouté (ou « non automatisable » avec justification).
```

## Leçons héritées du projet Docker

Leçons du projet frère [TELEMACH PLUGIN DOCKER](../telemach-plugin-docker/LESSON-LEARNED.md)
qui s'appliquent ici (research.md R14 de la feature 001). Le détail de chaque incident est
dans le registre d'origine ; seule la règle préventive, transposée à kubectl, est reprise.

- **D-LL-001 — Version de Go des outils d'analyse** : à chaque montée de version de Go,
  vérifier que golangci-lint, govulncheck… sont compilés avec une version de Go au moins
  égale ; sinon les compiler avec la chaîne d'outils du projet (`install-mode: goinstall`).
- **D-LL-002 — zsh duplique stdout avec MULTIOS** : tout code shell partagé entre bash et zsh
  est testé sous les deux shells, avec des assertions sur stdout **et** stderr
  (`setopt NO_MULTIOS` dans la fonction zsh).
- **D-LL-003 — Fichier temporaire et Ctrl-C** : installer le `trap` de nettoyage **avant** de
  créer la ressource à nettoyer, et tester l'interruption à plusieurs moments.
- **D-LL-005 — Prérequis d'une gate** : à la création d'une gate CI, vérifier ses prérequis côté
  dépôt (Dependency graph pour `dependency-review`), les documenter dans `CONTRIBUTING.md` et
  ouvrir une PR de test dès que les gates sont en place.
- **D-LL-004 — Tags d'images d'exemple** : consulter le registre avant d'écrire un tag
  d'image ; pour nginx, la branche stable a un numéro mineur pair.
- **D-LL-006 et D-LL-013 — Intégration shell inactive ou périmée** : les tests de bout en bout
  reproduisent exactement la procédure d'installation documentée ; l'état chargé dans le shell
  porte sa version (`TELEMACH_KUBECTL_SHELL`) et `last`/`deepen` signalent un écart.
- **D-LL-007 — Tests à étiquette de build** : toute étiquette de build est ajoutée à
  `run.build-tags` de `.golangci.yml` dans la même PR.
- **D-LL-008 — Commit malgré un lint en échec** : enchaîner validation et commit uniquement par
  `&&` ; un commit n'est créé que si chaque validation a réussi.
- **D-LL-009 — Vérification qui n'a rien comparé** : un script de vérification ad hoc s'exécute
  dans `bash -c` et échoue si une valeur comparée est vide.
- **D-LL-012 — Objectif chiffré sans test** : tout objectif chiffré (délai, taille) reçoit un
  test qui le mesure dans la tâche même où il est annoncé.
- **D-LL-014 — Forme d'appel supposée** : chaque point d'entrée appelé par un autre programme
  est testé avec la forme exacte que ce programme lui transmet, observée sur le vrai programme
  (ici : kubectl refuse les options placées avant le nom du plugin).
- **D-LL-015 — Objets de l'utilisateur touchés par un essai** : un essai ne vise que des objets
  qu'il a créés lui-même ; ici, les tests ne s'exécutent que sur un cluster kind jetable, jamais
  sur le contexte kubectl courant de la machine.

## Incidents

Aucun incident consigné pour l'instant.
