# Feature Specification: Diagnostiquer ce qui réussit sans marcher

**Feature Branch**: `002-silent-pitfalls`

**Created**: 2026-10-17

**Status**: Draft

**Input**: User description: "Diagnostiquer ce qui réussit sans marcher (feature 002 de TELEMACH
PLUGIN KUBECTL). Étendre le guidage pédagogique aux pièges silencieux, ces commandes kubectl qui
réussissent (code 0) alors que l'application ne fonctionne pas, et compléter le catalogue
d'erreurs." (description complète : Service sans endpoints, `targetPort` incorrect, Ingress
sans classe, ConfigMap ou Secret modifié sans redémarrage, tag d'image mutable, rollout bloqué,
PVC `Pending` ; `diagnose` étendu à `service/`, `deployment/`, `ingress/`, `pvc/` en lecture
seule ; erreurs kubectl et anti-pattern `Terminating` à ajouter au catalogue ; hors périmètre :
suivi de progression, NetworkPolicy, réseau du nœud et CNI.)

## Clarifications

### Session 2026-10-17

- Précisions issues du plan (research.md S2, S9) : FR-018 autorise la lecture de quatre
  annotations publiques posées par Kubernetes ; FR-022 inclut `kubectl expose`.
- Q: Comment détecter qu'une ConfigMap ou un Secret a changé après le démarrage des pods, sachant
  que le plugin ne lit jamais d'objet Secret ? → A: Lecture des **seules métadonnées** des
  ConfigMap référencées par les pods (date de dernière modification), jamais de leurs valeurs ;
  pour un Secret, aucune détection : la leçon est donnée sans affirmer que le Secret a changé.

## Contexte

La feature 001 explique les commandes kubectl **en échec** et les pods qui ne démarrent pas.
Une recherche sur les difficultés courantes des utilisateurs de kubectl (2026-10-17 : guides de
dépannage, cours, forums) montre que la confusion la plus fréquente vient de commandes qui
**réussissent** alors que l'application ne fonctionne pas : `service/web created`,
`deployment.apps/web configured`… puis rien ne répond. Aucun message d'erreur n'existe à
expliquer ; c'est à TELEMACH d'aller voir, en lecture seule, et d'en faire une leçon.

Cette feature reprend sans les modifier les garanties de la feature 001 : français, aucune cause
inventée, aucune action à la place de l'apprenant, secrets masqués, lectures du cluster limitées
à une liste fermée, tests sur un cluster kind jetable, Kubernetes 1.35 à 1.37.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Comprendre pourquoi un Service ne répond pas (Priority: P1)

L'apprenant a créé un Deployment et un Service ; `curl` ou `kubectl port-forward` vers le
Service échoue (`connection refused`, aucune réponse). Il lance
`kubectl telemach diagnose service/web`. TELEMACH suit le chemin du trafic, dans l'ordre où un
formateur le ferait : le selector du Service, les pods qu'il désigne, leur état Ready, puis les
ports. Il nomme le premier maillon cassé, explique le concept en jeu (labels et selectors,
endpoints, `port`/`targetPort`/`containerPort`/`nodePort`) et propose la correction.

**Why this priority**: c'est le piège silencieux le plus cité, et il enseigne le modèle mental
central de Kubernetes (les objets se trouvent par leurs labels, pas par leur nom).

**Independent Test**: sur un cluster kind jetable, créer un Service dont le selector ne
correspond à aucun pod, un autre dont les pods ne sont pas Ready, un autre dont le `targetPort`
ne correspond à aucun port déclaré, et un Service sain ; vérifier que chaque diagnostic nomme la
bonne cause et que le Service sain est déclaré sans problème.

**Acceptance Scenarios**:

1. **Given** un Service dont le selector `app=web` ne correspond à aucun pod (les pods portent
   `app=frontend`), **When** l'apprenant lance `diagnose service/web`, **Then** TELEMACH
   explique qu'un Service trouve ses pods par leurs labels, cite le selector et les labels des
   pods les plus proches dans le namespace, et propose la correction du selector ou des labels
   ainsi que la commande pour comparer (`kubectl get pods --show-labels`).
2. **Given** un Service dont le selector désigne des pods qui ne sont pas Ready, **When**
   l'apprenant lance le diagnostic, **Then** TELEMACH explique qu'un pod n'est ajouté aux
   endpoints que lorsqu'il est Ready, nomme les pods concernés et leur état, et propose
   `kubectl telemach diagnose pod/<nom>`.
3. **Given** un Service dont le `targetPort` nommé (`http`) n'existe dans aucun conteneur des
   pods désignés, ou dont le `targetPort` numérique ne figure pas parmi les ports déclarés des
   conteneurs, **When** l'apprenant lance le diagnostic, **Then** TELEMACH explique la chaîne
   `port` → `targetPort` → port du conteneur, cite les valeurs observées et propose la
   correction ; si le conteneur ne déclare aucun port, il le dit et indique comment vérifier
   le port réellement écouté, sans affirmer de cause.
4. **Given** un Service sans selector (endpoints gérés à la main), **When** l'apprenant lance le
   diagnostic, **Then** TELEMACH explique ce cas particulier au lieu de signaler une erreur.
5. **Given** un Service sain (endpoints prêts, ports cohérents), **When** l'apprenant lance le
   diagnostic, **Then** TELEMACH le confirme, rappelle le nom DNS du Service dans son namespace
   et depuis un autre namespace (`web.demo`, `web.demo.svc.cluster.local`), et propose
   `kubectl port-forward service/web` pour tester.

---

### User Story 2 - Comprendre un déploiement qui n'avance pas (Priority: P2)

L'apprenant applique une nouvelle version : `deployment.apps/web configured`. L'application
répond encore, mais avec l'ancienne version, ou la nouvelle n'arrive jamais. Il lance
`kubectl telemach diagnose deployment/web`. TELEMACH décrit l'état du rollout (nouvelle et
ancienne version, nombre de pods prêts, échéance de progression), explique pourquoi les anciens
pods continuent de servir, puis diagnostique les pods de la nouvelle version avec les causes de
la feature 001.

**Why this priority**: c'est le deuxième grand « ça a marché mais rien n'a changé » ; il touche
aussi la notion de version d'une application (ReplicaSet, rollout, tag d'image).

**Independent Test**: sur un cluster jetable, mettre à jour un Deployment vers une image
inexistante (rollout bloqué), vers une version dont la sonde de disponibilité échoue, puis
réappliquer un manifeste au tag `latest` inchangé ; vérifier le diagnostic de chaque cas.

**Acceptance Scenarios**:

1. **Given** un rollout bloqué (nouveaux pods en `ImagePullBackOff`, anciens pods en service),
   **When** l'apprenant lance `diagnose deployment/web`, **Then** TELEMACH explique le rollout
   progressif (ReplicaSet ancien et nouveau), indique que l'ancienne version sert encore le
   trafic, nomme la cause dans les nouveaux pods et propose la correction ainsi que
   `kubectl rollout undo deployment/web` comme retour arrière, présenté comme un choix de
   l'apprenant.
2. **Given** un Deployment dont l'échéance de progression est dépassée
   (`ProgressDeadlineExceeded`), **When** l'apprenant lance le diagnostic, **Then** TELEMACH
   explique que Kubernetes ne revient pas en arrière de lui-même et que l'échéance est un
   symptôme, la cause étant dans les nouveaux pods.
3. **Given** un Deployment dont l'image utilise le tag `latest` ou aucun tag, **When**
   l'apprenant lance le diagnostic, **Then** TELEMACH signale, sous forme d'information non
   bloquante, qu'un tag mutable ne déclenche aucun nouveau déploiement quand l'image change
   sous le même nom, et recommande un tag explicite.
4. **Given** un Deployment sain et entièrement déployé, **When** l'apprenant lance le
   diagnostic, **Then** TELEMACH le confirme avec la version (image) en service.
5. **Given** un Deployment dont les pods utilisent une ConfigMap en variables d'environnement,
   et cette ConfigMap modifiée après le démarrage des pods, **When** l'apprenant lance le
   diagnostic, **Then** TELEMACH explique que les variables d'environnement sont figées au
   démarrage d'un pod (de même que les montages `subPath`) et propose
   `kubectl rollout restart deployment/web`. La modification est déduite des seules
   métadonnées de la ConfigMap (date de dernière modification), jamais de ses valeurs.
6. **Given** un Deployment dont les pods utilisent un Secret en variables d'environnement,
   **When** l'apprenant lance le diagnostic, **Then** TELEMACH rappelle, sous forme
   d'information, que les variables issues d'un Secret sont elles aussi figées au démarrage,
   sans affirmer que le Secret a changé (le plugin ne lit jamais d'objet Secret).

---

### User Story 3 - Comprendre un Ingress ou un stockage qui reste en attente (Priority: P3)

L'apprenant expose son application par un Ingress et obtient une page 404 sur tous les chemins,
ou crée un PersistentVolumeClaim qui reste `Pending`. Il lance
`kubectl telemach diagnose ingress/web` ou `diagnose pvc/data`. TELEMACH vérifie la classe
d'Ingress, l'adresse attribuée par le contrôleur, les règles et les Services visés ; pour un PVC,
la classe de stockage, la classe par défaut et le mode de liaison.

**Why this priority**: ces objets viennent plus tard dans l'apprentissage ; ils dépendent de
composants installés dans le cluster (contrôleur d'Ingress, provisionneur de stockage), ce qui
rend les causes plus variées.

**Independent Test**: sur un cluster kind jetable (qui fournit une StorageClass par défaut),
créer un Ingress sans `ingressClassName`, un Ingress vers un Service inexistant, un PVC dont la
classe n'existe pas et un PVC `WaitForFirstConsumer` sans pod ; vérifier chaque diagnostic.

**Acceptance Scenarios**:

1. **Given** un Ingress sans `ingressClassName` alors qu'aucune classe d'Ingress n'est marquée
   par défaut, **When** l'apprenant lance `diagnose ingress/web`, **Then** TELEMACH explique
   qu'aucun contrôleur ne prend en charge cet Ingress, liste les classes disponibles et propose
   d'ajouter `ingressClassName`.
2. **Given** un Ingress qui n'a reçu aucune adresse, **When** l'apprenant lance le diagnostic,
   **Then** TELEMACH explique qu'un Ingress n'est qu'une règle, appliquée par un contrôleur, et
   que l'absence d'adresse signifie que le contrôleur ne l'a pas (encore) traité ; si aucune
   classe d'Ingress n'existe, il explique qu'aucun contrôleur n'est installé.
3. **Given** un Ingress dont une règle vise un Service inexistant ou un port que ce Service
   n'expose pas, **When** l'apprenant lance le diagnostic, **Then** TELEMACH nomme la règle et
   le Service et propose la correction ; si le Service existe, il propose aussi
   `diagnose service/<nom>`.
4. **Given** un PVC dont `storageClassName` ne correspond à aucune StorageClass, ou sans classe
   alors qu'aucune n'est marquée par défaut, **When** l'apprenant lance `diagnose pvc/data`,
   **Then** TELEMACH explique le provisionnement dynamique, liste les classes disponibles avec
   la classe par défaut, et propose la correction.
5. **Given** un PVC `Pending` dont la classe utilise le mode de liaison `WaitForFirstConsumer`
   et qu'aucun pod n'utilise encore, **When** l'apprenant lance le diagnostic, **Then**
   TELEMACH explique que cet état est **normal** et que le volume sera créé au démarrage du
   premier pod qui l'utilise, sans le présenter comme une erreur.

---

### User Story 4 - Être prévenu d'un piège juste après l'avoir créé (Priority: P4)

L'intégration shell vérifie déjà, environ 10 secondes après un `apply`, `create` ou `run` réussi,
l'état des pods créés (feature 001). Elle le fait désormais aussi après `expose`, la façon la
plus courante de créer un Service, et signale, à l'invite suivante et en une seule ligne, les pièges détectables à ce moment-là pour les objets que l'apprenant vient de
créer ou de modifier : Service qui ne désigne aucun pod, `targetPort` nommé introuvable, Ingress
sans classe pris en charge, PVC dont la classe n'existe pas, image au tag mutable.

**Why this priority**: c'est ce qui rend les pièges visibles sans que l'apprenant sache qu'il
doit chercher ; elle vient en dernier car elle repose sur les diagnostics des histoires 1 à 3.

**Independent Test**: dans un shell interactif avec l'intégration active, appliquer chacun des
manifestes piégés puis un manifeste sain ; vérifier qu'une ligne s'affiche à l'invite suivante
pour chaque piège, rien pour le manifeste sain, et que l'invite n'est pas retardée.

**Acceptance Scenarios**:

1. **Given** l'apprenant vient d'appliquer un Service dont le selector ne correspond à aucun pod
   du namespace, **When** l'invite suivante s'affiche après la vérification, **Then** une seule
   ligne apparaît : « ⚠ Le Service web ne désigne aucun pod : kubectl telemach diagnose
   service/web -n demo ».
2. **Given** l'apprenant vient d'appliquer un manifeste dont l'image n'a pas de tag explicite,
   **When** l'invite suivante s'affiche, **Then** une ligne d'information (ℹ) le signale, au
   plus une fois par image et par 24 heures.
3. **Given** des pods encore en cours de création au moment de la vérification (pas encore
   Ready), **When** l'invite suivante s'affiche, **Then** aucun Service n'est signalé pour ce
   seul motif.
4. **Given** `TELEMACH_WATCH=0` ou `TELEMACH_DISABLE=1`, **When** l'apprenant applique un
   manifeste, **Then** aucune lecture du cluster n'a lieu.

---

### User Story 5 - Comprendre de nouvelles erreurs kubectl courantes (Priority: P5)

L'apprenant rencontre des erreurs absentes du catalogue de la feature 001 ; TELEMACH les
explique au format habituel.

**Why this priority**: c'est le prolongement direct de la feature 001, sans nouvelle lecture du
cluster ; chaque entrée est indépendante.

**Independent Test**: pour chaque erreur, rejouer la commande sur un cluster kind jetable,
relever le message réel et vérifier l'explication.

**Acceptance Scenarios**:

1. **Given** `kubectl exec` ou `kubectl logs` avec un nom de conteneur mal écrit (`unable to
   upgrade connection: container not found`, `container … is not valid`), **When** l'erreur est
   expliquée, **Then** TELEMACH explique qu'un pod peut contenir plusieurs conteneurs et propose
   la commande qui les liste.
2. **Given** `kubectl logs` sur un pod à plusieurs conteneurs sans `-c` (`a container name must
   be specified`), **When** l'erreur est expliquée, **Then** TELEMACH propose la commande
   corrigée avec chaque conteneur du message.
3. **Given** un échec de `kubectl port-forward` (port local déjà utilisé, pod introuvable ou
   arrêté, port du pod qui refuse la connexion), **When** l'erreur est expliquée, **Then**
   TELEMACH distingue le port de la machine du port du pod et propose la correction adaptée,
   dont le transfert vers `service/<nom>` plutôt que vers un pod dont le nom change à chaque
   déploiement.
4. **Given** `kubectl rollout status` qui échoue sur l'échéance de progression (`exceeded its
   progress deadline`), **When** l'erreur est expliquée, **Then** TELEMACH explique le rollout
   et propose `kubectl telemach diagnose deployment/<nom>`.
5. **Given** un `kubectl apply` refusé parce qu'un champ immuable a changé (`field is
   immutable`, par exemple le selector d'un Deployment), **When** l'erreur est expliquée,
   **Then** TELEMACH explique pourquoi ce champ ne peut pas changer et propose les deux voies
   possibles (garder l'ancien selector, ou supprimer puis recréer l'objet en connaissant la
   coupure de service), sans jamais exécuter la suppression.
6. **Given** un pod en échec dont le journal contient une erreur de résolution DNS d'un nom
   court de Service situé dans un autre namespace, **When** l'apprenant lance `diagnose`,
   **Then** TELEMACH explique la portée d'un nom court et propose `service.namespace` ou le nom
   complet `service.namespace.svc.cluster.local`.
7. **Given** une commande `kubectl delete … --force --grace-period=0` ou la suppression des
   finalizers d'un objet bloqué en `Terminating` (`kubectl patch … finalizers`), **When**
   TELEMACH explique l'erreur ou la situation qui y a conduit, **Then** il présente ces gestes
   comme un **anti-pattern** (données potentiellement corrompues, garanties d'un StatefulSet
   rompues, ressources orphelines) et propose d'abord d'identifier ce qui bloque (finalizers,
   nœud injoignable, volume encore attaché).

---

### Edge Cases

- Plusieurs causes à la fois (selector correct mais pods non Ready **et** `targetPort` faux) :
  TELEMACH nomme d'abord le maillon le plus en amont du chemin du trafic, puis liste les
  autres problèmes trouvés.
- Service de type `ExternalName` ou sans selector : présenté comme un cas particulier, jamais
  comme une erreur.
- Service désignant des pods d'un autre namespace : impossible par construction ; TELEMACH
  explique que le selector ne cherche que dans le namespace du Service.
- Lecture refusée par RBAC (EndpointSlice, IngressClass, StorageClass sont souvent moins
  accessibles qu'un pod) : diagnostic marqué partiel, lecture refusée nommée, aucune cause
  affirmée à partir d'une information manquante.
- Objets à droits de cluster (IngressClass, StorageClass) : lus seulement s'ils sont lisibles ;
  sinon, le diagnostic le dit et reste partiel.
- Rollout en cours depuis moins longtemps que son échéance : présenté comme en cours, jamais
  comme bloqué.
- Deployment en pause (`kubectl rollout pause`) : l'échéance n'est pas évaluée ; TELEMACH le
  dit et rappelle `kubectl rollout resume`.
- Image référencée par digest (`@sha256:…`) : jamais signalée comme tag mutable.
- `targetPort` numérique alors que le conteneur ne déclare aucun port : pas de cause affirmée ;
  explication de la déclaration facultative des ports et de la façon de vérifier le port
  écouté.
- Ingress dont la classe est fournie par l'ancienne annotation `kubernetes.io/ingress.class` :
  reconnue comme une classe, avec mention que l'annotation est dépréciée au profit de
  `ingressClassName`.
- Cluster sans aucun contrôleur d'Ingress (cas de kind sans installation) : expliqué comme tel,
  sans prétendre qu'un contrôleur précis est défaillant.
- Nom ambigu (`diagnose web` alors qu'un pod, un Service et un Deployment s'appellent `web`) :
  TELEMACH liste les objets correspondants et les commandes à copier, sans choisir à la place
  de l'apprenant.
- Vérification différée : si les lectures n'ont pas abouti dans le temps imparti, rien n'est
  signalé (pas de signal fondé sur une information incomplète).

## Requirements *(mandatory)*

### Functional Requirements

#### Diagnostic d'un Service (US1)

- **FR-001**: `kubectl telemach diagnose service/<nom>` (et `svc/<nom>`) MUST examiner, dans cet
  ordre, le selector du Service, les pods du namespace qu'il désigne, leur état Ready, les
  endpoints du Service et la cohérence des ports, et MUST nommer le premier maillon défaillant
  avant de lister les autres.
- **FR-002**: Quand le selector ne désigne aucun pod, le diagnostic MUST citer le selector et,
  s'ils existent, au plus 3 groupes de labels de pods du namespace les plus proches de ce
  selector, ainsi que les commandes de comparaison.
- **FR-003**: Le diagnostic MUST expliquer `port`, `targetPort`, `containerPort` et, pour un
  Service `NodePort` ou `LoadBalancer`, `nodePort`, avec les valeurs observées ; il MUST
  signaler un `targetPort` nommé absent des conteneurs désignés et un `targetPort` numérique
  absent des ports déclarés, en précisant dans ce second cas que la déclaration des ports est
  facultative.
- **FR-004**: Un Service sans selector ou de type `ExternalName` MUST être expliqué comme un cas
  particulier, sans être signalé en erreur.
- **FR-005**: Pour un Service sain, le diagnostic MUST le confirmer et donner ses noms DNS
  (nom court dans le namespace, `nom.namespace` et `nom.namespace.svc.cluster.local` depuis un
  autre namespace) ainsi qu'une commande de test par `kubectl port-forward service/<nom>`.

#### Diagnostic d'un Deployment (US2)

- **FR-006**: `kubectl telemach diagnose deployment/<nom>` MUST décrire l'état du rollout :
  image de la version en cours de déploiement et de la version précédente, pods prêts de
  chacune, état de l'échéance de progression (en cours, dépassée, en pause, terminé).
- **FR-007**: En cas de rollout bloqué ou d'échéance dépassée, le diagnostic MUST expliquer que
  l'ancienne version peut continuer à servir, que Kubernetes ne revient pas en arrière de
  lui-même, et MUST diagnostiquer le pod en difficulté le plus récent de la nouvelle version
  avec les causes de la feature 001 (FR-025 de la feature 001).
- **FR-008**: Le diagnostic MUST proposer `kubectl rollout undo` comme une option de l'apprenant,
  jamais comme une action du plugin, et MUST rappeler `kubectl rollout status` pour suivre un
  déploiement.
- **FR-009**: Une image sans tag ou au tag `latest` MUST faire l'objet d'une information non
  bloquante (principe III de la constitution) expliquant qu'un tag mutable ne déclenche pas de
  nouveau déploiement ; une image référencée par digest MUST NOT être signalée.
- **FR-010**: Le diagnostic MUST expliquer que les variables d'environnement issues d'une
  ConfigMap ou d'un Secret (et les montages `subPath`) sont figées au démarrage du pod et
  proposer `kubectl rollout restart` ; il MUST signaler qu'une ConfigMap référencée en variable
  d'environnement ou en montage `subPath` a été modifiée après le démarrage du plus ancien pod
  en service, en comparant la date de dernière modification lue dans les **seules métadonnées**
  de la ConfigMap à la date de démarrage des pods. Pour un Secret, aucune détection : la leçon
  est donnée sous forme d'information, sans affirmer de changement.

#### Diagnostic d'un Ingress et d'un PVC (US3)

- **FR-011**: `kubectl telemach diagnose ingress/<nom>` (et `ing/<nom>`) MUST vérifier la classe
  d'Ingress (champ `ingressClassName`, ancienne annotation, classe par défaut), l'adresse
  attribuée, et, pour chaque règle, l'existence du Service visé et du port demandé.
- **FR-012**: L'absence de toute classe d'Ingress dans le cluster MUST être expliquée comme
  l'absence de contrôleur installé, sans désigner de contrôleur particulier.
- **FR-013**: `kubectl telemach diagnose pvc/<nom>` MUST vérifier l'existence de la classe de
  stockage demandée, la présence d'une classe par défaut, le mode de liaison de la classe et
  l'existence d'un pod qui utilise le PVC.
- **FR-014**: Un PVC `Pending` en mode `WaitForFirstConsumer` sans pod consommateur MUST être
  présenté comme un état normal, avec l'explication du mode de liaison.

#### Diagnostic sans argument et noms ambigus

- **FR-015**: `kubectl telemach diagnose` sans argument MUST conserver le comportement de la
  feature 001 (pods en échec du namespace) et MUST lister ensuite, au plus 10 au total, les
  Services sans pod prêt, les Deployments dont le rollout est bloqué, les Ingress sans classe
  prise en charge et les PVC en attente anormale du namespace, chacun avec la commande
  `diagnose <type>/<nom>` à copier.
- **FR-016**: Un nom sans type correspondant à plusieurs objets MUST produire la liste des
  objets correspondants et des commandes à copier, sans choix arbitraire.

#### Lectures du cluster et sécurité

- **FR-017**: Les lectures du cluster MUST rester une liste fermée, documentée dans
  `SECURITY.md`, étendue aux seules lectures nécessaires : Service, EndpointSlice, Ingress,
  IngressClass, PersistentVolumeClaim, StorageClass, ReplicaSet et les **métadonnées** des
  ConfigMap référencées par les pods diagnostiqués, en plus des lectures de la feature 001 ; toute lecture MUST passer par la commande kubectl de l'apprenant, avec ses
  droits, son contexte et son namespace.
- **FR-018**: Le plugin MUST NOT lire d'objet Secret ni décoder une valeur de ConfigMap (`data`,
  `binaryData`), une valeur de variable d'environnement ou une annotation (garantie de la
  feature 001 étendue aux nouveaux objets) ; d'une ConfigMap, seuls le nom, le namespace et les
  dates de modification sont conservés. Seule exception : quatre annotations publiques posées
  par Kubernetes, sans donnée de l'apprenant, sont lues parce qu'elles portent seules
  l'information nécessaire (classe d'Ingress et classe de stockage par défaut, ancienne classe
  d'un Ingress, révision d'un ReplicaSet) ; toute autre annotation est ignorée dès la lecture.
- **FR-019**: Le plugin MUST NOT exécuter de commande dans un conteneur, ouvrir de
  `port-forward`, ni modifier ou supprimer une ressource : les commandes de test et de
  correction sont affichées, jamais exécutées.
- **FR-020**: Une lecture refusée MUST rendre le diagnostic partiel, avec la lecture nommée, et
  aucune cause MUST NOT être affirmée à partir d'une information non lue.
- **FR-021**: Chaque diagnostic MUST suivre le format de leçon de la feature 001 (résumé,
  concept, cause probable, corrections prêtes à copier qui conservent contexte et namespace,
  anti-pattern éventuel, renvoi à `deepen`) et devenir le dernier diagnostic pour `last` et
  `deepen`.

#### Vérification différée (US4)

- **FR-022**: Après un `apply`, `create`, `run` ou `expose` réussi, la vérification différée MUST, en plus
  des pods (feature 001), signaler en une ligne par objet, pour les seuls objets créés ou
  modifiés par la commande : un Service dont le selector ne désigne aucun pod, un `targetPort`
  nommé introuvable, un Ingress sans classe prise en charge, un PVC dont la classe n'existe
  pas, chaque ligne avec la commande `diagnose` à copier.
- **FR-023**: Une image sans tag explicite dans un manifeste appliqué MUST produire au plus une
  ligne d'information (ℹ) par image et par 24 heures, à partir du seul fichier local (aucune
  lecture du cluster pour ce cas).
- **FR-024**: Un piège qui dépend d'un état transitoire (pods pas encore Ready, rollout en
  cours, PVC en attente d'un premier pod) MUST NOT être signalé par la vérification différée.
- **FR-025**: La vérification différée MUST conserver les garanties de la feature 001 : aucune
  sortie pendant une saisie, invite non retardée, désactivation par `TELEMACH_WATCH=0` et
  `TELEMACH_DISABLE=1`, rien de signalé si les lectures n'ont pas abouti.

#### Catalogue d'erreurs (US5)

- **FR-026**: Le catalogue MUST reconnaître et expliquer : nom de conteneur inconnu pour `exec`
  et `logs`, conteneur non précisé pour `logs` sur un pod à plusieurs conteneurs, échecs de
  `port-forward` (port local occupé, pod introuvable ou arrêté, connexion refusée par le pod),
  échéance dépassée de `rollout status`, champ immuable refusé à l'`apply`.
- **FR-027**: Les motifs de journal de `diagnose` MUST reconnaître une résolution DNS en échec
  d'un nom court et expliquer la portée des noms de Service par namespace.
- **FR-028**: Les commandes `delete --force --grace-period=0` et la suppression des finalizers
  MUST être présentées comme des anti-patterns chaque fois qu'elles apparaissent dans une
  correction possible, avec la démarche d'identification de ce qui bloque.
- **FR-029**: Chaque nouvelle entrée et chaque piège MUST être validé par au moins un cas réel
  relevé sur un cluster kind jetable pour chacune des versions 1.35, 1.36 et 1.37 (message et
  état réels conservés dans le corpus de test), comme pour la feature 001.

#### Qualité et tests

- **FR-030**: Chaque scénario du guide de validation de cette feature MUST être rejoué par un
  test automatisé, avec la forme exacte de la commande écrite dans le guide (LL-004).
- **FR-031**: Les rendus de référence de cette feature MUST passer les contrôles de langue et de
  masquage existants (SC-005 et SC-006 de la feature 001).
- **FR-032**: Les commandes proposées MUST NOT utiliser l'API Endpoints, dépréciée depuis
  Kubernetes 1.33 au profit d'EndpointSlice ; les entrées existantes du catalogue qui
  proposent `kubectl get endpoints` MUST être corrigées
  (`kubectl get endpointslices -l kubernetes.io/service-name=<nom>`).

### Key Entities

- **Piège silencieux (Pitfall)** : situation où une commande réussit sans que l'application
  fonctionne ; attributs : type d'objet, maillon concerné (selector, readiness, port, classe,
  rollout, tag, configuration), gravité (avertissement ou information), état transitoire ou
  non, entrée du catalogue associée.
- **Instantané d'objet (ObjectSnapshot)** : vue masquée d'un Service, Deployment, Ingress ou PVC
  et des objets liés lus pour le diagnostic (pods désignés, ReplicaSets, classes) ; ne contient
  jamais de valeur sensible.
- **Chemin du trafic (TrafficPath)** : suite ordonnée des maillons examinés pour un Service ou
  un Ingress (règle → Service → selector → pods → Ready → port), chacun avec son état (sain,
  défaillant, non lu).
- **Dernier diagnostic** : entité existante de la feature 001, étendue aux nouveaux types
  d'objets.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Sur le corpus de référence relevé sur kind, au moins 90 % des pièges silencieux
  sont diagnostiqués avec la bonne cause, pour chacune des versions 1.35, 1.36 et 1.37.
- **SC-002**: 0 objet sain du corpus n'est signalé comme piège, que ce soit par `diagnose` ou
  par la vérification différée.
- **SC-003**: 100 % des nouvelles erreurs du catalogue du corpus sont reconnues, et 0 erreur non
  reconnue ne reçoit de cause inventée.
- **SC-004**: Le diagnostic d'un objet s'affiche en moins de 5 secondes sur un cluster local
  dans 95 % des cas.
- **SC-005**: 0 écriture dans le cluster et 0 lecture hors de la liste fermée pendant
  l'ensemble des tests (vérifié par l'état du namespace avant et après et par le journal des
  appels).
- **SC-006**: 0 valeur sensible du corpus n'apparaît dans une sortie ou dans les données
  conservées du plugin ; 100 % des textes rédigés sont en français.
- **SC-007**: La vérification différée ne retarde pas l'invite de plus de 50 ms au p95 (même
  objectif que la feature 001) et signale chaque piège non transitoire du corpus à l'invite qui
  suit la vérification.
- **SC-008**: Lors d'essais avec des débutants, au moins 80 % trouvent la cause d'un Service qui
  ne répond pas après lecture du diagnostic, sans autre ressource.

## Assumptions

- Les apprenants utilisent un cluster local ou de formation (kind, minikube, microk8s) ; kind
  fournit une StorageClass par défaut (`standard`, mode `WaitForFirstConsumer`) mais aucun
  contrôleur d'Ingress : les scénarios d'Ingress avec contrôleur nécessitent son installation
  dans le cluster jetable de test.
- Le port réellement écouté par un conteneur n'est pas observable sans exécuter de commande dans
  le conteneur, ce que le plugin s'interdit : la vérification des ports se limite aux ports
  déclarés, et le reste est enseigné comme une démarche.
- L'échéance de progression par défaut d'un Deployment (600 s) dépasse le délai de la
  vérification différée (10 s) : un rollout bloqué est donc diagnostiqué par
  `diagnose deployment/<nom>` et par l'erreur de `rollout status`, pas par la vérification
  différée.
- EndpointSlice remplace Endpoints pour lire les endpoints d'un Service (Endpoints est déprécié
  depuis Kubernetes 1.33).
- Le suivi de la progression de l'apprenant, les NetworkPolicy, le réseau des nœuds et le CNI
  sont hors périmètre ; un Service dont les endpoints sont prêts mais qui ne répond pas est
  présenté comme hors de portée du diagnostic, avec des pistes, sans cause inventée.
- Les leçons du catalogue de la feature 001 sont réutilisées quand elles s'appliquent (pods en
  échec, connexion, RBAC) plutôt que dupliquées.
