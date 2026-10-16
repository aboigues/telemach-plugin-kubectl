# TELEMACH PLUGIN KUBECTL Constitution

## Core Principles

### I. Vocation pédagogique

TELEMACH PLUGIN KUBECTL est un plugin pédagogique dont la finalité est de faciliter
l'apprentissage et la compréhension de Kubernetes au travers de `kubectl`. En conséquence :

- Chaque fonctionnalité MUST apporter une valeur d'apprentissage explicite : elle explique
  le *pourquoi* (concept Kubernetes sous-jacent), pas seulement le *comment*.
- Les messages du plugin (succès, avertissements, erreurs) MUST être compréhensibles par
  un débutant : ils nomment le concept Kubernetes concerné, décrivent la cause probable et
  proposent une action corrective concrète.
- Le vocabulaire Kubernetes (cluster, nœud, namespace, pod, conteneur, deployment,
  replicaset, service, ingress, configmap, secret, contexte, kubeconfig, etc.) MUST être
  employé de façon rigoureuse et cohérente ; tout terme introduit pour la première fois
  MUST être défini ou relié à une ressource de référence.
- Les exemples fournis MUST être exécutables tels quels et refléter les bonnes pratiques
  (notamment de sécurité, cf. principe III) : on n'enseigne jamais un anti-pattern sans
  le signaler explicitement comme tel.

**Justification** : un outil pédagogique qui enseigne de mauvaises pratiques ou produit des
messages opaques échoue à sa mission, même s'il fonctionne techniquement.

### II. Français pour l'humain, anglais pour le code

La langue est fixée selon le destinataire du contenu :

- MUST être rédigés en **français** : les spécifications (`spec.md`, `plan.md`,
  `tasks.md`, etc.), la documentation, le glossaire et tout le vocable pédagogique, les
  messages affichés par le plugin, les commentaires dans le code, le fichier
  `LESSON-LEARNED.md` et la présente constitution.
- MUST être rédigés en **anglais** : le code source (identifiants de variables, fonctions,
  classes, modules, fichiers de code), les noms de branches et les clés de configuration.
- Les termes techniques Kubernetes, les commandes et les champs de manifeste
  (`kubectl apply`, `Deployment`, `spec.containers`, `CrashLoopBackOff`, etc.) restent dans
  leur forme d'origine, y compris au sein d'un texte français.
- Le français MUST respecter l'orthographe complète, accents et diacritiques compris.

**Justification** : le public cible apprend en français ; le code en anglais reste
interopérable avec l'écosystème Kubernetes et les outils de développement.

### III. Sécurité élevée (NON NÉGOCIABLE)

Un niveau de sécurité élevé est exigé pour le plugin comme pour ce qu'il enseigne :

- Les secrets (jeton, mot de passe, clé, certificat client, contenu du kubeconfig, valeurs
  des objets `Secret`) MUST NOT être versionnés, journalisés ni affichés ; les secrets
  transitent uniquement par des mécanismes dédiés (variables d'environnement protégées,
  GitHub Secrets, objets `Secret` Kubernetes).
- Toute entrée externe (arguments, fichiers, manifestes, sorties de commandes `kubectl`)
  MUST être validée ; l'exécution de commandes shell MUST NOT reposer sur une
  concaténation de chaînes non échappées (pas d'injection de commande).
- Le plugin applique le principe du moindre privilège : il MUST NOT exiger de droits root
  sur la machine, ni de rôle `cluster-admin` ou de droits RBAC plus larges que ceux de la
  commande expliquée, sans justification documentée et avertissement explicite à
  l'utilisateur.
- Le plugin et ses tests MUST NOT créer, modifier ni supprimer de ressource dans un cluster
  de l'utilisateur sans action explicite de sa part ; les tests d'intégration MUST
  s'exécuter sur un cluster jetable dédié (ex. kind) ou dans un namespace créé pour
  l'occasion, jamais sur le contexte courant par défaut.
- Les dépendances, outils et images de base MUST être épinglés sur une version stable
  vérifiée au moment de l'ajout (recherche de la dernière version réelle, pas de
  supposition) ; les images de conteneur SHOULD être épinglées par digest.
- Les manifestes d'exemple produits MUST suivre les bonnes pratiques : `securityContext`
  non-root (`runAsNonRoot`, `allowPrivilegeEscalation: false`), pas de `privileged` ni de
  `hostPath`/`hostNetwork` sans avertissement, `resources` (requests/limits) déclarées,
  aucun secret en clair dans un manifeste ou une `ConfigMap`.
- Les exemples SHOULD référencer les images avec un tag explicite. Une image sans tag (donc
  `latest` implicite) ou avec le tag `latest` MUST déclencher un **avertissement non
  bloquant** dans les contrôles automatisés (tests, CI), et non un échec. Le contenu
  pédagogique SHOULD alors rappeler pourquoi un tag explicite est préférable.
  **Justification** : pour un débutant, `kubectl run web --image=nginx` est l'exemple le
  plus lisible ; le bloquer nuirait à la pédagogie, mais l'avertissement garde la bonne
  pratique visible.

**Justification** : un plugin qui pilote `kubectl` agit avec les identifiants de
l'utilisateur sur des clusters parfois partagés ou de production ; et un outil pédagogique
qui banalise l'insécurité propage de mauvaises habitudes.

### IV. Gates GitHub Actions obligatoires (NON NÉGOCIABLE)

Aucune modification n'atteint la branche principale sans passer des gates automatisées
GitHub Actions :

- La branche principale MUST être protégée : fusion uniquement via pull request, gates
  au statut « required », aucun push direct ni contournement administrateur.
- Les gates minimales MUST inclure : lint, tests automatisés, analyse statique de sécurité
  (SAST), détection de secrets, audit des dépendances (vulnérabilités connues) ; dès qu'un
  Dockerfile existe, lint Dockerfile et scan de l'image ; dès que des manifestes Kubernetes
  existent, validation de schéma et lint de sécurité des manifestes.
- Les workflows eux-mêmes MUST être durcis : actions tierces épinglées par SHA de commit
  complet, `permissions` minimales déclarées explicitement, pas de `pull_request_target`
  exécutant du code non fiable.
- Une gate en échec MUST bloquer la fusion ; la désactiver ou l'affaiblir constitue un
  amendement à cette constitution.

**Justification** : la sécurité exigée par le principe III n'est crédible que si elle est
vérifiée mécaniquement à chaque changement.

### V. Mémoire des incidents (LESSON-LEARNED.md)

Toute erreur commise une fois MUST NOT se reproduire :

- Tout incident (bug en production ou en CI, faille, régression, gate contournée, erreur
  de processus, mauvaise manipulation) MUST être consigné dans `LESSON-LEARNED.md` à la
  racine du dépôt.
- Chaque entrée MUST contenir : date (AAAA-MM-JJ), contexte, cause racine, impact,
  correctif appliqué et règle préventive à retenir.
- `LESSON-LEARNED.md` MUST être relu au démarrage de chaque session de travail et avant
  chaque étape Spec Kit (`/speckit-specify`, `/speckit-plan`, `/speckit-tasks`,
  `/speckit-implement`) ; les règles préventives pertinentes MUST être prises en compte
  dans les artefacts produits.
- Les leçons du projet frère TELEMACH PLUGIN DOCKER SHOULD être relues lors de la
  planification ; celles qui s'appliquent ici (intégration shell, options globales de la
  CLI, nettoyage des ressources de test, versions vérifiées, etc.) MUST être reportées
  dans le `LESSON-LEARNED.md` de ce dépôt.
- Quand une leçon est automatisable, elle SHOULD être transformée en gate (principe IV)
  ou en test de non-régression.

**Justification** : la relecture systématique transforme chaque incident en garde-fou
durable plutôt qu'en connaissance perdue, y compris lorsqu'il s'est produit dans le projet
frère.

## Exigences de sécurité

- **Chaîne d'approvisionnement** : chaque nouvelle dépendance est justifiée dans le plan
  de la feature (besoin, alternative envisagée, maintenance du projet, licence) ; les
  fichiers de verrouillage sont versionnés.
- **Mises à jour** : les mises à jour de sécurité des dépendances et des actions sont
  automatisées (ex. Dependabot ou équivalent) et traitées en priorité.
- **Vulnérabilités** : une vulnérabilité de sévérité critique ou haute détectée par les
  gates bloque la fusion jusqu'à correction ou dérogation documentée (justification,
  date d'expiration) dans la pull request.
- **Signalement** : le dépôt fournit un fichier `SECURITY.md` décrivant la procédure de
  signalement responsable des vulnérabilités.
- **Journalisation** : les journaux du plugin MUST NOT contenir de secrets, de contenu de
  kubeconfig, d'adresses de serveurs API privés ni de données personnelles.

## Workflow de développement et quality gates

1. **Démarrage** : relire `LESSON-LEARNED.md` (principe V) et la présente constitution.
2. **Spécification** : toute feature passe par Spec Kit (`/speckit-specify` →
   `/speckit-plan` → `/speckit-tasks` → `/speckit-implement`), artefacts rédigés en
   français.
3. **Constitution Check** : chaque `plan.md` vérifie explicitement la conformité aux
   cinq principes ; toute dérogation est justifiée dans la section « Complexity Tracking ».
4. **Revue** : chaque pull request est relue et vérifie, en plus du code, la qualité
   pédagogique des messages, le respect de la règle linguistique (principe II) et
   l'absence de régression de sécurité.
5. **Fusion** : uniquement lorsque toutes les gates GitHub Actions requises sont vertes
   (principe IV).
6. **Incident** : tout incident déclenche l'ajout d'une entrée dans `LESSON-LEARNED.md`
   dans la pull request de correction.

## Governance

- Cette constitution prévaut sur toute autre pratique ou convention du projet ; en cas de
  conflit, elle s'applique.
- **Amendement** : toute modification passe par une pull request dédiée, décrivant le
  changement, sa justification et son impact sur les artefacts existants (plan de
  migration si nécessaire), et soumise aux mêmes gates que le code.
- **Versionnement sémantique** :
  - MAJOR : suppression ou redéfinition incompatible d'un principe ou d'une règle de
    gouvernance ;
  - MINOR : ajout d'un principe ou d'une section, ou extension substantielle d'une règle ;
  - PATCH : clarification, reformulation, correction typographique sans effet normatif.
- **Conformité** : chaque revue de pull request et chaque `plan.md` vérifient la
  conformité à la constitution ; toute complexité ou dérogation MUST être justifiée par
  écrit.
- **Revue périodique** : la constitution et `LESSON-LEARNED.md` sont revus au moins une
  fois par trimestre pour intégrer les leçons récurrentes sous forme de règles.

**Version**: 1.0.0 | **Ratified**: 2026-10-16 | **Last Amended**: 2026-10-16
