# Recherche (Phase 0) : Diagnostiquer ce qui réussit sans marcher

**Feature** : `002-silent-pitfalls` | **Date** : 2026-10-17

Décisions techniques de la feature 002. Les décisions R1 à R14 de la
[feature 001](../001-kubectl-error-guidance/research.md) restent valables ; seules les
extensions et les écarts sont décrits ici (numérotés S1 à S14 pour éviter toute confusion).
`LESSON-LEARNED.md` relu le 2026-10-17 : LL-001 à LL-004 et les leçons héritées D-LL-xxx sont
prises en compte (voir S14).

## Sources de la recherche sur les difficultés des utilisateurs

Recherche du 2026-10-17 (guides de dépannage, cours, forums). Les classements viennent surtout
de blogs d'éditeurs et de formations, pas d'enquêtes chiffrées : chaque piège est donc
**confirmé sur kind** avant d'entrer dans le catalogue (FR-029, S12).

- Service sans endpoints : [oneuptime](https://oneuptime.com/blog/post/2026-02-09-service-endpoints-not-populating/view),
  [Blue Matador](https://www.bluematador.com/docs/troubleshooting/kubernetes-service).
- Ports : [iximiuz Kubelings](https://labs.iximiuz.com/courses/kubelings-dbd840c8/module-04/broken-targetport),
  [KodeKloud « Port Mania »](https://notes.kodekloud.com/docs/Kubernetes-Troubleshooting-for-Application-Developers/Troubleshooting-Scenarios/Port-Mania/page.md).
- Ingress : [oneuptime](https://oneuptime.com/blog/post/2026-02-09-troubleshoot-ingress-not-forwarding/view),
  [devopsboys](https://devopsboys.com/today-i-fixed/kubernetes-ingress-404-all-paths-fix).
- ConfigMap : [KodeKloud « Config Out of Date »](https://notes.kodekloud.com/docs/Kubernetes-Troubleshooting-for-Application-Developers/Troubleshooting-Scenarios/Config-Out-of-Date/page.md),
  [ITNEXT](https://itnext.io/the-challenge-of-configmap-rollouts-in-kubernetes-1431398c0866).
- Tag mutable : [DEV](https://dev.to/oleksandr_kuryzhev_42873f/kubectl-apply-not-updating-pods-fix-the-stale-image-pitfall-592b).
- Rollout : [Netdata](https://www.netdata.cloud/guides/kubernetes/kubernetes-deployment-rollout-stuck/),
  [freeCodeCamp](https://www.freecodecamp.org/news/how-to-debug-a-stuck-kubernetes-rollout-with-a-hands-on-lab/).
- PVC : [Netdata](https://www.netdata.cloud/guides/kubernetes/kubernetes-pvc-stuck-pending/).
- `Terminating` et suppression forcée : [Netdata](https://www.netdata.cloud/guides/kubernetes/kubernetes-pod-stuck-terminating/).
- DNS entre namespaces : [documentation Kubernetes](https://www.kubernetes.io/docs/tasks/administer-cluster/dns-debugging-resolution/).
- Endpoints déprécié : [blog Kubernetes](https://kubernetes.io/blog/2025/04/24/endpoints-deprecation/),
  [KEP-4974](https://www.kubernetes.dev/resources/keps/4974/).
- ingress-nginx retiré : [blog Kubernetes](https://kubernetes.io/blog/2025/11/11/ingress-Nginx-retirement/).

## S1. Lectures ajoutées à la liste fermée (FR-017, FR-018, FR-019)

- **Decision** : `internal/kuberead` reste le **seul** point d'accès au cluster ; il gagne
  exactement les lectures ci-dessous, toutes en `get … -o json` (sauf la ConfigMap, S3), avec
  la portée de l'apprenant (`--context`, `-n`, `--kubeconfig`, `--as`) et
  `--request-timeout=10s` :

  | Lecture | Commande | Usage |
  |---------|----------|-------|
  | Service | `get service <nom>` | US1, US3 (backend d'Ingress), US4 |
  | Services du namespace | `get services` | `diagnose` sans argument |
  | EndpointSlices d'un Service | `get endpointslices -l kubernetes.io/service-name=<nom>` | US1 |
  | EndpointSlices du namespace | `get endpointslices` | `diagnose` sans argument |
  | Deployments du namespace | `get deployments` | `diagnose` sans argument |
  | ReplicaSets d'un Deployment | `get replicasets -l <selector>` | US2 |
  | Ingress | `get ingress <nom>`, `get ingresses` | US3, sans argument |
  | Classes d'Ingress | `get ingressclasses` | US3, US4 |
  | PVC | `get pvc <nom>`, `get pvc` | US3, sans argument |
  | Classes de stockage | `get storageclasses` | US3, US4 |
  | Dates d'une ConfigMap | voir S3 | US2 |

  Lectures existantes réutilisées : `get pod`, `get pods [-l]`, `get deployment <nom>` (type
  étendu), `get events`, `logs --tail=20`.
- **Jamais** : `get secret(s)`, `describe` (qui afficherait des valeurs), `exec`,
  `port-forward`, ni aucun verbe d'écriture. `allowlist_test.go` est étendu : il énumère les
  appels possibles et échoue sur tout autre verbe ou ressource.
- **Rationale** : chaque lecture correspond à un maillon d'un diagnostic de la spec ;
  EndpointSlice plutôt qu'Endpoints, déprécié depuis Kubernetes 1.33 (FR-032).
- **Alternatives considered** : `kubectl describe` (texte non stable, valeurs affichées) ;
  `client-go` (écarté par la feature 001, R8).

## S2. Types décodés et annotations (FR-018)

- **Decision** : chaque objet est décodé dans un type dédié qui ne contient **aucun** champ de
  valeur sensible : ni `data`/`binaryData`, ni `env[].value`, ni l'ensemble des annotations.
  Les annotations sont décodées par un type qui ne garde qu'une **liste fermée de clés
  publiques**, nécessaires au diagnostic et sans donnée de l'apprenant :
  - `ingressclass.kubernetes.io/is-default-class` (classe d'Ingress par défaut) ;
  - `storageclass.kubernetes.io/is-default-class` (classe de stockage par défaut) ;
  - `kubernetes.io/ingress.class` (ancienne façon de choisir la classe d'un Ingress) ;
  - `deployment.kubernetes.io/revision` (numéro de version d'un ReplicaSet).

  Toute autre clé est ignorée dès le décodage (test : une annotation inventée contenant un
  faux secret n'apparaît ni dans l'instantané, ni dans le rendu, ni dans `last.json`).
- **Conséquence sur la spec** : FR-018 est précisé en ce sens (exception limitée à ces quatre
  clés).
- **Rationale** : la classe par défaut et la révision ne sont publiées **que** par ces
  annotations ; s'en passer rendrait US2 et US3 imprécis.
- **Alternatives considered** : ne lire aucune annotation (diagnostic de la classe par défaut
  impossible) ; décoder toutes les annotations puis filtrer (une erreur de filtre suffirait à
  fuiter `last-applied-configuration`).

## S3. Date de modification d'une ConfigMap (FR-010, clarification du 2026-10-17)

- **Decision** : la date de dernière modification est la plus récente des dates
  `metadata.managedFields[].time`. Elle est lue par
  `get configmap <nom> --show-managed-fields -o jsonpath='{range .metadata.managedFields[*]}{.time}{"\n"}{end}'` :
  kubectl n'écrit sur sa sortie **que des dates**, aucune valeur de la ConfigMap n'entre dans
  le processus du plugin. Elle est comparée à la date de démarrage
  (`status.startTime`) du plus ancien pod en service qui référence la ConfigMap par `env`,
  `envFrom` ou un montage `subPath`. Un montage de volume sans `subPath` est mis à jour par le
  kubelet : il n'est pas signalé, et la leçon l'explique.
- **À vérifier en premier** (tâche d'amorçage, sur kind 1.35, 1.36, 1.37) : que
  `--show-managed-fields` s'applique bien à la sortie `jsonpath` (la documentation ne cite que
  JSON et YAML) et que la date change après `kubectl apply` et `kubectl edit`.
  **Repli** si ce n'est pas le cas : `-o json --show-managed-fields` décodé dans un type qui ne
  contient que `metadata.name` et `metadata.managedFields[].time` (même principe que les
  valeurs d'`env` de la feature 001).
- **Secrets** : aucune lecture ; la leçon est donnée sous forme d'information quand un pod
  référence un Secret par `env`/`envFrom` (US2, scénario 6).
- **Alternatives considered** : `resourceVersion` (pas une date, non comparable) ; annotation
  `last-applied-configuration` (contient les valeurs) ; hachage du contenu (lecture des valeurs).

## S4. Chemin du trafic d'un Service (FR-001 à FR-005)

- **Decision** : diagnostic en maillons ordonnés, chacun `ok`, `broken`, `unknown` (lecture
  refusée) ou `skipped` :
  1. **type** : `ExternalName` ou sans selector → cas particulier expliqué (FR-004) ;
  2. **selector** : pods du namespace qui le satisfont (`get pods -l`) ; aucun → `broken`, avec
     au plus 3 groupes de labels proches (distance d'édition sur les paires `clé=valeur` des
     pods du namespace, `internal/textdist`) ;
  3. **readiness** : pods désignés non Ready → `broken`, avec renvoi à `diagnose pod/<nom>` ;
  4. **endpoints** : EndpointSlices du Service ; cohérence avec les maillons 2 et 3 (un écart
     est signalé tel quel, sans cause inventée) ;
  5. **ports** : pour chaque port du Service, `targetPort` nommé absent de tous les
     `containerPort` nommés des pods désignés → `broken` ; numérique absent des ports
     déclarés → `broken` avec mention « déclaration facultative » ; aucun port déclaré →
     `unknown` avec la démarche de vérification (aucun `exec`, FR-019).
  Le premier maillon `broken` porte l'explication complète ; les autres sont listés.
- **Rationale** : c'est l'ordre des guides de dépannage et celui d'un formateur ; il enseigne
  le modèle mental labels → selectors → endpoints.
- **Alternatives considered** : tester la connexion (impossible sans `exec` ou
  `port-forward`, interdits).

## S5. État d'un rollout (FR-006 à FR-009)

- **Decision** : à partir du Deployment (`status.conditions`, `observedGeneration`,
  `updatedReplicas`, `readyReplicas`, `availableReplicas`, `spec.paused`,
  `spec.progressDeadlineSeconds`) et de ses ReplicaSets (sélectionnés par le selector du
  Deployment, propriétaire vérifié, version par l'annotation de révision, S2) :
  - `complete` : condition `Progressing` de raison `NewReplicaSetAvailable` et tous les pods à
    jour prêts ;
  - `progressing` : échéance non atteinte ;
  - `stalled` : condition `Progressing=False`, raison `ProgressDeadlineExceeded` ;
  - `paused` : `spec.paused` (échéance non évaluée) ;
  - pour `stalled` ou `progressing` avec des pods en échec, le pod en difficulté le plus récent
    du **nouveau** ReplicaSet (label `pod-template-hash`) est diagnostiqué par
    `workload.Diagnose` (feature 001).
- **Tag mutable (FR-009)** : une référence d'image sans tag ni digest, ou au tag `latest`,
  produit une information ; une référence `@sha256:` n'est jamais signalée. L'analyse de la
  référence (registre, chemin, tag, digest) suit la grammaire des références d'image OCI.
- **Alternatives considered** : `kubectl rollout status` (bloquant, et c'est une commande de
  suivi, pas une lecture d'état ponctuelle).

## S6. Ingress (FR-011, FR-012)

- **Decision** : classe effective = `spec.ingressClassName`, sinon annotation
  `kubernetes.io/ingress.class` (dépréciée, signalée), sinon la classe marquée par défaut.
  Aucune classe dans le cluster → « aucun contrôleur d'Ingress installé » (FR-012). Classe
  absente de la liste → nom à corriger. `status.loadBalancer.ingress` vide → « pas encore
  traité par le contrôleur ». Pour chaque règle (au plus 10 backends lus), le Service visé est
  lu (`get service`) et son port comparé au port demandé (numéro ou nom).
- **Cluster de test** : aucun contrôleur n'est installé. Les pièges visés (pas de classe,
  classe inconnue, Service absent, pas d'adresse) n'en ont pas besoin, et ingress-nginx est
  retiré depuis mars 2026. Le scénario « classe existante mais non par défaut » crée seulement
  un objet IngressClass dans le cluster jetable.
- **Alternatives considered** : installer un contrôleur dans kind (plus lent, dépendance
  externe, sans apport pour les pièges visés) ; Gateway API (hors périmètre de cette feature,
  à envisager plus tard).

## S7. PVC (FR-013, FR-014)

- **Decision** : `spec.storageClassName` absent → classe par défaut (annotation, S2) ; chaîne
  vide → provisionnement dynamique désactivé (expliqué) ; nom absent des StorageClasses →
  classe à corriger. Mode de liaison lu sur la classe : `WaitForFirstConsumer` sans pod
  consommateur (pods du namespace dont `volumes[].persistentVolumeClaim.claimName` désigne le
  PVC) → état **normal** ; avec un pod consommateur `Pending` → diagnostic de ce pod
  (feature 001).
- **Précision (2026-10-19)** : le pod consommateur `Pending` est diagnostiqué par la même
  fonction que le pod en difficulté d'un rollout (`ObjectSnapshot.Failing`), sans nouvel
  identifiant de piège : si le PVC lui-même n'a aucun piège, la leçon du pod devient la leçon
  principale (« le PVC attend le pod X, qui ne démarre pas »). Relevé réel :
  `testdata/objects/pvc-consumer-pending/` (nodeSelector impossible, `NodeSelectorMismatch`).
  Alternative écartée : un piège `pvc.consumer-pending` qui renvoie à `diagnose pod/<nom>`
  (identifiant de plus, et une deuxième commande pour l'apprenant).
- **Alternatives considered** : lire les PersistentVolumes (objets du cluster, rarement
  lisibles par un apprenant, inutiles au provisionnement dynamique de kind).

## S8. `diagnose` sans argument et noms ambigus (FR-015, FR-016)

- **Decision** : sans argument, les lectures du namespace (pods, Services, EndpointSlices,
  Deployments, Ingress, PVC, classes) sont lancées **en parallèle**, chacune dans son propre
  processus kubectl, pour tenir les 5 s de SC-004. Les pods en échec sont expliqués comme dans
  la feature 001 ; les pièges des autres objets sont listés ensuite (au plus 10 au total).
  Un nom sans type est cherché parmi les pods, Services et Deployments : un seul résultat →
  diagnostic direct (compatible avec la feature 001 pour un pod) ; plusieurs → liste à copier.
- **Alternatives considered** : diagnostic séquentiel (dépasse 5 s avec sept lectures) ;
  chercher aussi parmi Ingress et PVC (trois lectures de plus pour un cas rare : le type reste
  demandé pour eux).

## S9. Vérification différée étendue (FR-022 à FR-025)

- **Decision** :
  - **cibles**, toujours sans lecture : objets `Service`, `Ingress`,
    `PersistentVolumeClaim` des manifestes locaux appliqués, Service créé par
    `create service …` ou par **`expose`** (nom de `--name`, sinon celui de la ressource
    exposée) ; `expose` est ajouté aux commandes qui lancent la vérification (spec FR-022
    précisée en ce sens : c'est la façon la plus courante pour un débutant de créer un
    Service) ;
  - **détections non transitoires seulement** (FR-024) : Service dont le selector ne désigne
    **aucun** pod du namespace, quel que soit son état, **sauf** si la même commande applique
    un Pod ou un Deployment dont les labels satisfont ce selector (les pods peuvent ne pas
    exister encore) ; `targetPort` nommé absent des pods désignés existants ; Ingress sans
    classe effective ; PVC dont la classe n'existe pas ;
  - **tag mutable** : décidé depuis le seul fichier local (FR-023), mémorisé dans
    `$XDG_STATE_HOME/telemach-kubectl/hints.json` (`0600`, empreinte de l'image seulement),
    au plus une ligne par image et par 24 heures ;
  - **budget** : au plus 12 lectures par vérification ; au-delà, rien n'est signalé pour les
    objets restants (pas de signal sur information incomplète).
- **Rationale** : l'échéance de rollout (600 s) dépasse la fenêtre de 10 s : le rollout bloqué
  relève de `diagnose` et de l'erreur de `rollout status`, pas de la vérification différée.

## S10. Capture de stderr des commandes interactives (US5, FR-026)

- **Constat** : la feature 001 (R3) ne capte pas stderr pour `exec -it`, `attach`,
  `port-forward`… ; or les erreurs visées (`unable to upgrade connection: container not
  found`, échecs de `port-forward`) viennent justement de ces commandes. Avec `-t`, la sortie
  d'erreur du programme distant passe par stdout ; le stderr local ne porte que les messages
  de kubectl lui-même.
- **Decision** : capter stderr (même mécanisme `tee`, stdin et stdout intacts) aussi pour
  `exec`, `attach`, `debug` et `port-forward`, **après validation par les tests en
  pseudo-terminal** (bash et zsh) : session interactive utilisable, aucun écho dupliqué
  (D-LL-002), Ctrl-C transmis. Restent sans capture : `edit`, `proxy`, `logs -f`, `get -w`.
  Une commande interactive dont stderr ne correspond à aucune entrée garde l'explication par
  le seul code de sortie (le code peut venir du programme distant). Après une interruption
  (code 130) d'un `port-forward`, une explication n'est donnée que si stderr contient une
  erreur de transfert reconnue (`an error occurred forwarding … connection refused`).
- **Repli** si un test en pseudo-terminal échoue : comportement de la feature 001 conservé
  pour la commande concernée, et l'entrée du catalogue est reconnue sur les seules formes non
  interactives (`exec` sans `-t`, `logs -c`).
- **Alternatives considered** : relire le pod pour vérifier le nom du conteneur (impossible :
  `explain` ne contacte jamais le cluster).

## S11. Entrées du catalogue (FR-026 à FR-028, FR-032)

- **Decision** : nouvelles entrées dans `internal/catalog/data/` :
  `notfound.container-exec` (`unable to upgrade connection: container not found`,
  `container … is not valid for pod`), `cmdline.container-required` (`a container name must
  be specified for pod … choose one of: [...]`, avec une correction par conteneur cité),
  `portforward.local-port-in-use`, `portforward.pod-not-running`,
  `portforward.connection-refused`, `rollout.progress-deadline`, `conflict.field-immutable`,
  motif de journal `runtime.dns-short-name` (« could not resolve host », « no such host »,
  « Name or service not known » sur un nom sans point) ; leçons de pièges
  `service.*`, `rollout.*`, `ingress.*`, `pvc.*`, `config.*`, `image.mutable-tag`.
  L'anti-pattern `Terminating` (FR-028) est porté par une note réutilisable
  (`anti_pattern`) attachée aux entrées où une suppression forcée peut être tentée.
  Les commandes `kubectl get endpoints` existantes sont remplacées par
  `kubectl get endpointslices -l kubernetes.io/service-name=<nom>` (FR-032).
- **Messages exacts** : relevés sur kind (S12) avant d'écrire les motifs ; aucun motif n'est
  écrit d'après un message supposé (principe « ne jamais inventer »).

## S12. Corpus réel relevé sur kind (FR-029)

- **Decision** :
  - `scripts/collect-errors` reçoit les nouveaux cas d'erreur (exec et logs avec un mauvais
    conteneur, port-forward, rollout status, champ immuable) ;
  - un nouvel outil `scripts/collect-objects` relève, pour chaque piège, les objets JSON
    réels (Service, EndpointSlices, pods, Deployment, ReplicaSets, Ingress, classes, PVC, dates
    de ConfigMap), masqués, dans `testdata/objects/<cas>/` avec un `expected.yaml` rédigé à la
    main (même principe que `testdata/pods/`) ;
  - un workflow `corpus.yml` (`workflow_dispatch`, et `pull_request` limité aux outils et
    scénarios du relevé, car un `workflow_dispatch` n'est lançable qu'une fois sur `main` ;
    matrice des trois images kind,
    `permissions: contents: read`, actions épinglées par SHA) lance les deux outils et publie
    le résultat comme artefact. Le mainteneur relit l'artefact (déjà masqué) avant de le
    versionner. Ce workflow ne s'exécute que sur un cluster kind créé par lui-même (D-LL-015)
    et rend le relevé reproductible, y compris depuis une machine sans Docker.
- **Alternatives considered** : relevé local uniquement (Docker non disponible sur toutes les
  machines du mainteneur, par exemple WSL2 sans intégration Docker Desktop) ; objets écrits à
  la main (contraire à FR-029).

## S13. Rendu (FR-021)

- **Decision** : nouveau rendu `render.RenderObject` pour un instantané d'objet : en-tête
  (type, nom, namespace, contexte), maillons du chemin (✔ sain, ✖ défaillant, ? non lu),
  puis la leçon au format existant (résumé, concept, cause, corrections, anti-pattern) et le
  renvoi à `deepen`. Information (ℹ) pour les cas non bloquants (tag mutable, Secret en
  variable d'environnement, PVC en attente normale). Golden files `testdata/golden/f002/` en
  couleur et sans couleur, contrôlés par `TestGoldenFrench` et `TestGoldenNoSecret`
  (FR-031).

## S14. Leçons appliquées (principe V)

- **LL-004** : chaque scénario W1… du quickstart a son test d'intégration, avec la commande
  exacte écrite dans le guide ; aucun dossier de `testdata/` qui mêle objets et attentes n'est
  donné à `apply` ou `check`.
- **LL-003 et LL-001** : les nouveaux objets passent par `redact` ; toute nouvelle règle de
  masquage reçoit un cas de fuzzing ; `TestCorpusLanguageAndSecrets` est étendu au corpus
  `testdata/objects/`.
- **LL-002** : `scripts/secrets` et relecture de `gh pr checks` après chaque push.
- **D-LL-012** : SC-004 (< 5 s) et SC-007 (< 50 ms) ont chacun un test de mesure.
- **D-LL-014** : les messages et formes d'appel sont relevés sur le vrai kubectl (S12).
- **D-LL-015** : tests et relevés uniquement sur un cluster kind créé pour l'occasion.
- **D-LL-001, D-LL-007, D-LL-008, D-LL-009** : inchangées (outils compilés avec Go 1.27.2,
  étiquettes de build déclarées, validation enchaînée par `&&`, contrôles qui échouent s'ils
  n'ont rien comparé).

## Versions

Aucune nouvelle dépendance Go. Outils et images inchangés par rapport à la feature 001
(revérifiées le 2026-10-17, T001 : Go 1.27.2, kind v0.33.0, `kindest/node` 1.35.8, 1.36.4 et 1.37.0 épinglées par digest,
kubectl 1.35.9, 1.36.5 et 1.37.1) ; à revérifier au moment de l'implémentation, conformément
aux instructions globales du mainteneur (dernière version stable réelle, correctifs de sécurité
compris).
