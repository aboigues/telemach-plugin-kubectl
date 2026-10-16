# Feature Specification: Guidage pédagogique à partir des erreurs kubectl

**Feature Branch**: `001-kubectl-error-guidance`

**Created**: 2026-10-16

**Status**: Draft

**Input**: User description: "le plugin aura pour vocation à aider l'utilisateur apprenant à
comprendre ses erreurs de syntaxe avec la ligne, à aider diagnostiquer ce qui ne va de la même
manière que le propose le plugin ../telemach-plugin-docker pour docker"

## Clarifications

### Session 2026-10-16

- Q: Le diagnostic de l'état des ressources du cluster (pod `Pending`, `CrashLoopBackOff`…)
  fait-il partie de cette feature ? → A: Oui, en priorité P4, au moyen de **lectures seules**
  du cluster limitées à une liste fermée (FR-022 à FR-027), testées sur un cluster jetable.
- Q: Que doit afficher `kubectl telemach diagnose` sans argument quand plusieurs pods du
  namespace sont en difficulté ? → A: L'explication complète du pod le plus récent, puis la
  liste courte des autres (nom, état) avec la commande pour diagnostiquer chacun.
- Q: Après un `kubectl apply`/`create`/`run` réussi, le plugin doit-il orienter l'apprenant vers
  `diagnose` ? → A: Oui, par une vérification différée (`watch`) unique, en lecture seule, de l'état
  des pods concernés quelques secondes après la commande, avec un signal d'une ligne s'ils sont
  en échec, désactivable (sur le modèle de la feature 004 du projet Docker) (FR-028 à FR-031).
- Q: Sur quelle version des règles de Kubernetes s'appuie la vérification hors ligne des
  manifestes ? → A: Les trois dernières versions mineures maintenues, embarquées ; choix
  automatique selon la version du client `kubectl` (FR-001b), la plus récente par défaut
  (FR-014a).
- Q: Que fait `kubectl telemach check` sans fichier précisé ? → A: Rien n'est lu ; un message
  pédagogique explique l'option `-f` (fichier, dossier, `-R`) et donne un exemple (FR-017a).
- Q: Comment traiter l'alias `k` ? → A: S'il est déjà défini comme alias de `kubectl` à
  l'initialisation de l'intégration shell, il est couvert automatiquement ; sinon rien n'est
  créé ni modifié (FR-001a).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Comprendre l'échec d'une commande kubectl (Priority: P1)

Un apprenant lance une commande `kubectl` courante (`get`, `describe`, `apply`, `create`,
`run`, `delete`, `logs`, `exec`, `expose`, `scale`, `rollout`…) qui échoue. Au lieu de rester
face à un message technique en anglais, il reçoit du plugin, en français, une explication
structurée : ce qui s'est passé, le concept Kubernetes concerné (pod, deployment, service,
namespace, contexte, RBAC…), la cause probable et une ou plusieurs actions correctives
concrètes, avec la commande corrigée lorsque c'est possible.

Cette histoire couvre aussi les **erreurs d'écriture de la ligne de commande** que `kubectl`
refuse : type de ressource inconnu ou mal orthographié (`kubectl get pod` ✔, `kubectl get
pdos` ✖), option inconnue, argument manquant, nom de ressource invalide.

**Why this priority**: c'est la situation la plus fréquente chez un débutant et le cœur de la
mission pédagogique du plugin (principe I). À elle seule, cette histoire constitue un MVP utile.

**Independent Test**: peut être testé en provoquant des erreurs connues (type de ressource mal
orthographié, ressource introuvable, namespace inexistant, cluster injoignable, accès refusé par
RBAC, option inconnue) et en vérifiant que chaque erreur produit une explication complète et
correcte en français.

**Acceptance Scenarios**:

1. **Given** l'apprenant fait une faute de frappe dans un type de ressource, **When** il exécute
   `kubectl get deploymnet`, **Then** le plugin explique ce qu'est un type de ressource
   Kubernetes, signale que celui-ci n'existe pas et suggère le type probablement voulu
   (`deployment`), avec la commande corrigée et un rappel de `kubectl api-resources`.
2. **Given** un pod `web` existe dans le namespace `demo` mais pas dans `default`, **When**
   l'apprenant exécute `kubectl get pod web`, **Then** le plugin explique la notion de
   namespace, indique que la recherche a eu lieu dans le namespace courant et propose
   `kubectl get pod web -n demo` ainsi que la commande pour lister les pods de tous les
   namespaces.
3. **Given** aucun cluster n'est joignable (cluster local arrêté, contexte périmé), **When**
   l'apprenant exécute n'importe quelle commande qui contacte le cluster, **Then** le plugin
   explique la différence entre le client `kubectl` et le serveur API, la notion de contexte et
   de kubeconfig, et indique comment vérifier le contexte courant et démarrer ou joindre le
   cluster, sans jamais afficher le contenu du kubeconfig.
4. **Given** l'utilisateur n'a pas les droits nécessaires, **When** `kubectl` répond
   `Forbidden`, **Then** le plugin explique RBAC (utilisateur, verbe, ressource, namespace),
   reformule ce qui a été refusé et propose `kubectl auth can-i` pour vérifier ses droits, sans
   suggérer de s'attribuer `cluster-admin`.
5. **Given** une erreur que le plugin ne reconnaît pas, **When** elle survient, **Then** le
   plugin l'indique honnêtement, reformule en français ce qu'il peut en déduire et oriente vers
   la documentation officielle, sans inventer de cause.

---

### User Story 2 - Localiser une erreur dans un manifeste YAML, à la ligne près (Priority: P2)

Un apprenant lance `kubectl apply -f`, `kubectl create -f` ou `kubectl diff -f` et obtient une
erreur liée à son manifeste : YAML mal formé (indentation, tabulations, deux-points manquants,
séparateur `---` mal placé), champ inconnu ou mal orthographié, type de valeur incorrect,
`apiVersion` ou `kind` absent ou incompatible, champ obligatoire manquant, incohérence entre les
labels du selector et ceux du template. Le message de `kubectl` donne parfois une ligne
(`yaml: line 7: …`), parfois seulement un chemin de champ (`spec.template.spec.containers[0]`),
souvent sans le nom du fichier ni du document concerné. Le plugin relit le ou les manifestes
utilisés, localise l'élément fautif (fichier, document, ligne, colonne, ressource, champ),
l'affiche avec son contexte, explique la règle enfreinte et propose une correction.

**Why this priority**: écrire des manifestes est l'étape centrale de l'apprentissage de
Kubernetes ; leurs erreurs sont déroutantes car le message pointe souvent mal la cause réelle
dans le YAML. La valeur pédagogique est forte, mais elle s'appuie sur le socle de l'histoire 1.

**Independent Test**: peut être testé avec un jeu de manifestes volontairement erronés (une
erreur par fichier) en vérifiant que le plugin désigne le bon fichier, la bonne ligne, le bon
champ et la bonne correction.

**Acceptance Scenarios**:

1. **Given** un manifeste dont un conteneur est indenté avec une tabulation, **When**
   l'apprenant exécute `kubectl apply -f deploy.yaml`, **Then** le plugin indique le fichier et
   la ligne exacte, explique que YAML interdit les tabulations pour l'indentation et montre la
   version corrigée.
2. **Given** un conteneur qui écrit `imagePullPolicyy:` ou `containerPort` au mauvais niveau,
   **When** l'apprenant applique le manifeste et que `kubectl` signale un champ inconnu,
   **Then** le plugin traduit le chemin du champ en ligne du fichier, suggère le champ proche
   ou le bon niveau d'indentation et rappelle son rôle.
3. **Given** un `Deployment` dont `spec.selector.matchLabels` ne correspond pas aux labels de
   `spec.template.metadata.labels`, **When** l'apprenant l'applique, **Then** le plugin
   explique le lien entre selector et labels, désigne les deux lignes concernées et propose la
   correction.
4. **Given** un fichier contenant plusieurs documents séparés par `---`, **When** l'erreur
   concerne le troisième, **Then** le plugin indique le numéro du document, sa ressource
   (`kind` et `metadata.name`) et la ligne dans le fichier, pas dans le document isolé.
5. **Given** un manifeste contenant un objet `Secret` ou des valeurs sensibles en clair dans
   `env`, **When** le plugin l'analyse, **Then** il masque ces valeurs dans tout ce qu'il
   affiche et signale la mauvaise pratique en proposant une alternative (objet `Secret`
   référencé par `secretKeyRef`, fichier non versionné).
6. **Given** un manifeste qu'il n'a pas encore appliqué, **When** l'apprenant demande au plugin
   de le vérifier (`kubectl telemach check -f deploy.yaml`), **Then** le plugin applique les
   mêmes contrôles qu'après une erreur, sans contacter le cluster, et liste les problèmes
   trouvés ou confirme explicitement qu'aucun n'a été détecté.

---

### User Story 3 - Approfondir un concept à la demande (Priority: P3)

Après une explication, l'apprenant veut aller plus loin : comprendre pourquoi l'erreur se
produit, voir un schéma mental du mécanisme (relation deployment → replicaset → pod, résolution
d'un service vers ses pods, chemin d'une requête vers le serveur API) ou obtenir un petit
exercice pour vérifier sa compréhension. Le plugin propose un niveau d'explication approfondi,
distinct de l'explication courte initiale (`kubectl telemach deepen`).

**Why this priority**: renforce la mission pédagogique mais n'est pas nécessaire pour résoudre
l'erreur immédiate.

**Independent Test**: peut être testé en demandant l'approfondissement après une explication
de l'histoire 1 ou 2 et en vérifiant que le contenu supplémentaire est exact, en français, et
relié à l'erreur d'origine.

**Acceptance Scenarios**:

1. **Given** une explication courte vient d'être affichée, **When** l'apprenant demande à
   approfondir, **Then** le plugin présente le concept sous-jacent, un exemple exécutable et,
   si pertinent, un lien vers la documentation officielle.
2. **Given** un concept déjà approfondi, **When** l'apprenant demande un exercice, **Then** le
   plugin propose une manipulation courte et sans risque, limitée à un namespace d'exercice
   que l'apprenant crée lui-même, pour vérifier sa compréhension.

---

### User Story 4 - Diagnostiquer une ressource qui ne fonctionne pas (Priority: P4)

La commande `kubectl apply` a réussi, mais l'application ne tourne pas : un pod reste en
`Pending`, `ImagePullBackOff`, `CrashLoopBackOff`, `CreateContainerConfigError` ou est
`OOMKilled`. L'apprenant lance `kubectl telemach diagnose` (sans argument : les pods en
difficulté du namespace courant ; avec argument : un pod ou un deployment précis). Le plugin lit,
en lecture seule et avec les droits de l'apprenant, le statut de la ressource, ses événements
récents et, si un conteneur a redémarré, les dernières lignes de son journal précédent. Il
explique ensuite, au format habituel, l'état observé, le concept Kubernetes en jeu, la cause
probable et les corrections possibles.

**Why this priority**: c'est la deuxième grande source de confusion des débutants (« ça a
marché, mais rien ne tourne »). Elle vient en dernier car elle suppose que le plugin lise
l'état du cluster, ce qui élargit sa surface de sécurité (principe III) : ces lectures sont
donc strictement bornées.

**Independent Test**: sur un cluster jetable, créer un pod volontairement en erreur pour chaque
état listé et vérifier que le plugin nomme l'état, explique sa cause probable à partir des
événements et du statut, et propose une correction, sans rien modifier dans le cluster.

**Acceptance Scenarios**:

1. **Given** un pod en `ImagePullBackOff` à cause d'un tag d'image inexistant, **When**
   l'apprenant demande le diagnostic, **Then** le plugin explique le téléchargement de l'image
   par le nœud, cite l'image en cause et propose de vérifier le nom et le tag.
2. **Given** un pod en `CrashLoopBackOff`, **When** l'apprenant demande le diagnostic, **Then**
   le plugin explique la politique de redémarrage, distingue une erreur de Kubernetes d'une
   erreur de l'application, affiche au plus les 20 dernières lignes (masquées) du journal
   précédent et indique le code de sortie du conteneur avec sa signification.
3. **Given** un pod en `Pending` faute de ressources sur les nœuds ou à cause d'un
   `PersistentVolumeClaim` non lié, **When** l'apprenant demande le diagnostic, **Then** le
   plugin explique le rôle du scheduler, cite l'événement qui bloque et propose une correction
   (requests plus faibles, PVC à vérifier).
4. **Given** un pod en `CreateContainerConfigError` parce qu'il référence un `Secret` ou une
   `ConfigMap` absente, **When** l'apprenant demande le diagnostic, **Then** le plugin nomme
   l'objet manquant sans jamais afficher le contenu d'un `Secret`.
5. **Given** l'apprenant n'a pas le droit de lire les événements ou les journaux, **When** il
   demande le diagnostic, **Then** le plugin explique quelle lecture a été refusée par RBAC,
   produit le diagnostic possible avec les informations restantes et le signale comme partiel.
6. **Given** aucun pod en difficulté dans le namespace, **When** l'apprenant lance
   `kubectl telemach diagnose`, **Then** le plugin le confirme explicitement et rappelle
   comment cibler un autre namespace.
7. **Given** l'intégration shell active, **When** l'apprenant exécute
   `kubectl apply -f deploy.yaml` (code 0) et que les pods du deployment passent en
   `ImagePullBackOff` dans les secondes qui suivent, **Then** à l'affichage suivant de
   l'invite, TELEMACH affiche une seule ligne : « ⚠ Le pod <nom> est en ImagePullBackOff :
   kubectl telemach diagnose pod/<nom> pour comprendre pourquoi ».
8. **Given** la même commande pour un deployment dont les pods démarrent normalement ou sont
   encore en cours de création, **When** la vérification a lieu, **Then** rien n'est affiché.
9. **Given** la vérification désactivée (variable dédiée) ou `TELEMACH_DISABLE=1`, **When**
   l'apprenant applique un manifeste, **Then** TELEMACH n'interroge pas le cluster.

---

### Edge Cases

- **Options globales placées avant la sous-commande** (`kubectl -n demo get pods`,
  `kubectl --context kind-dev apply -f …`) : la commande est reconnue et analysée comme si les
  options étaient placées après (leçon LL-014 du projet Docker).
- **Manifestes multiples** (`-f a.yaml -f b.yaml`, `-f dossier/`, `-R`) : le plugin indique
  dans quel fichier et quel document se trouve l'erreur.
- **Manifeste lu sur l'entrée standard** (`-f -`) ou par URL (`-f https://…`) : le plugin ne
  peut pas relire le contenu ; il explique l'erreur à partir du seul message, sans localisation
  de ligne, et le dit.
- **Plusieurs erreurs dans un même manifeste** : le plugin liste toutes celles qu'il peut
  détecter, dans l'ordre du fichier. Une erreur de syntaxe YAML empêche l'analyse du reste :
  seules les erreurs de syntaxe sont alors listées.
- **Fichier introuvable ou illisible** (droits, encodage) : le plugin explique la cause du refus.
- **`kubectl` est lui-même un alias** (par exemple `microk8s kubectl` ou `minikube kubectl --`) :
  la commande exécutée reste exactement celle de l'alias ; seule l'interception des erreurs
  s'ajoute (FR-001a).
- **Alias `k` défini après l'initialisation de l'intégration** : il n'est pas couvert tant que
  le shell n'a pas été réinitialisé ; la documentation l'indique.
- **`check` sans `-f`** : aucun fichier n'est lu ; le plugin explique l'option `-f` (FR-017a).
- **Ressource personnalisée (CRD) inconnue du plugin** : le plugin le dit, valide uniquement la
  syntaxe YAML et la structure commune (`apiVersion`, `kind`, `metadata`), sans inventer de
  règle sur les champs propres à la ressource.
- **`apiVersion` obsolète ou retirée** (`extensions/v1beta1`) : le plugin explique l'évolution
  des API Kubernetes et donne l'`apiVersion` actuelle.
- **Erreur provenant de l'application dans le conteneur** (et non de Kubernetes) : le plugin
  fait la distinction et oriente vers `kubectl logs`.
- **Commande interactive ou de longue durée** (`exec -it`, `run -it`, `attach`, `edit`,
  `port-forward`, `logs -f`, `get -w`) : l'entrée et la sortie ne sont jamais perturbées ; le
  plugin explique l'échec à partir du code de sortie et de ce qui a pu être capté, sinon il
  l'indique comme « erreur non reconnue ».
- **Sortie ou ligne de commande contenant des secrets** (`--token`, `--password`,
  `create secret generic --from-literal=…`, en-têtes d'authentification, certificats) : jamais
  réaffichés.
- **Capture automatique impossible** (commande lancée hors de l'environnement observé) : aucune
  explication n'est produite ; le plugin ne doit ni échouer ni bloquer la commande.
- **Vérification différée impossible ou inutile** : manifeste lu sur l'entrée standard ou
  par URL, `-k`, `--dry-run`, session non interactive (script), terminal fermé avant la
  vérification : aucune vérification, sans processus ni fichier laissé derrière.
- **Plusieurs créations à la suite** : chaque commande donne lieu à une seule vérification ;
  les signaux sont regroupés à l'invite suivante.
- **Pod qui échoue après la vérification** (au-delà du délai de FR-030) : il n'est pas signalé
  automatiquement ; `kubectl telemach diagnose` le trouve.
- **Même erreur répétée en moins de 10 minutes** : le plugin affiche une version courte et
  indique comment réafficher l'explication complète (`kubectl telemach last`).
- **Version de kubectl ancienne ou hors de la plage des règles embarquées** : le plugin
  connaît la version du client et reconnaît les variantes connues des messages, ou retombe sur
  « erreur non reconnue » en mentionnant la version détectée ; pour `check`, les règles les
  plus récentes sont utilisées et l'apprenant en est informé (FR-014a). La version du cluster
  n'est jamais lue.

## Requirements *(mandatory)*

### Functional Requirements

**Capture et reconnaissance**

- **FR-001**: Le plugin MUST récupérer automatiquement le message d'erreur, la commande et le
  code de sortie dès qu'une commande `kubectl` échoue, sans action de l'apprenant.
- **FR-001a**: Le plugin MUST n'observer que les commandes `kubectl` ; les autres commandes et
  leurs sorties MUST être ignorées et ne jamais être conservées. Si `kubectl` est un alias,
  la commande qu'il désigne MUST rester celle qui est exécutée. Si, à l'initialisation de
  l'intégration shell, l'apprenant a déjà défini l'alias `k` comme `kubectl` (avec ou sans
  options), les commandes tapées avec `k` MUST être couvertes de la même manière, et les
  corrections proposées reprennent la forme `k …`. Si `k` n'est pas défini, ou désigne autre
  chose, l'intégration MUST NOT créer, modifier ni supprimer cet alias.
- **FR-001b**: Le plugin MUST connaître la version du client `kubectl`. Elle est lue une seule
  fois, à l'initialisation de l'intégration shell, par une lecture locale qui ne contacte pas le
  cluster, puis transmise à chaque analyse. Si elle est indisponible, l'analyse continue sans
  elle.
- **FR-002**: Le plugin MUST identifier la commande en cause (sous-commande, type de ressource,
  options, y compris les options globales placées avant la sous-commande) et catégoriser
  l'erreur (ligne de commande, connexion/contexte, authentification/RBAC, ressource
  introuvable, namespace, manifeste/YAML, validation de schéma, conflit, application,
  inconnue).
- **FR-003**: Le plugin MUST couvrir au minimum les sous-commandes `get`, `describe`, `apply`,
  `create`, `delete`, `run`, `expose`, `logs`, `exec`, `scale`, `rollout`, `set image`,
  `edit`, `diff`, `config` (`use-context`, `current-context`, `get-contexts`) et
  `port-forward`.
- **FR-004**: Le plugin MUST distinguer une erreur de `kubectl` ou du cluster d'une erreur de
  l'application exécutée dans le conteneur.

**Explication pédagogique**

- **FR-005**: Chaque explication MUST contenir, en français : un résumé en une phrase, le
  concept Kubernetes concerné, la cause probable, au moins une action corrective concrète et,
  quand c'est possible, la commande ou l'extrait de manifeste corrigé.
- **FR-006**: Les termes, commandes et champs Kubernetes MUST rester dans leur forme
  d'origine ; tout terme introduit pour la première fois MUST être défini brièvement.
- **FR-007**: Le message d'erreur d'origine MUST rester consultable à côté de l'explication.
  Lorsque la couleur est active, il MUST être affiché atténué, avec les fragments reconnus
  (type de ressource, nom, namespace, champ, ligne, chemin) surlignés en couleur.
- **FR-007a**: Toute sortie du plugin MUST être colorée quand la sortie est un terminal :
  chaque type de bloc (résumé, concept, cause, corrections, anti-pattern, message d'origine) a
  son style, et les commandes et extraits YAML bénéficient d'une coloration syntaxique. La
  couleur MUST pouvoir être désactivée (sortie non-terminal, préférence explicite, convention
  `NO_COLOR`) et MUST NOT être le seul porteur d'information : chaque bloc garde un symbole et
  un libellé lisibles sans couleur. Une seule palette est fournie, qui s'adapte
  automatiquement à un fond de terminal clair ou sombre.
- **FR-008**: Pour une erreur non reconnue, le plugin MUST le dire explicitement, MUST NOT
  inventer de cause et MUST orienter vers la documentation officielle.
- **FR-009**: Le plugin MUST proposer un niveau d'explication approfondi à la demande
  (concept sous-jacent, exemple exécutable, exercice court).
- **FR-009a**: L'apprenant MUST pouvoir réafficher à tout moment l'explication complète de la
  dernière erreur (`kubectl telemach last`), même après avoir effacé son terminal. Les données
  conservées pour cela sont déjà nettoyées et masquées (FR-018).
- **FR-009b**: Une même erreur répétée dans les 10 minutes MUST être affichée en version courte,
  avec l'indication de la commande qui réaffiche l'explication complète.
- **FR-010**: Les corrections proposées MUST respecter les bonnes pratiques de sécurité (pas de
  `privileged`, pas de `hostPath` ni de `hostNetwork`, conteneur non-root, pas d'attribution de
  `cluster-admin`, pas de secret en clair) ; si la seule solution rapide en est une, elle MUST
  être signalée comme anti-pattern avec l'alternative sûre.

**Erreurs de ligne de commande**

- **FR-011**: Pour un type de ressource, une sous-commande ou une option inconnus, le plugin
  MUST suggérer la forme valide la plus proche (y compris les noms courts tels que `po`,
  `deploy`, `svc`) et afficher la commande corrigée en conservant tous les autres arguments.
- **FR-012**: La commande corrigée MUST conserver le contexte et le namespace choisis par
  l'apprenant et masquer toute valeur sensible présente dans la ligne de commande.

**Analyse des manifestes**

- **FR-013**: En cas d'erreur liée à un manifeste lu depuis un fichier ou un dossier local, le
  plugin MUST relire le ou les fichiers utilisés et localiser l'élément fautif (fichier, numéro
  de document, ligne, colonne, ressource `kind`/`metadata.name`, chemin du champ).
- **FR-014**: Le plugin MUST détecter au minimum : YAML mal formé (indentation, tabulations,
  syntaxe, séparateur de documents), `apiVersion`/`kind` absents, inconnus ou retirés, champs
  inconnus avec suggestion du champ proche ou du bon niveau, types de valeurs incorrects, champs
  obligatoires manquants pour les ressources de base (`Pod`, `Deployment`, `Service`,
  `ConfigMap`, `Secret`, `Namespace`, `Ingress`, `Job`, `CronJob`, `StatefulSet`,
  `DaemonSet`, `PersistentVolumeClaim`), et incohérence selector/labels.
- **FR-014a**: Les règles de schéma des ressources de base MUST être embarquées pour les trois
  dernières versions mineures de Kubernetes maintenues à la date de publication du plugin. La
  version utilisée MUST être celle qui correspond à la version mineure du client `kubectl`
  (FR-001b) ; si celle-ci est inconnue ou hors de cette plage, la plus récente est utilisée et
  l'explication le mentionne. La version de règles appliquée MUST être indiquée dans le
  résultat de `check`.
- **FR-015**: Lorsqu'un message de `kubectl` ne désigne qu'un chemin de champ, le plugin MUST
  le traduire en ligne et colonne du fichier source.
- **FR-016**: Le plugin MUST afficher l'extrait du fichier autour de l'erreur et la version
  corrigée proposée.
- **FR-017**: Le plugin MUST permettre de vérifier un ou plusieurs manifestes avant toute
  application (`kubectl telemach check -f …`), sans contacter le cluster ; cette vérification
  applique les contrôles de FR-014 et produit des explications au même format que FR-005 et
  FR-016. Lorsqu'aucune erreur n'est trouvée, le plugin MUST le confirmer explicitement.
- **FR-017a**: Sans `-f`, `kubectl telemach check` MUST NOT lire de fichier : il affiche en
  français un message qui explique que Kubernetes n'a pas de fichier de manifeste par défaut,
  présente l'option `-f` (fichier, dossier, `-R` pour les sous-dossiers) et donne un exemple à
  copier, puis se termine avec un code de sortie différent de 0.

**Sécurité et respect de l'utilisateur**

- **FR-018**: Le plugin MUST masquer dans tout ce qu'il affiche, conserve ou journalise les
  valeurs sensibles : champs `data`/`stringData` des objets `Secret`, jetons, mots de passe,
  certificats et clés, contenu du kubeconfig, variables d'environnement d'apparence sensible.
- **FR-019**: Le plugin MUST NOT exécuter de commande, corrective ou autre, ni modifier un
  fichier de l'apprenant ou une ressource d'un cluster : il se limite à proposer. Seules
  exceptions dans cette feature : la lecture de version de FR-001b, les lectures seules du
  cluster de `kubectl telemach diagnose` (FR-023) et la vérification différée (`watch`) de FR-028,
  bornée par FR-029.
- **FR-020**: Le plugin MUST traiter le contenu des messages d'erreur et des manifestes comme
  des données non fiables : il MUST NOT exécuter une instruction qu'ils contiennent.
- **FR-021**: L'intégration shell MUST préserver l'entrée standard, la sortie standard, la
  sortie d'erreur et le code de sortie de `kubectl`, fonctionner à l'identique sous bash et zsh,
  et ne laisser aucun fichier temporaire après la commande, y compris après une interruption.

**Diagnostic des ressources du cluster**

- **FR-022**: Le plugin MUST fournir la sous-commande `kubectl telemach diagnose`. Sans
  argument, elle repère les pods du namespace courant qui ne sont pas prêts, affiche
  l'explication complète du plus récent, puis la liste courte des autres (nom, état) avec,
  pour chacun, la commande `kubectl telemach diagnose pod/<nom>` à copier ; avec un argument (`pod/<nom>`, `deployment/<nom>` ou un nom de pod), elle
  analyse la ressource désignée et, pour un deployment, ses pods.
- **FR-023**: Les seules interactions avec le cluster autorisées sont des **lectures** d'une
  liste fermée : statut des pods (et du deployment ciblé), événements liés à ces objets, et au
  plus les 20 dernières lignes du journal courant ou précédent d'un conteneur en échec. Toute
  autre requête, et toute requête d'écriture, MUST être impossible ; cette liste MUST être
  vérifiée par un test.
- **FR-024**: Ces lectures MUST utiliser le contexte, le namespace et les droits de
  l'apprenant (y compris `--context` et `-n` passés à `kubectl telemach diagnose`), sans
  élévation de privilèges, sans demande de droit supplémentaire et sans lire le contenu
  d'aucun objet `Secret`.
- **FR-025**: Le diagnostic MUST reconnaître au minimum les états `Pending` (ressources
  insuffisantes, PVC non lié, nœud non compatible), `ImagePullBackOff`/`ErrImagePull`,
  `CrashLoopBackOff`, `CreateContainerConfigError`, `OOMKilled`, `Error` avec code de sortie,
  et les échecs de sonde (`readinessProbe`, `livenessProbe`). Il suit le format de FR-005 et
  distingue une cause liée à Kubernetes d'une cause liée à l'application (FR-004).
- **FR-026**: Quand une lecture est refusée (RBAC) ou que le cluster est injoignable, le plugin
  MUST l'expliquer comme une erreur de l'histoire 1, produire le diagnostic possible avec les
  informations restantes et le marquer comme partiel ; quand aucune source ne donne de cause
  précise, il MUST le dire sans inventer.
- **FR-027**: Le dernier diagnostic MUST être conservé (masqué) et réaffichable par
  `kubectl telemach last`, comme le prévoit FR-009a pour les erreurs de commande.
  `diagnose` ne s'exécute qu'à la demande de l'apprenant ; la seule lecture du cluster sans
  demande de l'apprenant est la vérification différée (`watch`) de FR-028.

**Vérification différée (`watch`) après une création réussie**

- **FR-028**: Après un `kubectl apply -f`, `kubectl create -f` (fichiers ou dossiers locaux),
  `kubectl create deployment` ou `kubectl run` **réussi**, hors `--dry-run`, dans une session
  interactive où l'intégration shell est active, TELEMACH MUST vérifier **une seule fois**
  l'état des pods des `Pod` et `Deployment` concernés (identifiés à partir des manifestes
  locaux ou du nom passé à la commande), et, si l'un d'eux est dans un état d'échec de FR-025,
  le signaler en une ligne indiquant le pod, son état et la commande
  `kubectl telemach diagnose pod/<nom>`. Un pod encore en cours de création n'est pas signalé.
- **FR-029**: Cette vérification MUST se limiter à la lecture du statut de ces pods (pas
  d'événements, pas de journaux), dans le contexte et le namespace de la commande d'origine,
  avec les droits de l'apprenant ; si le cluster est injoignable ou la lecture refusée, elle se
  tait.
- **FR-030**: La vérification MUST avoir lieu environ 10 secondes après la fin de la commande
  (délai nécessaire pour que les états `ImagePullBackOff`, `CrashLoopBackOff` ou
  `CreateContainerConfigError` apparaissent), en arrière-plan, sans retarder le retour de
  l'invite ; son signal MUST s'afficher à l'affichage suivant de l'invite (jamais au milieu
  d'une saisie ni de la sortie d'une autre commande), sur la sortie d'erreur, sans message de
  tâche de fond du shell, et préserver les garanties de FR-021.
- **FR-031**: La vérification MUST pouvoir être désactivée seule, sans désactiver le reste de
  TELEMACH, et `TELEMACH_DISABLE=1` MUST tout désactiver.

### Key Entities

- **Erreur kubectl capturée** : texte brut de l'erreur, commande d'origine (masquée), code de
  sortie, horodatage, version du client `kubectl` si connue, contexte et namespace explicites
  s'ils figurent dans la commande.
- **Diagnostic** : catégorie, concept Kubernetes concerné, cause probable, niveau de confiance
  (reconnue / partiellement reconnue / inconnue), localisation (fichier, document, ligne,
  colonne, ressource, champ).
- **Explication pédagogique** : résumé, explication du concept, actions correctives,
  correction proposée, avertissement anti-pattern éventuel, approfondissement disponible.
- **Manifeste analysé** : chemin, documents qu'il contient (`kind`, `metadata.name`, position
  dans le fichier), valeurs sensibles repérées (masquées).
- **Ressource diagnostiquée** : type, nom, namespace, contexte, état observé (phase, raison,
  code de sortie, nombre de redémarrages), événements récents, extrait masqué du journal,
  liste des lectures refusées.
- **Catalogue d'erreurs connues** : ensemble des motifs d'erreurs reconnus, chacun relié à un
  concept, une cause et des corrections ; extensible au fil des versions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Sur un jeu de référence d'au moins 30 erreurs `kubectl` courantes (ligne de
  commande, connexion, RBAC, ressources introuvables), au moins 90 % reçoivent une explication
  jugée correcte (bonne cause, correction valide).
- **SC-002**: Sur un jeu de référence d'au moins 20 manifestes erronés, le plugin désigne le
  bon fichier, la bonne ligne et le bon champ dans au moins 90 % des cas.
- **SC-003**: L'explication s'affiche en moins de 2 secondes après l'erreur dans 95 % des cas.
- **SC-004**: Lors de tests avec des débutants, au moins 80 % corrigent leur erreur au premier
  essai après lecture de l'explication, sans autre ressource.
- **SC-005**: 100 % des explications sont rédigées en français et 0 erreur non reconnue ne
  reçoit de cause inventée (vérifié sur le jeu de référence).
- **SC-006**: 0 valeur sensible présente dans les manifestes, commandes ou erreurs de test
  n'apparaît en clair dans la sortie ou les données conservées du plugin.
- **SC-007**: 100 % des explications affichées dans un terminal sont colorées, et 100 % restent
  complètes et compréhensibles avec la couleur désactivée.
- **SC-008**: 0 commande n'est exécutée, 0 fichier de l'apprenant n'est modifié et 0 ressource
  de cluster n'est créée, modifiée ou supprimée par le plugin ; les seules interactions sont la
  lecture de version de FR-001b, les lectures de FR-023 et celles de FR-029 (état du cluster de
  test identique avant et après chaque diagnostic ou vérification).
- **SC-010**: Sur un cluster jetable, pour 100 % des pods créés par un `apply` réussi qui sont
  en échec au moment de la vérification, le signal de FR-028 est affiché à l'invite suivante ;
  pour 100 % des pods sains ou en cours de création, rien n'est affiché ; le retour de l'invite
  n'est pas retardé de plus de 50 ms au p95.
- **SC-009**: Sur un cluster jetable comportant au moins un pod volontairement en erreur pour
  chaque état de FR-025, le plugin nomme l'état et une cause correcte dans au moins 90 % des
  cas, en moins de 5 secondes chacun.

## Assumptions

- Le public cible est un apprenant débutant à intermédiaire, francophone, qui utilise
  Kubernetes en ligne de commande avec `kubectl` ; les messages d'origine sont en anglais.
- Le plugin s'utilise comme un plugin `kubectl` standard (`kubectl telemach …`), complété
  par une intégration shell qui capte les erreurs, sur le modèle du projet TELEMACH PLUGIN
  DOCKER.
- La couverture initiale vise les versions stables de `kubectl` et de Kubernetes encore
  maintenues au moment de la publication ; les règles de schéma embarquées correspondent à ces
  versions (FR-014a), et un décalage est signalé plutôt qu'ignoré. Le cluster n'est jamais
  interrogé pour connaître sa version.
- Kustomize (`-k`, `kubectl kustomize`), Helm et les opérateurs tiers sont hors périmètre ; une
  erreur qui en provient est traitée comme « erreur non reconnue » ou expliquée à partir du seul
  message.
- La capture automatique (FR-001) suppose que `kubectl` est exécuté dans un shell bash ou zsh
  que le plugin peut observer ; le collage manuel d'erreurs est hors périmètre.
- Les commandes `kubectl` **réussies** ne déclenchent aucune explication ; seules les
  commandes de création de FR-028 déclenchent la vérification différée (`watch`), en lecture seule,
  de l'état des pods concernés.
- Le diagnostic couvre les pods et les deployments ; les autres contrôleurs (StatefulSet,
  DaemonSet, Job), les services sans endpoints et les problèmes d'ingress ou de réseau sont
  hors périmètre de cette feature.
- Le plugin ne nécessite pas d'accès réseau pour expliquer les erreurs reconnues ni pour
  vérifier un manifeste ; seuls `diagnose` et la vérification de FR-028 contactent le
  cluster ; les liens vers la documentation officielle sont fournis à titre de
  ressources.
- Ce dépôt ne contient pas encore de `LESSON-LEARNED.md`. Les leçons transposables du projet
  Docker ont été prises en compte : options globales avant la sous-commande (LL-014),
  comportement identique bash/zsh (LL-002, LL-006, LL-009), fichier temporaire nettoyé après
  interruption (LL-003), aucun test agissant sur les ressources de l'utilisateur (LL-015).
