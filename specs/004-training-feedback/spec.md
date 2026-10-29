# Feature Specification: Couvrir les TP d'une vraie formation

**Feature Branch**: `004-training-feedback`

**Created**: 2026-10-29

**Status**: Draft

**Input**: User description: "Retours de formation : couvrir les commandes et pannes des 14 TP
de la formation kubernetes-formation encore mal prises en charge par la version 0.1.0 (issues
GitHub #13, #14, #15, #16, #17, #19)." (description complète : `check -f -` ; refus
d'admission (ResourceQuota, LimitRange, PodSecurity) et `ReplicaFailure` d'un Deployment ;
maintenance des nœuds (`drain`, taints, cordon, anti-affinité) ; métriques et `diagnose hpa/` ;
`diagnose` des Jobs, CronJobs, DaemonSets et StatefulSets et syntaxe cron ; `wait` expiré,
`rollout undo` sans révision, `debug` indisponible ; relevé des messages non reconnus d'une
formation ; lecture seule du cluster via la liste fermée, aucun droit au-delà de get/list.)

## Contexte

La version 0.1.0 a été confrontée aux 14 TP du parcours de formation Kubernetes du projet
(kubernetes-formation). Quatre défauts ont été corrigés à part (issues #10, #11, #12, #18 ;
LL-014 à LL-017). Restent six familles de situations, rencontrées dans les TP, que le plugin
ne couvre pas : il répond « Erreur non reconnue », « n'est pas pris en charge par diagnose »,
ou se tait alors qu'un piège silencieux est en place.

Cette feature reprend sans les modifier les garanties des features 001 à 003 : français, aucune
cause inventée, aucune action à la place de l'apprenant, secrets masqués, lectures du cluster
limitées à une liste fermée de lectures (get/list), tests sur un cluster kind jetable,
Kubernetes 1.35 à 1.37, distribution inchangée (binaire, Krew, image).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Comprendre un objet refusé à l'admission (Priority: P1)

L'apprenant travaille dans un namespace doté d'un ResourceQuota, d'un LimitRange ou d'un niveau
PodSecurity (TP08, durcissement « restricted » du parcours). Sa commande est refusée
(`exceeded quota`, `must specify limits.cpu`, `maximum cpu usage per Container`,
`violates PodSecurity "restricted:latest"`), ou, plus piégeux, elle réussit (`deployment.apps/api
scaled`) alors que le ReplicaSet se voit refuser ses pods. TELEMACH reconnaît le refus, nomme la
règle du namespace en cause avec les valeurs extraites (ressource, demandé, utilisé, limite ;
niveau PodSecurity et champs à corriger), propose les commandes pour lire cette règle et, pour
PodSecurity, montre le `securityContext` à ajouter.

**Why this priority**: le TP08 provoque ces refus volontairement, le niveau `restricted` est
enseigné dans tout le parcours, et le cas du Deployment « scaled » sans pods est le plus
déroutant : rien n'échoue côté kubectl.

**Independent Test**: sur un cluster kind jetable, créer un namespace avec quota, LimitRange et
label `pod-security.kubernetes.io/enforce=restricted`, puis rejouer chaque refus et un
`scale` au-delà du quota ; vérifier l'entrée reconnue, les valeurs extraites et le diagnostic du
Deployment.

**Acceptance Scenarios**:

1. **Given** un quota `limits.cpu=1` déjà atteint, **When** l'apprenant crée un pod qui demande
   `limits.cpu=2`, **Then** l'explication nomme le quota, la ressource, le demandé, l'utilisé et
   la limite, propose `kubectl describe resourcequota -n <ns>` et présente « gonfler le quota »
   comme un anti-pattern.
2. **Given** un quota sur `limits.cpu`, **When** un pod sans `resources.limits` est créé,
   **Then** l'explication dit que le quota impose de déclarer la limite pour chaque conteneur
   cité et montre où l'ajouter.
3. **Given** un namespace `restricted`, **When** un pod non conforme est créé, **Then**
   l'explication cite le niveau, la liste des champs à corriger, et montre le `securityContext`
   corrigé ; retirer le label `enforce` est présenté comme un anti-pattern.
4. **Given** un namespace en mode `warn`, **When** un `apply` réussit avec
   `Warning: would violate PodSecurity …`, **Then** une information non bloquante explique
   l'avertissement après la commande.
5. **Given** un Deployment dont le ReplicaSet est refusé par le quota, **When** l'apprenant lance
   `kubectl telemach diagnose deployment/api`, **Then** le diagnostic lit la condition
   `ReplicaFailure` et l'événement `FailedCreate` du ReplicaSet courant et explique le refus avec
   la même leçon que pour la commande refusée.
6. **Given** la même situation, **When** l'apprenant lance `kubectl scale deployment api
   --replicas=3` avec l'intégration shell active, **Then** la vérification différée signale que
   des pods sont refusés.

---

### User Story 2 - Comprendre un pod qui n'est placé sur aucun nœud (Priority: P2)

Au TP09 (placement et maintenance), l'apprenant pose des taints, des tolérances, des affinités,
met un nœud en maintenance (`cordon`, `drain`). Un pod reste `Pending`, ou `drain` s'arrête sur
un DaemonSet, un PodDisruptionBudget ou un volume local. TELEMACH explique le message de
`drain` et, pour un pod `Pending`, détaille la répartition des nœuds par cause donnée par le
scheduler (taint non toléré, anti-affinité, nœud non planifiable, ressources), avec le taint
concerné et la correction adaptée.

**Why this priority**: c'est un TP entier où chaque erreur reste aujourd'hui « non reconnue » ou
réduite à `Unschedulable` ; la confusion « une toleration autorise, elle ne force pas » est le
piège n° 1 du TP.

**Independent Test**: sur un cluster kind à plusieurs nœuds jetable, poser un taint, créer un
pod sans tolérance, un pod à anti-affinité impossible, mettre un nœud en `cordon`, lancer un
`drain` bloqué par un DaemonSet puis par un PDB ; vérifier chaque explication et la répartition
des nœuds.

**Acceptance Scenarios**:

1. **Given** un nœud portant le taint `dedie=gpu:NoSchedule`, **When** l'apprenant diagnostique
   un pod `Pending` sans tolérance, **Then** le diagnostic nomme le taint, le nombre de nœuds
   concernés, propose de lire les taints des nœuds et rappelle qu'une tolérance autorise sans
   forcer le placement (il faut aussi un nodeSelector ou une affinité).
2. **Given** un message du scheduler qui agrège plusieurs causes (« 1 node(s) had untolerated
   taint …, 2 node(s) didn't match pod anti-affinity rules »), **When** le pod est diagnostiqué,
   **Then** chaque cause est listée avec son nombre de nœuds, au lieu de la seule première.
3. **Given** un nœud en `cordon`, **When** un pod ne trouve pas de place, **Then** la cause
   « nœud non planifiable » est expliquée avec `kubectl uncordon` comme correction.
4. **Given** un `drain` arrêté par des pods de DaemonSet, **When** l'erreur s'affiche, **Then**
   l'explication propose `--ignore-daemonsets` et explique pourquoi ces pods restent.
5. **Given** un `drain` qui ne peut pas évincer un pod protégé par un PodDisruptionBudget,
   **When** l'erreur s'affiche, **Then** l'explication présente le PDB, propose
   `kubectl get pdb -A`, et désigne `--disable-eviction` et `--force` comme anti-patterns.
6. **Given** un `drain` arrêté par un pod à stockage local (`emptyDir`), **When** l'erreur
   s'affiche, **Then** l'explication propose `--delete-emptydir-data` en avertissant de la perte
   des données.

---

### User Story 3 - Diagnostiquer un Job, un CronJob, un DaemonSet ou un StatefulSet (Priority: P3)

Aux TP04 et TP05, l'apprenant manipule des Jobs, des CronJobs, des DaemonSets et un
StatefulSet. Il lance `kubectl telemach diagnose job/calc` (ou `cronjob/`, `daemonset/`,
`statefulset/`). TELEMACH lit l'état de l'objet, explique sa condition (échecs répétés, délai
dépassé, planification suspendue, exécution sautée, DaemonSet incomplet, pod ordonné en attente,
volume ou Service manquant), puis enchaîne sur le pod en difficulté le plus récent avec les
leçons existantes. Une erreur de syntaxe cron est expliquée à l'application, et détectée par
`check` hors ligne.

**Why this priority**: deux TP entiers ; les pannes propres à ces objets ne se voient pas sur un
pod isolé, et `diagnose` les refuse aujourd'hui.

**Independent Test**: sur kind, créer un Job qui échoue (backoffLimit atteint), un Job qui
dépasse `activeDeadlineSeconds`, un CronJob suspendu, un DaemonSet absent du control-plane, un
StatefulSet dont le pod `-0` n'est pas prêt et un autre sans Service headless ; vérifier chaque
diagnostic et le cas sain.

**Acceptance Scenarios**:

1. **Given** un Job en échec `BackoffLimitExceeded`, **When** il est diagnostiqué, **Then** le
   diagnostic explique la limite de tentatives, puis la cause du dernier pod en échec (journal,
   code de sortie) avec la leçon existante.
2. **Given** un Job en échec `DeadlineExceeded`, **When** il est diagnostiqué, **Then** le
   diagnostic explique `activeDeadlineSeconds`.
3. **Given** un Job terminé dont les pods ont été supprimés par `ttlSecondsAfterFinished`,
   **When** il est diagnostiqué, **Then** le diagnostic explique pourquoi aucun journal n'est
   lisible.
4. **Given** un CronJob `suspend: true`, ou qui n'a jamais été déclenché, ou dont des exécutions
   sont sautées par `concurrencyPolicy: Forbid`, **When** il est diagnostiqué, **Then** la cause
   est nommée avec le champ en jeu (y compris `timeZone` et `startingDeadlineSeconds`).
5. **Given** un DaemonSet dont le nombre de pods voulus est inférieur au nombre de nœuds,
   **When** il est diagnostiqué, **Then** les nœuds exclus et leur taint (control-plane) sont
   expliqués comme un comportement normal ou un oubli de tolérance.
6. **Given** un StatefulSet dont le pod `-1` n'existe pas parce que `-0` n'est pas prêt, ou dont
   un PVC de `volumeClaimTemplates` est `Pending`, ou dont le Service de `serviceName` est
   absent, **When** il est diagnostiqué, **Then** la cause est nommée (`OrderedReady`, PVC,
   Service headless).
7. **Given** un CronJob dont `schedule` n'a que 4 champs, **When** l'apprenant l'applique ou le
   vérifie avec `check`, **Then** l'explication rappelle les 5 champs avec un exemple commenté.
8. **Given** un namespace contenant un Job en échec, **When** l'apprenant lance `diagnose` sans
   argument, **Then** le Job figure dans la liste des objets en difficulté.

---

### User Story 4 - Comprendre l'autoscaling et les métriques (Priority: P4)

Aux TP06 et TP13, l'apprenant utilise `kubectl top` et un HorizontalPodAutoscaler. `top` échoue
(`Metrics API not available`), ou `kubectl get hpa` affiche `<unknown>/50%` sans erreur.
TELEMACH explique l'absence de metrics-server et, avec `diagnose hpa/<nom>`, suit la chaîne :
API de métriques disponible, cible existante, request déclarée pour la ressource visée par
chaque conteneur, conditions de l'autoscaler (y compris le plafond `maxReplicas`).

**Why this priority**: le piège `<unknown>` est le plus courant du TP06 et rien n'échoue : seul
un diagnostic peut l'expliquer.

**Independent Test**: sur kind sans metrics-server, lancer `kubectl top pods` ; créer un HPA dont
la cible n'a pas de request CPU ; puis installer metrics-server et un HPA sain plafonné ;
vérifier chaque explication.

**Acceptance Scenarios**:

1. **Given** un cluster sans metrics-server, **When** `kubectl top pods` échoue, **Then**
   l'explication présente metrics-server, propose de vérifier son APIService et son
   installation, et précise qu'il faut environ une minute après l'installation.
2. **Given** un HPA dont la cible n'a pas de request CPU, **When** il est diagnostiqué, **Then**
   le conteneur sans request est nommé et l'explication dit que le pourcentage est calculé par
   rapport à la request.
3. **Given** un HPA sur une cible inexistante, ou avec metrics-server indisponible, **When** il
   est diagnostiqué, **Then** le premier maillon cassé est nommé.
4. **Given** un HPA sain qui atteint `maxReplicas`, **When** il est diagnostiqué, **Then** la
   condition `ScalingLimited` est expliquée comme un plafond et non comme une panne.
5. **Given** l'intégration shell active, **When** l'apprenant applique un HPA dont la cible n'a
   pas de request CPU, **Then** la vérification différée le signale.
6. **Given** des fichiers locaux contenant un HPA et le Deployment qu'il vise sans request CPU,
   **When** l'apprenant lance `check`, **Then** une information signale l'absence de request.

---

### User Story 5 - Vérifier ce que produisent Helm, Kustomize et les générateurs (Priority: P5)

L'apprenant veut vérifier, avant de l'appliquer, ce que produit `helm template`,
`kubectl kustomize` ou `kubectl create … --dry-run=client -o yaml`. Il lance
`… | kubectl telemach check -f -`. TELEMACH lit l'entrée standard comme un fichier, désigne les
positions par `<stdin>:ligne:colonne` et, pour un rendu Helm, rattache chaque problème au
template d'origine indiqué par les commentaires `# Source:`.

**Why this priority**: c'est la seule façon de vérifier un rendu avant de l'appliquer ;
`kubectl apply -f -` étant standard, l'apprenant s'attend à ce que `check -f -` marche.

**Independent Test**: rendre un chart de test avec `helm template` et le passer à `check -f -` ;
vérifier les positions et le template d'origine ; passer un flux trop gros ou vide.

**Acceptance Scenarios**:

1. **Given** un manifeste valide sur l'entrée standard, **When** l'apprenant lance
   `check -f -`, **Then** le résultat est identique à celui du même fichier, désigné `<stdin>`.
2. **Given** un rendu Helm dont un document est fautif, **When** il est vérifié, **Then** le
   problème cite `<stdin>:ligne:colonne` et le template d'origine
   (`chart/templates/api.yaml`).
3. **Given** un flux au-delà de la taille maximale, ou vide, **When** il est vérifié, **Then**
   un message explique la limite ou l'absence de manifeste, avec le code de sortie d'un fichier
   illisible.
4. **Given** le README, **When** l'apprenant cherche à vérifier un chart, **Then** la recette
   `helm template … | kubectl telemach check -f -` y figure, et les informations de `check` sur
   les templates Helm la proposent.

---

### User Story 6 - Comprendre les dernières erreurs courantes des TP (Priority: P6)

L'apprenant utilise `kubectl wait`, `kubectl rollout undo --to-revision` et `kubectl debug`.
Ces commandes échouent avec des messages aujourd'hui non reconnus. TELEMACH explique un `wait`
expiré et enchaîne sur le diagnostic de l'objet attendu, explique une révision absente de
l'historique, et un conteneur éphémère indisponible. Le mainteneur dispose d'un relevé qui
rejoue les scénarios de pannes d'une formation et liste les messages encore non reconnus.

**Why this priority**: entrées de catalogue isolées, utiles mais moins structurantes ; le relevé
outille les prochaines confrontations à une formation.

**Independent Test**: sur kind, rejouer un `wait` qui expire (condition, jsonpath, delete), un
`rollout undo --to-revision=99`, et un `debug` sur un cluster sans conteneurs éphémères si
disponible (sinon message relevé) ; lancer le relevé sur les scénarios de pannes.

**Acceptance Scenarios**:

1. **Given** `kubectl wait --for=condition=Ready pod/web --timeout=30s` qui expire, **When**
   l'erreur s'affiche, **Then** l'explication dit que l'objet n'est pas dans l'état attendu et
   propose `kubectl telemach diagnose pod/web` avec la portée de la commande (même chose pour
   `deployment/…`, `--for=jsonpath=…` et `--for=delete`).
2. **Given** `rollout undo --to-revision=5` sur une révision absente, **When** l'erreur
   s'affiche, **Then** l'explication présente l'historique des ReplicaSets et
   `revisionHistoryLimit`, et propose `kubectl rollout history`.
3. **Given** un cluster où les conteneurs éphémères sont indisponibles, **When** `kubectl debug`
   échoue, **Then** l'explication propose les alternatives (pod d'enquête, `debug node/`).
4. **Given** l'explication d'un exécutable introuvable dans un conteneur, **When** l'apprenant
   approfondit, **Then** elle rappelle `kubectl debug --target=<conteneur>` pour voir les
   processus de l'application.
5. **Given** un dossier de scénarios de pannes, **When** le mainteneur lance le relevé, **Then**
   il obtient la liste des messages non reconnus avec la commande qui les produit.

---

### Edge Cases

- Un message de quota ou de PodSecurity d'une version de Kubernetes différente (1.35, 1.36) :
  chaque motif est validé sur les relevés des trois versions (LL-008).
- Une lecture refusée par RBAC (nœuds, APIService, ReplicaSets, Jobs) : le diagnostic devient
  partiel et nomme la lecture manquante, sans inventer la suite.
- Un pod `Pending` dont le message du scheduler mêle une cause connue et une cause inconnue :
  les causes connues sont expliquées, l'inconnue est citée telle quelle.
- Un HPA sur une métrique autre que CPU/mémoire (métrique personnalisée) : hors de portée,
  présenté comme tel.
- Un CronJob jamais déclenché parce que sa première échéance n'est pas encore arrivée : présenté
  comme normal, avec l'heure de la prochaine exécution si elle est calculable.
- `check -f -` combiné à d'autres `-f` : l'entrée standard est lue une seule fois ; un second
  `-f -` est refusé comme avec kubectl.
- Un rendu Helm sans commentaire `# Source:` (Kustomize, générateur) : seules les positions
  `<stdin>` sont données.
- L'entrée standard est un terminal (rien n'est redirigé) : un message explique comment
  utiliser `-f -`, sans attendre indéfiniment.
- Un avertissement PodSecurity répété à chaque `apply` : affiché au plus une fois par
  namespace et par 24 h.
- Un `wait` sur plusieurs objets (`--all`, sélecteur) : la proposition de diagnostic vise le
  premier objet cité par le message.

## Requirements *(mandatory)*

### Functional Requirements

#### Refus d'admission (US1)

- **FR-001**: Le catalogue MUST reconnaître le dépassement de quota, l'exigence de déclarer une
  ressource imposée par un quota, le dépassement d'un LimitRange et le refus PodSecurity, en
  extrayant ressource, valeurs demandée/utilisée/limite, nom du quota, niveau PodSecurity et
  champs à corriger.
- **FR-002**: Chaque refus d'admission MUST proposer la lecture de la règle du namespace en
  cause (`describe resourcequota`, `describe limitrange`, labels du namespace) ; gonfler le
  quota et retirer le label `pod-security.kubernetes.io/enforce` MUST être présentés comme
  anti-patterns.
- **FR-003**: Pour un refus PodSecurity portant sur un fichier local, l'explication MUST
  montrer la version corrigée du `securityContext` sur ce fichier, comme `check`.
- **FR-004**: `check` MUST signaler par une information (sans changer le code de sortie) un
  conteneur sans `resources.limits`.
- **FR-005**: `diagnose deployment/<nom>` MUST lire la condition `ReplicaFailure` et les
  événements `FailedCreate` du ReplicaSet courant, et expliquer le refus avec l'entrée du
  catalogue correspondante.
- **FR-006**: La vérification différée MUST couvrir aussi `kubectl scale` et signaler des pods
  refusés par l'admission.
- **FR-007**: Un avertissement `Warning: would violate PodSecurity` d'une commande réussie MUST
  être expliqué par une information non bloquante après la commande, au plus une fois par
  namespace et par 24 h.

#### Placement et maintenance des nœuds (US2)

- **FR-008**: Le catalogue MUST reconnaître les arrêts de `drain` dus à des pods de DaemonSet, à
  un PodDisruptionBudget et à un stockage local, avec leurs corrections ; `--disable-eviction`
  et `--force` MUST être présentés comme anti-patterns.
- **FR-009**: Le diagnostic d'un pod `Pending` MUST distinguer les causes taint non toléré
  (taint extrait), anti-affinité de pods, nœud non planifiable (`cordon`), en plus des causes
  existantes, et lister chaque cause du message du scheduler avec son nombre de nœuds.
- **FR-010**: L'explication d'un taint non toléré MUST rappeler qu'une tolérance autorise sans
  forcer le placement.

#### Jobs, CronJobs, DaemonSets, StatefulSets (US3)

- **FR-011**: `diagnose` MUST accepter `job/`, `cronjob/` (`cj/`), `daemonset/` (`ds/`) et
  `statefulset/` (`sts/`), sur le modèle de `deployment/` : condition de l'objet, puis pod en
  difficulté le plus récent.
- **FR-012**: Les causes couvertes MUST inclure : `BackoffLimitExceeded`, `DeadlineExceeded`,
  pods supprimés par `ttlSecondsAfterFinished` ; CronJob suspendu, jamais déclenché,
  exécutions sautées par `concurrencyPolicy: Forbid`, `timeZone`, `startingDeadlineSeconds` ;
  DaemonSet incomplet à cause d'un taint ; StatefulSet bloqué par `OrderedReady`, PVC de
  `volumeClaimTemplates` en attente, Service de `serviceName` absent.
- **FR-013**: Le catalogue MUST expliquer une expression cron invalide (5 champs, exemple
  commenté) et `check` MUST valider hors ligne la syntaxe de `spec.schedule` d'un CronJob.
- **FR-014**: `diagnose` sans argument MUST lister aussi les Jobs en échec du namespace.

#### Métriques et autoscaling (US4)

- **FR-015**: Le catalogue MUST reconnaître l'indisponibilité de l'API de métriques
  (`Metrics API not available`, `metrics not available yet`).
- **FR-016**: `diagnose hpa/<nom>` MUST suivre la chaîne : API de métriques disponible, cible
  existante, request déclarée par chaque conteneur pour chaque ressource visée, conditions
  `AbleToScale`, `ScalingActive`, `ScalingLimited` ; le premier maillon cassé est nommé.
- **FR-017**: La vérification différée MUST signaler, après l'application d'un HPA, une cible
  dont un conteneur n'a pas de request pour la ressource visée.
- **FR-018**: `check` MUST signaler hors ligne, par une information, un HPA dont la cible
  présente dans les fichiers vérifiés n'a pas de request pour la ressource visée.

#### Vérifier un rendu (US5)

- **FR-019**: `check -f -` MUST lire l'entrée standard avec la même taille maximale et le même
  nettoyage qu'un fichier, et désigner les positions par `<stdin>:ligne:colonne`.
- **FR-020**: Pour un document précédé d'un commentaire `# Source: <chemin>`, le problème MUST
  citer ce template d'origine.
- **FR-021**: Une entrée standard vide, trop grosse, non UTF-8 ou reliée à un terminal MUST
  produire un message explicatif, sans attente indéfinie.
- **FR-022**: Le README et les informations de `check` sur les templates Helm MUST proposer la
  recette `helm template … | kubectl telemach check -f -`.

#### Autres erreurs courantes (US6)

- **FR-023**: Le catalogue MUST reconnaître un `wait` expiré (condition, jsonpath, delete) et
  proposer `diagnose` de l'objet cité, avec la portée de la commande.
- **FR-024**: Le catalogue MUST reconnaître une révision absente de l'historique pour
  `rollout undo` et des conteneurs éphémères indisponibles pour `debug`.
- **FR-025**: L'approfondissement de l'exécutable introuvable MUST rappeler l'option `--target`
  de `kubectl debug`.
- **FR-026**: Le relevé du mainteneur MUST pouvoir rejouer un dossier de scénarios de pannes et
  lister les messages non reconnus avec leur commande.

#### Lectures du cluster, sécurité et qualité

- **FR-027**: Les nouvelles lectures du cluster MUST s'ajouter à la liste fermée existante et
  se limiter à get/list (Jobs, CronJobs, DaemonSets, StatefulSets, HPA, nœuds, APIService,
  événements d'un ReplicaSet) ; aucune lecture d'objet Secret, aucune écriture.
- **FR-028**: Une lecture refusée MUST rendre le diagnostic partiel avec la lecture nommée.
- **FR-029**: Chaque nouvelle entrée et chaque nouvelle cause MUST être validée par au moins un
  cas réel relevé sur kind pour les trois versions prises en charge (LL-008), ou, à défaut,
  par un message relevé en formation, signalé comme tel.
- **FR-030**: Chaque scénario du guide de validation MUST être rejoué par un test, sous la forme
  exacte écrite dans le guide (LL-004).
- **FR-031**: Les nouveaux rendus MUST passer les contrôles de langue et de masquage des
  features précédentes.

### Key Entities

- **Refus d'admission** : règle du namespace en cause (quota, LimitRange, niveau PodSecurity),
  valeurs extraites du message, objet refusé.
- **Cause de placement** : cause élémentaire du message du scheduler (taint, anti-affinité,
  nœud non planifiable, ressource insuffisante, PVC, sélecteur) et nombre de nœuds concernés.
- **Objet de charge de travail** : Job, CronJob, DaemonSet, StatefulSet ; conditions, pods
  rattachés, objets associés (PVC, Service headless).
- **Chaîne de l'autoscaler** : API de métriques, cible, requests des conteneurs, conditions.
- **Entrée standard vérifiée** : flux lu une fois, désigné `<stdin>`, documents et template
  d'origine (`# Source:`).
- **Relevé de formation** : scénario de panne, commande, message, entrée reconnue ou non.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Sur le corpus des nouveaux messages relevés, 100 % reçoivent la bonne entrée, et
  0 message non reconnu ne reçoit de cause.
- **SC-002**: Le taux global de bonnes explications du corpus d'erreurs reste d'au moins 90 %.
- **SC-003**: Sur le corpus d'objets relevé sur kind, au moins 90 % des Jobs, CronJobs,
  DaemonSets, StatefulSets, HPA et pods `Pending` en difficulté reçoivent la bonne cause, et 0
  objet sain n'est signalé.
- **SC-004**: Rejoué sur les scénarios de pannes des 14 TP, le relevé ne compte plus aucun
  message des familles couvertes par cette feature parmi les non reconnus.
- **SC-005**: Le diagnostic d'un objet s'affiche en moins de 5 secondes sur un cluster local.
- **SC-006**: La vérification différée ne retarde pas l'invite de plus de 50 ms au p95.
- **SC-007**: 0 écriture dans le cluster et 0 lecture hors de la liste fermée pendant les tests.
- **SC-008**: 0 valeur sensible du corpus n'apparaît dans une sortie ou dans l'état enregistré.
- **SC-009**: `check -f -` donne, pour un même contenu, exactement les mêmes problèmes que la
  vérification du fichier correspondant.

## Assumptions

- Les messages des issues #13 à #19 ont été relevés en formation sur un vrai cluster ; ils
  seront relevés de nouveau sur kind pour les trois versions, et les écarts consignés.
- kind permet de créer plusieurs nœuds, de poser des taints, des quotas, des LimitRange et des
  labels PodSecurity ; metrics-server n'y est pas installé par défaut et devra l'être dans le
  cluster jetable de test pour les scénarios d'HPA sain.
- Les conteneurs éphémères sont activés sur toutes les versions prises en charge (1.35 à
  1.37) : le message « ephemeral containers are disabled » peut ne pas être reproductible sur
  kind ; il est alors validé par le message relevé en formation (FR-029).
- Les métriques personnalisées ou externes d'un HPA, la configuration de metrics-server
  (certificats du kubelet) et l'ordonnancement avancé (priorités, préemption) sont hors
  périmètre ; ils sont présentés comme tels, sans cause inventée.
- Les lectures de nœuds et d'APIService sont des lectures d'objets du cluster : un apprenant
  limité à son namespace obtient un diagnostic partiel (FR-028), jamais une demande de droits
  plus larges.
- Les leçons existantes (pods en échec, PVC, Service) sont réutilisées plutôt que dupliquées.
- Les schémas embarqués des CRD (bonus de l'issue #12) restent hors périmètre.
