---

description: "Liste des tâches pour la feature 002-silent-pitfalls"
---

# Tasks: Diagnostiquer ce qui réussit sans marcher

**Input**: Design documents from `/specs/002-silent-pitfalls/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli.md, quickstart.md

**Tests**: inclus. FR-029 à FR-031 et les critères SC-001 à SC-007 exigent des corpus réels,
des mesures et le rejeu de chaque scénario du quickstart (LL-004) ; le principe IV exige des
tests bloquants en CI. Dans chaque histoire, les tests sont écrits **avant** l'implémentation
et doivent d'abord échouer.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 à US5 de spec.md)
- Include exact file paths in descriptions

## Conventions du projet (à respecter dans chaque tâche)

- Conventions de la feature 001 inchangées : code et identifiants en anglais, commentaires,
  messages, catalogue et documentation en français avec accents (principe II) ; toute chaîne
  affichée ou écrite passe par `sanitize` puis `redact` ; relire `LESSON-LEARNED.md` avant
  chaque phase (principe V).
- **Aucun test ne touche le contexte kubectl courant** : cluster kind jetable uniquement
  (D-LL-015).
- **Toute lecture du cluster passe par `internal/kuberead`** et figure dans la liste fermée de
  research.md S1 ; jamais d'objet Secret, de valeur de ConfigMap (`data`, `binaryData`), de
  valeur d'`env` ni d'annotation hors des quatre clés de research.md S2.
- **Aucun message ou état n'est supposé** : motifs et attentes viennent du corpus relevé sur
  kind (research.md S12, D-LL-014).
- `testdata/objects/scenarios/` ne contient **que** des manifestes ; les attentes vont dans
  `testdata/objects/<cas>/expected.yaml` (LL-004).
- Validation et commit enchaînés par `&&` (D-LL-008) ; après chaque push, relire
  `gh pr checks` (LL-002) ; `scripts/secrets` avant chaque commit.
- Les commandes proposées n'utilisent jamais `kubectl get endpoints` (FR-032).

---

## Phase 1: Setup (corpus et outillage)

**Purpose**: rendre le relevé du corpus réel possible et reproductible avant toute logique de
diagnostic.

- [X] T001 Revérifier les dernières versions stables réelles de Go, kind, des images `kindest/node` 1.35, 1.36, 1.37 (digests) et de kubectl 1.35, 1.36, 1.37, et mettre à jour `go.mod` (`toolchain`), `.github/workflows/ci.yml` et `specs/002-silent-pitfalls/research.md` § Versions si une version de correctif est sortie depuis le 2026-10-17
- [X] T002 [P] Créer les manifestes des scénarios dans `testdata/objects/scenarios/` (manifestes seulement, LL-004), un fichier par scénario du quickstart : `selector-typo.yaml`, `not-ready.yaml`, `targetport-name.yaml`, `targetport-undeclared.yaml`, `no-selector.yaml`, `healthy.yaml` (Deployment `hello` dont le conteneur déclare un port nommé `http`, et son Service), `mutable-tag.yaml` (image sans tag), `config-env.yaml` (ConfigMap `conf-settings` et Secret de test en `env`/`envFrom`, faux secret du corpus), `rollout-stalled.yaml` (`progressDeadlineSeconds: 30`), `ingress-no-class.yaml`, `ingress-unknown-class.yaml`, `ingress-backend-missing.yaml`, `ingressclass-not-default.yaml`, `pvc-unknown-class.yaml`, `pvc-waiting.yaml`, `pvc-empty-class.yaml`, `watch-selector.yaml`, `watch-selector-2.yaml`, `watch-together.yaml`, `two-containers.yaml`, `immutable-selector.yaml` (même Deployment que `healthy.yaml`, selector modifié), `dns-short-name.yaml` (pod qui appelle le Service `kubernetes` du namespace `default` par son nom court et s'arrête en erreur) ; le Deployment `stalled` de W12 est celui de `rollout-stalled.yaml` ; non-root, `resources` déclarées, images avec tag explicite vérifié sur le registre (D-LL-004) sauf `mutable-tag.yaml`
- [X] T003 [P] Étendre `tests/manifests/lint_test.go` à `testdata/objects/scenarios/` : règles de sécurité de la feature 001, défaut volontaire de chaque scénario déclaré dans `testdata/objects/scenarios.expected.yaml` (hors du dossier, LL-005) seul toléré ; `TestScenarioDirsOnlyManifests` vérifie que chaque fichier du dossier est un objet Kubernetes ; l'image sans tag de `mutable-tag.yaml` reste un avertissement non bloquant (principe III)
- [X] T004 [P] Ajouter les nouveaux cas d'erreur à `scripts/collect-errors/main.go` : `exec-container-not-found` (`exec -it deploy/hello -c helo -- sh`, et la forme sans `-t`), `logs-container-required` (pod `two-containers`), `portforward-local-port-in-use`, `portforward-pod-not-running`, `portforward-connection-refused`, `rollout-status-deadline` (Deployment `stalled`), `apply-field-immutable` (`immutable-selector.yaml`)
- [X] T005 Créer `scripts/collect-objects/main.go` (étiquette `integration`, sur le modèle de `scripts/collect-errors`) : pour chaque scénario de T002, appliquer le manifeste dans un namespace jetable, attendre l'état visé, puis relever **avec les seules lectures de research.md S1** les objets JSON (Service, EndpointSlices, pods, Deployment, ReplicaSets, Ingress, IngressClasses, PVC, StorageClasses, dates de ConfigMap) masqués par `redact`, dans `testdata/objects/<cas>/` ; ne jamais écraser `expected.yaml` ; option `-variant 1.35|1.36` comme `collect-errors` (dépend de T002)
- [X] T006 Vérifier, dans `scripts/collect-objects/main.go`, la lecture des dates d'une ConfigMap (research.md S3) : `get configmap <nom> --show-managed-fields -o jsonpath='{range .metadata.managedFields[*]}{.time}{"\n"}{end}'` renvoie au moins une date, aucune valeur, et une date plus récente après `kubectl apply` puis après `kubectl edit` ; consigner le résultat par version dans `testdata/objects/configmap-dates/result.txt` ; si une version échoue, appliquer le repli de S3 et le noter dans `specs/002-silent-pitfalls/research.md` (dépend de T005)
- [X] T007 Créer `.github/workflows/corpus.yml` : `workflow_dispatch` seulement, matrice des trois images kind et kubectl de `ci.yml`, `permissions: contents: read`, actions épinglées par SHA de commit complet (versions vérifiées au moment de l'ajout), installation vérifiée de kind et kubectl comme dans `ci.yml`, exécution de `scripts/collect-errors` et `scripts/collect-objects`, publication de `testdata/errors/` et `testdata/objects/` en artefact ; aucun secret, aucun autre cluster que celui créé par le job (D-LL-015) (dépend de T004, T005)
- [X] T008 Lancer `corpus.yml` sur la branche (`gh workflow run corpus.yml --ref 002-silent-pitfalls`), télécharger l'artefact, le relire en entier (aucune valeur du corpus de secrets ni de `conf-settings`, `scripts/secrets` vert), rédiger chaque `testdata/objects/<cas>/expected.yaml` et `testdata/errors/<cas>/expected.yaml` à la main, puis versionner (dépend de T006, T007)

**Checkpoint**: corpus réel des trois versions disponible ; comportement de la lecture des dates
de ConfigMap connu.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: lectures, types, instantanés, rendu et routage CLI communs à toutes les histoires.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Tests for Foundational ⚠️

- [X] T009 [P] Étendre `internal/kuberead/allowlist_test.go` : chaque nouvelle fonction de lecture produit exactement la commande de research.md S1 (verbe `get`, ressource, `-l` éventuel, `-o json` ou le `jsonpath` des dates), avec la portée de l'apprenant et `--request-timeout=10s` ; `TestNoOtherReads` échoue pour `secret`, `secrets`, `describe`, `exec`, `port-forward` et tout verbe d'écriture
- [X] T010 [P] Écrire `internal/kuberead/annotations_test.go` : `KnownAnnotations` ne garde que `ingressclass.kubernetes.io/is-default-class`, `storageclass.kubernetes.io/is-default-class`, `kubernetes.io/ingress.class`, `deployment.kubernetes.io/revision` ; une annotation `kubectl.kubernetes.io/last-applied-configuration` ou inventée contenant `s3cret-fake` n'apparaît dans aucun type décodé
- [X] T011 [P] Écrire `internal/kuberead/objects_test.go` : décodage des JSON réels de `testdata/objects/*/` ; `ConfigMapDates` ne contient que nom, namespace et dates ; aucun champ `data`, `binaryData`, `env[].value` dans les types (test par réflexion sur les champs, comme `TestEnvValuesNeverDecoded`)
- [X] T012 [P] Écrire `internal/pitfall/snapshot_test.go` : règle « un maillon `unknown` ne devient jamais `broken` » (data-model.md § Link) ; le premier maillon `broken` porte la leçon ; un `Pitfall.ID` inconnu du catalogue fait échouer le test de cohérence
- [X] T013 [P] Écrire `internal/state/hints_test.go` : `hints.json` en `0600`, écriture atomique, empreinte SHA-256 de la référence d'image seulement, entrées de plus de 24 h purgées, fichier illisible ignoré sans erreur
- [X] T014 [P] Écrire `cmd/kubectl-telemach/diagnose_route_test.go` (faux kubectl de `diagnose_test.go` étendu aux nouvelles lectures) : `service/`, `svc/`, `deployment/`, `deploy/`, `ingress/`, `ing/`, `pvc/`, `persistentvolumeclaim/` routés ; type inconnu → liste des types, code `2` ; `pod/` inchangé

### Implementation for Foundational

- [X] T015 Implémenter `internal/kuberead/annotations.go` (`KnownAnnotations`, quatre clés, research.md S2) et `internal/kuberead/objects.go` (types de data-model.md § Types lus, champs listés seulement), étendre `Pod` (`status.startTime`, `volumes`, `volumeMounts[].subPath`, `ports`) et `Deployment` dans `internal/kuberead/types.go`. Faire passer T010, T011
- [X] T016 Ajouter à `internal/kuberead/read.go` les lectures de research.md S1 et elles seules (`GetService`, `ListServices`, `ServiceEndpointSlices`, `ListEndpointSlices`, `ListDeployments`, `ReplicaSetsBySelector`, `GetIngress`, `ListIngresses`, `ListIngressClasses`, `GetPVC`, `ListPVCs`, `ListStorageClasses`, `ConfigMapDates`), avec la commande retenue par T006. Faire passer T009 (dépend de T006, T015)
- [X] T017 Créer `internal/pitfall/doc.go` et `internal/pitfall/snapshot.go` : `ObjectSnapshot`, `Link` (`Step` et `State` `ok` \| `broken` \| `unknown` \| `skipped`), `Pitfall` (`Severity` `warning` \| `info`, `Transient`), `RolloutState` et `ObjectSummary` selon data-model.md ; `Reader` (interface des lectures, pour le faux de test) ; fonction de cohérence avec le catalogue. Faire passer T012 (dépend de T015)
- [X] T018 [P] Étendre `internal/catalog/schema.go` et `internal/catalog/loader.go` : champ `pitfalls` (identifiants de `Pitfall.ID` couverts par une entrée), validation qu'un identifiant n'est couvert qu'une fois, que chaque entrée de piège a concept, cause, ≥ 1 correction et approfondissement, et qu'aucune commande du catalogue ne contient `get endpoints` (FR-032)
- [X] T019 [P] Implémenter `internal/state/hints.go` (`HintMemory`, `$XDG_STATE_HOME/telemach-kubectl/hints.json`) et ajouter `Object` (instantané encodé) et la source `object` à `LastDiagnosis` dans `internal/state/state.go`. Faire passer T013
- [X] T020 Implémenter `internal/render/object.go` : `RenderObject` (en-tête type, nom, namespace, contexte ; maillons ✔ ✖ · ? ; leçon au format existant ; information ℹ ; liste courte ; renvoi à `deepen`) selon l'exemple de `specs/002-silent-pitfalls/contracts/cli.md` § diagnose (dépend de T017)
- [X] T021 Étendre `cmd/kubectl-telemach/diagnose.go` : analyse de `<type>/<nom>`, routage vers `internal/pitfall`, message des types pris en charge (code `2`), enregistrement de `LastDiagnosis.Object` ; étendre `cmd/kubectl-telemach/last.go` pour réafficher un instantané d'objet sans relire le cluster, précédé de sa date. Faire passer T014 (dépend de T016, T017, T019, T020)
- [X] T022 Remplacer `kubectl get endpoints` par `kubectl get endpointslices -l kubernetes.io/service-name=<nom>` dans `internal/catalog/data/workload.yaml` et `internal/catalog/data/schema.yaml` (FR-032), régénérer les golden files concernés (`go test ./internal/render/ -update`) et relire leur différence (dépend de T018)

**Checkpoint**: lectures, instantanés, rendu et routage prêts ; `diagnose service/x` répond
« non encore pris en charge » proprement tant qu'US1 n'est pas faite.

---

## Phase 3: User Story 1 - Comprendre pourquoi un Service ne répond pas (Priority: P1) 🎯 MVP

**Goal**: `diagnose service/<nom>` suit le chemin du trafic et nomme le premier maillon cassé.

**Independent Test**: W1 à W4 du quickstart sur kind : selector sans pod, pods non Ready,
`targetPort` nommé introuvable, port non déclaré (aucune cause affirmée), Service sans
selector, Service sain.

### Tests for User Story 1 ⚠️

- [X] T023 [P] [US1] Écrire `internal/pitfall/service_test.go` sur le corpus `testdata/objects/{selector-typo,not-ready,targetport-name,targetport-undeclared,no-selector,healthy}/` : maillons et états attendus de `expected.yaml`, labels proches (au plus 3), `targetPort` numérique non déclaré → mention « déclaration facultative », conteneur sans port → `unknown` sans cause, lecture d'EndpointSlices refusée → `unknown` et diagnostic partiel ; mesure du taux de bonnes causes (SC-001 ≥ 90 %) et 0 piège sur `healthy` (SC-002)
- [X] T024 [P] [US1] Écrire `internal/render/object_golden_test.go` : rendus de chaque cas US1 dans `testdata/golden/f002/`, couleur et sans couleur, contrôlés par `TestGoldenFrench` et `TestGoldenNoSecret` (FR-031) ; noms DNS et `kubectl port-forward service/<nom>` pour le Service sain (FR-005)
- [X] T025 [P] [US1] Écrire `cmd/kubectl-telemach/diagnose_ambiguous_test.go` : `diagnose hello` avec un Deployment et un Service `hello` → liste des deux objets et commandes à copier, code `2` ; un seul pod → diagnostic direct (compatibilité feature 001) ; rien → noms proches, code `2` (FR-016)
- [X] T026 [P] [US1] Écrire les scénarios W1 à W4 dans `tests/integration/pitfalls_test.go` (étiquette `integration`, bash et zsh, commandes exactes du quickstart, état du namespace identique avant et après chaque `diagnose`)

### Implementation for User Story 1

- [X] T027 [P] [US1] Rédiger `internal/catalog/data/service.yaml` : entrées `service.no-matching-pods`, `service.pods-not-ready`, `service.targetport-name`, `service.targetport-number`, `service.ports-undeclared`, `service.no-selector`, `service.external-name`, `service.healthy` ; concepts *label selector*, *endpoints* (EndpointSlice), *Service ports* (`port`, `targetPort`, `containerPort`, `nodePort`), *DNS d'un Service* ; corrections prêtes à copier qui conservent contexte et namespace ; liens vers la documentation officielle
- [X] T028 [US1] Implémenter `internal/pitfall/service.go` : maillons `type` → `selector` → `readiness` → `endpoints` → `ports` dans cet ordre (research.md S4), labels proches par `internal/textdist`, renvoi `diagnose pod/<nom>` pour les pods non Ready. Faire passer T023 (dépend de T016, T017, T027)
- [X] T029 [US1] Brancher `service`/`svc` et la résolution des noms sans type (pod, Service, Deployment) dans `cmd/kubectl-telemach/diagnose.go` ; ajouter à `diagnose` sans argument, après les pods, la liste des Services sans pod prêt (au plus 10 objets au total, lectures parallèles, research.md S8). Faire passer T024, T025 (dépend de T020, T021, T028)
- [X] T030 [US1] Valider W1 à W4 sur kind via la CI (`integration`) et faire passer T026 (dépend de T029)

**Checkpoint**: MVP — un Service qui ne répond pas est expliqué maillon par maillon.

---

## Phase 4: User Story 2 - Comprendre un déploiement qui n'avance pas (Priority: P2)

**Goal**: `diagnose deployment/<nom>` décrit le rollout, signale le tag mutable et la
configuration figée, puis diagnostique le pod en échec de la nouvelle version.

**Independent Test**: W5 à W7 du quickstart et corpus `rollout-stalled`, `mutable-tag`,
`config-env`.

### Tests for User Story 2 ⚠️

- [X] T031 [P] [US2] Écrire `internal/pitfall/rollout_test.go` sur `testdata/objects/{rollout-stalled,healthy,mutable-tag}/` et un rollout en pause : phases `complete`, `progressing`, `stalled`, `paused` (research.md S5), nouveau et ancien ReplicaSet par l'annotation de révision, pod en échec le plus récent du nouveau ReplicaSet diagnostiqué par `workload.Diagnose`, `kubectl rollout undo` proposé comme choix de l'apprenant (FR-008)
- [X] T032 [P] [US2] Écrire `internal/pitfall/image_test.go` : référence sans tag, `:latest`, avec registre et port (`registre:5000/app`), avec digest `@sha256:` (jamais signalée), tag explicite (non signalé)
- [X] T033 [P] [US2] Écrire `internal/pitfall/config_test.go` sur `testdata/objects/config-env/` : ConfigMap modifiée après le démarrage du plus ancien pod → `config.env-stale` ; montage de volume sans `subPath` → non signalé ; montage `subPath` → signalé ; Secret en `env`/`envFrom` → `config.secret-env` (`info`) sans lecture de Secret ; ni `prod` ni aucune valeur du corpus de secrets dans l'instantané, le rendu ou `last.json` (SC-006)
- [X] T034 [P] [US2] Écrire les scénarios W5 à W7 dans `tests/integration/pitfalls_test.go`

### Implementation for User Story 2

- [X] T035 [P] [US2] Rédiger `internal/catalog/data/rollout.yaml` : `rollout.stalled`, `rollout.progressing`, `rollout.paused`, `image.mutable-tag`, `config.env-stale`, `config.secret-env` ; concepts *rollout* (ReplicaSet, version), *progress deadline*, *image tag*, *configuration figée au démarrage* ; correction `kubectl rollout restart deployment/<nom>`
- [X] T036 [P] [US2] Implémenter `internal/pitfall/image.go` (analyse d'une référence d'image OCI : registre, chemin, tag, digest). Faire passer T032
- [X] T037 [US2] Implémenter `internal/pitfall/rollout.go` (research.md S5) et `internal/pitfall/config.go` (research.md S3 : dates de ConfigMap comparées à `status.startTime`, Secrets jamais lus). Faire passer T031, T033 (dépend de T016, T017, T035, T036)
- [X] T038 [US2] Brancher `deployment`/`deploy` dans `cmd/kubectl-telemach/diagnose.go` (le diagnostic des pods d'un Deployment de la feature 001 devient la dernière étape du rollout) et ajouter les rollouts bloqués à la liste de `diagnose` sans argument (dépend de T029, T037)
- [X] T039 [US2] Valider W5 à W7 sur kind via la CI et faire passer T034 (dépend de T038)

**Checkpoint**: un rollout bloqué, un tag mutable et une configuration figée sont expliqués.

---

## Phase 5: User Story 3 - Comprendre un Ingress ou un stockage qui reste en attente (Priority: P3)

**Goal**: `diagnose ingress/<nom>` et `diagnose pvc/<nom>`.

**Independent Test**: W8 et W9 du quickstart et corpus `ingress-*`, `pvc-*`.

### Tests for User Story 3 ⚠️

- [X] T040 [P] [US3] Écrire `internal/pitfall/ingress_test.go` sur `testdata/objects/{ingress-no-class,ingress-unknown-class,ingress-backend-missing,ingressclass-not-default}/` : classe effective (`ingressClassName`, annotation dépréciée signalée, classe par défaut), aucune classe → « aucun contrôleur installé » sans contrôleur nommé (FR-012), adresse vide expliquée, Service ou port visé absent, au plus 10 backends lus, IngressClasses non lisibles → `unknown`
- [X] T041 [P] [US3] Écrire `internal/pitfall/pvc_test.go` sur `testdata/objects/{pvc-unknown-class,pvc-waiting,pvc-empty-class}/` : classe inexistante, classe par défaut utilisée, `storageClassName: ""` expliqué, `WaitForFirstConsumer` sans pod → `pvc.waiting-first-consumer` (`info`, transitoire, jamais présenté comme une erreur), pod consommateur `Pending` → diagnostic de ce pod
- [ ] T042 [P] [US3] Écrire les scénarios W8 et W9 dans `tests/integration/pitfalls_test.go` (`ingressclass-not-default.yaml` crée seulement un objet IngressClass, aucun contrôleur installé, research.md S6)

### Implementation for User Story 3

- [X] T043 [P] [US3] Rédiger `internal/catalog/data/ingress.yaml` (`ingress.no-class`, `ingress.unknown-class`, `ingress.no-controller`, `ingress.no-address`, `ingress.backend-missing`, `ingress.backend-port` ; concepts *Ingress* et *IngressClass*, contrôleur ; mention de Gateway API dans l'approfondissement) et `internal/catalog/data/storage.yaml` (`pvc.unknown-class`, `pvc.no-default-class`, `pvc.empty-class`, `pvc.waiting-first-consumer` ; concepts *StorageClass*, provisionnement dynamique, *volumeBindingMode*)
- [X] T044 [US3] Implémenter `internal/pitfall/ingress.go` (research.md S6) et `internal/pitfall/pvc.go` (research.md S7). Faire passer T040, T041 (dépend de T016, T017, T043)
- [X] T045 [US3] Brancher `ingress`/`ing` et `pvc`/`persistentvolumeclaim` dans `cmd/kubectl-telemach/diagnose.go`, ajouter Ingress et PVC en attente anormale à `diagnose` sans argument, et mesurer `diagnose` sans argument < 5 s au p95 avec toutes les lectures parallèles (SC-004, D-LL-012) dans `cmd/kubectl-telemach/diagnose_perf_test.go` (dépend de T038, T044)
- [ ] T046 [US3] Valider W8, W9 et W10 sur kind via la CI et faire passer T042 ; ajouter W10 (sans argument, nom ambigu) à `tests/integration/pitfalls_test.go` (dépend de T045)

**Checkpoint**: les quatre types d'objets sont diagnostiqués, y compris sans argument.

---

## Phase 6: User Story 4 - Être prévenu d'un piège juste après l'avoir créé (Priority: P4)

**Goal**: la vérification différée signale, à l'invite suivante, les pièges non transitoires
des objets créés, y compris après `expose`.

**Independent Test**: W11 et W6 (ligne ℹ du tag mutable) du quickstart dans un shell
interactif bash et zsh.

### Tests for User Story 4 ⚠️

- [ ] T047 [P] [US4] Étendre `internal/workload/targets_test.go` : cibles `service`, `ingress`, `pvc` des manifestes locaux, `create service clusterip web`, `expose deployment hello --name=hello-typo` (nom de `--name`, sinon de la ressource), images des manifestes ; `CoveredBySameApply` vrai quand la même commande applique un Pod ou Deployment dont les labels satisfont le selector du Service ; aucune cible pour `--dry-run`, `-f -`, URL, `-k`
- [ ] T048 [P] [US4] Étendre `cmd/kubectl-telemach/watch_test.go` (faux kubectl) : une ligne par piège non transitoire au format de `contracts/cli.md` § watch ; rien pour un piège `Transient` (FR-024) ni pour un objet dont une lecture a échoué ; au plus 12 lectures ; ligne ℹ du tag mutable au plus une fois par image et par 24 h, **sans aucune lecture** (FR-023) ; 0 signal sur `healthy` et `watch-together` (SC-002)
- [ ] T049 [P] [US4] Étendre `tests/shell/watch_test.go` (pseudo-terminal, bash et zsh) : `expose` déclenche la vérification ; invite retardée < 50 ms au p95 (SC-007) ; `TELEMACH_WATCH=0` et `TELEMACH_DISABLE=1` → aucun appel
- [ ] T050 [P] [US4] Écrire le scénario W11 dans `tests/integration/pitfalls_test.go`

### Implementation for User Story 4

- [ ] T051 [US4] Étendre `internal/workload/targets.go` : `ObjectTarget` (data-model.md), cibles d'objets et d'images, `expose` et `create service`, `CoveredBySameApply`. Faire passer T047
- [ ] T052 [US4] Étendre `cmd/kubectl-telemach/watch.go` : détections non transitoires de research.md S9 par les fonctions de `internal/pitfall`, budget de 12 lectures, ligne ℹ par `internal/state/hints.go`. Faire passer T048 (dépend de T028, T044, T051, T019)
- [ ] T053 [US4] Ajouter `expose` aux commandes qui lancent la vérification dans `internal/shellinit/scripts/bash.sh` et `internal/shellinit/scripts/zsh.sh` (`__telemach_after_success`). Faire passer T049 (dépend de T052)
- [ ] T054 [US4] Valider W11 sur kind via la CI et faire passer T050 (dépend de T053)

**Checkpoint**: les pièges deviennent visibles sans que l'apprenant ait à les chercher.

---

## Phase 7: User Story 5 - Comprendre de nouvelles erreurs kubectl courantes (Priority: P5)

**Goal**: nouvelles erreurs du catalogue, motif DNS, anti-pattern de suppression forcée.

**Independent Test**: W12 et W13 du quickstart ; corpus `testdata/errors/` complété par T008.

### Tests for User Story 5 ⚠️

- [ ] T055 [P] [US5] Étendre `tests/shell/shellinit_test.go` (pseudo-terminal, bash et zsh) **avant** toute modification des scripts : avec stderr capté, `exec -it` vers un faux kubectl interactif reste utilisable (saisie, écho unique, D-LL-002), Ctrl-C est transmis, code de sortie préservé ; `port-forward` interrompu par Ctrl-C (code 130) n'appelle `explain` que si une erreur de transfert reconnue a été captée ; `edit`, `proxy`, `logs -f`, `get -w` restent sans capture (research.md S10)
- [ ] T056 [P] [US5] Vérifier que `internal/diagnose/corpus_test.go` couvre chaque nouveau cas de `testdata/errors/` (SC-003 : 100 % reconnus) et ajouter dans `internal/catalog/catalog_test.go` le contrôle FR-028 : toute entrée dont une correction ou un texte mentionne `--force --grace-period=0` ou la suppression des finalizers porte un `anti_pattern`
- [ ] T057 [P] [US5] Réintégrer au corpus de pods `testdata/pods/dns-short-name/` (relevé par T008, mis de côté jusqu'à l'US5 ; journal réel : « wget: bad address 'kubernetes:443' ») et au test `internal/workload/diagnose_test.go` le cas DNS (FR-027)
- [ ] T058 [P] [US5] Écrire les scénarios W12 et W13 dans `tests/integration/pitfalls_test.go` (session `exec -it` d'un nom correct utilisable, codes préservés)

### Implementation for User Story 5

- [ ] T059 [US5] Étendre la capture de stderr à `exec`, `attach`, `debug` et `port-forward` dans `internal/shellinit/scripts/bash.sh` et `internal/shellinit/scripts/zsh.sh` (research.md S10), avec le cas du code 130 ; si un test de T055 ne peut pas passer, appliquer le repli de S10 pour la commande concernée et le consigner dans `specs/002-silent-pitfalls/research.md`. Faire passer T055
- [ ] T060 [P] [US5] Rédiger les entrées `notfound.container-exec` (`internal/catalog/data/notfound.yaml`), `cmdline.container-required` (`internal/catalog/data/cmdline.yaml`, une correction par conteneur cité), `internal/catalog/data/portforward.yaml` (`portforward.local-port-in-use`, `portforward.pod-not-running`, `portforward.connection-refused`, transfert vers `service/<nom>` recommandé), `rollout.progress-deadline` (`internal/catalog/data/rollout.yaml`, renvoi à `diagnose deployment/<nom>`), `conflict.field-immutable` (`internal/catalog/data/conflict.yaml`, deux voies, suppression jamais exécutée), à partir des seuls messages relevés par T008 (relevés mis de côté jusqu'à l'US5 pour ne pas laisser le test de corpus au rouge : les réintégrer dans `testdata/errors/` avec leurs `expected.yaml` ; « a container name must be specified » n'existe plus en 1.35 à 1.37, kubectl choisit un conteneur par défaut : entrée abandonnée ; le vrai message de exec et logs est « container … is not valid for pod … »)
- [ ] T061 [P] [US5] Ajouter le motif de journal `runtime.dns-short-name` à `internal/catalog/data/runtime.yaml` (« could not resolve host », « no such host », « Name or service not known » sur un nom sans point ; corrections `service.namespace` et `service.namespace.svc.cluster.local`). Faire passer T057
- [ ] T062 [US5] Ajouter la note `anti_pattern` de suppression forcée (données corrompues, garanties d'un StatefulSet, ressources orphelines ; démarche : finalizers, nœud injoignable, volume attaché) aux entrées concernées (`notfound.yaml`, `workload.yaml`, `conflict.yaml`) et faire passer T056 (dépend de T060)
- [ ] T063 [US5] Valider W12 et W13 sur kind via la CI et faire passer T058 (dépend de T059, T060, T061, T062)

**Checkpoint**: les erreurs courantes de `exec`, `logs`, `port-forward`, `rollout status` et
des champs immuables sont expliquées.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T064 [P] Mettre à jour `SECURITY.md` et `README.md` (liste des lectures du cluster, quatre annotations publiques, dates de ConfigMap sans valeur, nouveaux types de `diagnose`, `expose`, capture de stderr des commandes interactives, `hints.json`) dans la même PR que le code (contracts/cli.md § Sécurité transverse) ; chaque commande du README est rejouée par un test (LL-004)
- [ ] T065 [P] Étendre `cmd/kubectl-telemach/language_test.go` (`TestCorpusLanguageAndSecrets`) au corpus `testdata/objects/` : chaque cas passé par `diagnose <type>/<nom>` puis `last`, aucune valeur sensible dans la sortie, `last.json` et `hints.json`, textes en français (SC-006, LL-003)
- [ ] T066 [P] Ajouter W14 à `tests/integration/pitfalls_test.go` : état du namespace (`get all,configmaps,pvc,ingress,endpointslices -o json`, hors champs volatils) identique avant et après chaque `diagnose` et `watch` (SC-005) ; lecture refusée via `--as=system:serviceaccount:<ns>:default` → diagnostic partiel
- [ ] T067 [P] Ajouter une cible de fuzzing `FuzzImageRef` (`internal/pitfall/image_test.go`) au job hebdomadaire `.github/workflows/fuzz.yml` (60 s) et vérifier que toute nouvelle règle de masquage éventuelle a son cas de fuzzing (LL-001)
- [ ] T068 Lancer `scripts/couverture 80` (≥ 80 % sur `internal/`, `internal/pitfall` compris), golangci-lint compilé avec Go 1.27.2 (D-LL-001), `go test -race ./...`, `scripts/secrets` ; corriger tout écart
- [ ] T069 Vérifier dans le journal détaillé du job `integration` que W1 à W14 passent sous bash et zsh sur les trois versions, sans aucun test ignoré (D-LL-009), et consigner tout écart dans `LESSON-LEARNED.md`
- [ ] T070 Relire `LESSON-LEARNED.md`, transformer les leçons automatisables en tests ou gates (principe V), ouvrir la PR de la feature (description, gates vertes, SC-008 non mesurable en CI signalé) et relire `gh pr checks` après chaque push (LL-002)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** : T001 à T004 en parallèle ; T005 → T006 → T008 ; T004 + T005 → T007 → T008. Le corpus (T008) bloque tous les tests sur corpus.
- **Foundational (Phase 2)** : dépend de la Phase 1 ; bloque toutes les histoires.
- **US1 (Phase 3)** : dépend de la Phase 2 uniquement.
- **US2 (Phase 4)** : dépend de la Phase 2 ; T038 dépend de T029 (même fichier `diagnose.go`, même liste sans argument).
- **US3 (Phase 5)** : dépend de la Phase 2 ; T045 dépend de T038 (même fichier).
- **US4 (Phase 6)** : dépend des détections de US1 (T028) et US3 (T044).
- **US5 (Phase 7)** : dépend de la Phase 1 (corpus d'erreurs) et de T018 ; indépendante de US1 à US4, sauf `rollout.yaml` partagé avec T035 (T060 après T035).
- **Polish (Phase 8)** : dépend des histoires livrées.

### Dépendances fines notables

- T006 → T016 (commande des dates de ConfigMap) ; T015 → T016, T017 ; T017 → T020 → T021
- T027 → T028 → T029 → T030 ; T035 + T036 → T037 → T038 → T039 ; T043 → T044 → T045 → T046
- T028 + T044 + T051 + T019 → T052 → T053 → T054
- T055 → T059 ; T060 → T062 → T063

### Within Each User Story

- Tests écrits d'abord, et qui échouent
- Catalogue → diagnostic (`internal/pitfall`) → rendu → branchement CLI → validation kind
- Histoire validée par son checkpoint avant la priorité suivante

### Parallel Opportunities

- Phase 1 : T001 à T004
- Phase 2 : tests T009 à T014 en parallèle ; puis T018 et T019 en parallèle de T015 à T017
- US1 : T023 à T027 ; US2 : T031 à T036 ; US3 : T040 à T043 ; US4 : T047 à T050 ; US5 : T055 à T058, puis T060 et T061
- Après la Phase 2, US1, US2, US3 et US5 peuvent avancer en parallèle (hors fichiers partagés cités ci-dessus)

---

## Parallel Example: User Story 1

```bash
Task: "Écrire internal/pitfall/service_test.go sur le corpus testdata/objects/"
Task: "Écrire internal/render/object_golden_test.go"
Task: "Écrire cmd/kubectl-telemach/diagnose_ambiguous_test.go"
Task: "Rédiger internal/catalog/data/service.yaml"
```

## Parallel Example: User Story 2

```bash
Task: "Écrire internal/pitfall/rollout_test.go"
Task: "Écrire internal/pitfall/image_test.go"
Task: "Écrire internal/pitfall/config_test.go"
Task: "Rédiger internal/catalog/data/rollout.yaml"
Task: "Implémenter internal/pitfall/image.go"
```

## Parallel Example: User Story 5

```bash
Task: "Étendre tests/shell/shellinit_test.go (pseudo-terminal, capture de exec -it)"
Task: "Ajouter le contrôle FR-028 à internal/catalog/catalog_test.go"
Task: "Rédiger internal/catalog/data/portforward.yaml"
Task: "Ajouter le motif runtime.dns-short-name à internal/catalog/data/runtime.yaml"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 : corpus réel relevé sur les trois versions (T008 est le point de passage obligé)
2. Phase 2 : lectures, instantanés, rendu, routage
3. Phase 3 : `diagnose service/<nom>`
4. **STOP and VALIDATE** : W1 à W4 verts sur kind (bash, zsh, trois versions)
5. Ouvrir la PR en brouillon dès la Phase 1 pour vérifier les gates (D-LL-005)

### Incremental Delivery

1. Setup + Foundational → base prête
2. US1 → Service (MVP)
3. US2 → rollout, tag, configuration
4. US3 → Ingress, PVC, diagnostic sans argument complet
5. US4 → signal après création
6. US5 → nouvelles erreurs et capture des commandes interactives
7. Polish → documentation, sécurité, PR

### Notes

- [P] tasks = fichiers différents, aucune dépendance
- Commit après chaque tâche ou groupe logique, validation enchaînée par `&&` (D-LL-008)
- Ne jamais écrire un motif ou une attente d'après un message supposé : uniquement d'après le corpus
- Arrêt à chaque checkpoint pour valider l'histoire indépendamment
