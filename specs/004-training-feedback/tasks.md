---

description: "Liste des tâches pour la feature 004-training-feedback"
---

# Tasks: Couvrir les TP d'une vraie formation

**Input**: Design documents from `/specs/004-training-feedback/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli.md, quickstart.md

**Tests**: inclus. FR-029 à FR-031 et SC-001 à SC-009 exigent des corpus réels, des mesures
et le rejeu de chaque scénario du quickstart (LL-004) ; le principe IV exige des tests
bloquants en CI. Dans chaque histoire, les tests sont écrits **avant** l'implémentation et
doivent d'abord échouer.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 à US6 de spec.md)
- Include exact file paths in descriptions

## Conventions du projet (à respecter dans chaque tâche)

- Conventions des features 001 à 003 inchangées : code et identifiants en anglais ;
  commentaires, messages, catalogue et documentation en français avec accents (principe II) ;
  toute chaîne affichée ou écrite passe par `sanitize` puis `redact` ; relire
  `LESSON-LEARNED.md` avant chaque phase (principe V).
- **Aucun test ne touche le contexte kubectl courant** : cluster kind jetable uniquement
  (D-LL-015), à trois nœuds pour cette feature (`testdata/kind/three-nodes.yaml`).
- **Toute lecture du cluster passe par `internal/kuberead`** et figure dans la liste fermée
  (002 research.md S1 + 004 research.md F1) ; jamais d'objet Secret, de valeur d'`env`, ni de
  `status.addresses` d'un nœud.
- **Aucun motif ni aucune attente n'est écrit avant le relevé des trois versions** (LL-008,
  research.md F12) ; seuls `debug-ephemeral-disabled` et `apply-webhook-unreachable` portent
  `source: formation`.
- Toute vérification partielle de `check` est une **information** (ℹ), sans effet sur le code
  de sortie (LL-016) ; une entrée qui reconnaît une réponse du serveur contenant un texte
  réseau générique est rangée avant `connection.yaml` (LL-014).
- `testdata/objects/scenarios/` ne contient **que** des manifestes (LL-004, LL-005).
- Validation et commit enchaînés par `&&` (D-LL-008), **sans aucun `;` entre la première
  validation et `git push`** (LL-018) ; `golangci-lint fmt` puis `run`
  (v2.15.0, `GOTOOLCHAIN=go1.27.3`) avant chaque push ; après chaque push, relire
  `gh pr checks` (LL-002) ; `scripts/secrets` avant chaque commit.
- Cette feature part de `main` **après fusion de la PR #20** (niveau information de `check`,
  `admission.yaml`, LL-014 à LL-017) : rebaser `004-training-feedback` avant T009.

---

## Phase 1: Setup (corpus et outillage)

**Purpose**: rendre possibles les relevés réels des trois versions sur un cluster à trois
nœuds, avant toute logique.

- [X] T001 Revérifier les dernières versions stables réelles de Go, kind, des images `kindest/node` 1.35, 1.36, 1.37 (digests), de kubectl 1.35 à 1.37 et de metrics-server (v0.9.1 au 2026-10-29, digest de l'image), et mettre à jour `go.mod`, `.github/workflows/ci.yml`, `.github/workflows/corpus.yml` et `specs/004-training-feedback/research.md` § Versions si un correctif est sorti
- [X] T002 [P] Créer `testdata/kind/three-nodes.yaml` (un control-plane, deux workers) et `testdata/kind/metrics-server.yaml` (manifeste officiel de la version de T001, image épinglée par digest, `--kubelet-insecure-tls` commenté « cluster de test kind seulement, jamais en production ») ; ajouter une option `Nodes`/`Config` à `tests/integration/kindcluster/kindcluster.go` pour créer un cluster à partir d'un fichier de configuration
- [X] T003 [P] Créer les manifestes des scénarios dans `testdata/objects/scenarios/` (manifestes seulement, non-root, `resources` déclarées sauf défaut volontaire, images à tag vérifié D-LL-004) : `quota.yaml` (ResourceQuota `limits.cpu: "1"`, `limits.memory: 256Mi`), `quota-deployment.yaml` (Deployment `api`, `limits.cpu: 500m`), `limitrange.yaml` (LimitRange `max.cpu: 500m` par conteneur), `limitrange-pod.yaml` (pod `limits.cpu: "2"`), `pss-violation.yaml` (pod sans `allowPrivilegeEscalation: false`, sans `capabilities.drop`, sans `seccompProfile`), `pending-placement.yaml` (pod `place` sans tolérance, anti-affinité requise envers lui-même via un second pod), `drain-pdb.yaml` (Deployment `web` 1 réplique + PDB `minAvailable: 1` + pod `cache` avec `emptyDir`, nodeSelector `kubernetes.io/hostname: telemach-worker` remplacé par le test), `job-fails.yaml` (Job `calc`, `backoffLimit: 1`, `exit 3`), `cronjob-suspended.yaml` (CronJob `sauvegarde`, `suspend: true`), `daemonset.yaml` (DaemonSet `agent` sans tolérance), `statefulset-no-service.yaml` (StatefulSet `base`, `serviceName: base`, sans Service), `hpa-no-request.yaml` (Deployment `web` sans `requests.cpu` + HPA `web` 50 % CPU), `hpa-limited.yaml` (Deployment avec requests + HPA `maxReplicas: 1`) ; déclarer chaque défaut volontaire dans `testdata/objects/scenarios.expected.yaml`
- [X] T004 [P] Créer `testdata/manifests/invalid-cron/cronjob.yaml` (`schedule: "*/5 * * *"`) et `expected.yaml`, et `testdata/manifests/helm-rendered/rendu.yaml` (sortie réelle de `helm template` d'un petit chart `telemarche` avec commentaires `# Source:`, un document fautif) et `expected.yaml` (attentes du fichier ; T052 et T053 rejouent le même contenu par l'entrée standard)
- [X] T005 Ajouter à `scripts/collect-errors/main.go` le cluster à trois nœuds et les cas : `create-quota-exceeded`, `run-quota-must-specify`, `apply-limitrange`, `apply-pod-security`, `apply-pod-security-warn` (stderr d'une commande **réussie**, code 0), `drain-daemonset-pods`, `drain-pdb-blocked`, `drain-local-storage`, `top-metrics-unavailable`, `wait-condition-timeout`, `wait-deployment-timeout`, `wait-jsonpath-timeout`, `wait-delete-timeout`, `rollout-undo-revision-missing`, `apply-cronjob-invalid` ; relire dans le code de kubectl les codes de sortie de `drain`, `wait`, `top` et `debug` (LL-017) et les noter en commentaire (dépend de T002, T003, T004)
- [X] T006 Ajouter à `scripts/collect-objects/main.go` les lectures de research.md F1 et les cas : pods `Pending` (taint, anti-affinité, cordon, combinaison), Job (`backoff-limit`, `deadline`, `pods-gone`, `complete`), CronJob (`suspended`, `never-run`, `forbid-skipped`, `last-failed`, sain), DaemonSet (`excluded-nodes` control-plane, taint d'un worker), StatefulSet (`no-service`, `ordered-wait`, `pvc-pending`, sain), HPA (`metrics-unavailable`, `target-missing`, `missing-request`, `limited`, sain), Deployment `replica-failure` ; relevés masqués dans `testdata/objects/<cas>/`, `expected.yaml` jamais écrasé (dépend de T002, T003)
- [X] T007 Étendre `.github/workflows/corpus.yml` : cluster à trois nœuds, installation de metrics-server de `testdata/kind/metrics-server.yaml` pour les seuls cas qui l'exigent, nouveaux cas de T005 et T006 ; actions épinglées par SHA, `permissions: contents: read`, `timeout-minutes` sur chaque job et sur les étapes réseau (LL-011) (dépend de T005, T006)
- [X] T008 Lancer `corpus.yml` sur la branche (`gh workflow run corpus.yml --ref 004-training-feedback`) ou en local avec kind v0.33.0 (`go install sigs.k8s.io/kind@v0.33.0`, Docker de WSL), télécharger l'artefact, le relire en entier (`scripts/secrets` vert, aucune adresse de nœud), comparer chaque message des trois versions à celui des issues #13 à #19, rédiger à la main chaque `expected.yaml`, créer le cas `testdata/errors/debug-ephemeral-disabled/` d'après l'issue #19 avec `source: formation`, puis versionner (dépend de T007)

**Checkpoint**: corpus réel des trois versions disponible ; écarts de messages consignés dans
research.md F12.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: lectures, types, alias, catégories et routage communs à toutes les histoires.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Tests for Foundational ⚠️

- [X] T009 [P] Étendre `internal/kuberead/allowlist_test.go` : chaque lecture de research.md F1 produit exactement sa commande (`get job|jobs|cronjob|daemonset|statefulset|hpa|nodes`, `get apiservice v1beta1.metrics.k8s.io`, `get events --field-selector involvedObject.kind=ReplicaSet,involvedObject.name=<rs>`, `-o json`), avec la portée de l'apprenant et `--request-timeout=10s` ; `TestNoOtherReads` échoue toujours pour `secret`, `describe`, `exec`, tout verbe d'écriture
- [X] T010 [P] Étendre `internal/kuberead/objects_test.go` : décodage des JSON réels de `testdata/objects/*/` pour Job, CronJob, DaemonSet, StatefulSet, HPA, Node, APIService ; test par réflexion : aucun champ `env`, `value`, `addresses`, `annotations` hors liste de la 002 dans les nouveaux types (data-model.md § Types lus) ; valeurs par défaut quand le champ manque : « `backoffLimit` 6, `concurrencyPolicy` Allow, `podManagementPolicy` OrderedReady, `minReplicas` 1 »
- [X] T011 [P] Étendre `cmd/kubectl-telemach/diagnose_route_test.go` : `job/`, `jobs/`, `cronjob/`, `cj/`, `daemonset/`, `ds/`, `statefulset/`, `sts/`, `hpa/`, `horizontalpodautoscaler/` routés ; type inconnu → liste de **tous** les types pris en charge, code `2`
- [X] T012 [P] Étendre `internal/redact/schema_fields_test.go` (LL-015) : après T014, les schémas embarqués couvrent HPA ; tout nouveau champ au nom sensible est classé

### Implementation for Foundational

- [X] T013 Ajouter à `internal/kuberead/commands.go` et `internal/kuberead/read.go` les lectures de research.md F1 et elles seules (`GetJob`, `ListJobs`, `GetCronJob`, `GetDaemonSet`, `GetStatefulSet`, `GetHPA`, `ListNodes`, `GetMetricsAPIService`, `ReplicaSetEvents`), et à `internal/kuberead/objects.go` les types de data-model.md § Types lus (champs listés seulement ; `Node` sans `status.addresses`) ; étendre le modèle de pod (`tolerations`, `requests`). Faire passer T009, T010 (dépend de T008)
- [X] T014 [P] Ajouter `autoscaling/v2/HorizontalPodAutoscaler` à `scripts/gen-schemas/sources.txt` et `scripts/gen-schemas/main.go`, régénérer `internal/schema/data/v1.3{5,6,7}.json.gz`, ajouter `HorizontalPodAutoscaler` à `knownKinds` si besoin dans `internal/manifest/semantic.go`. Faire passer T012
- [X] T015 [P] Ajouter les alias de research.md F8 à `internal/kubeterms/kubeterms.go` et les catégories `node`, `metrics`, `batch` à `internal/catalog/schema.go`
- [X] T016 Étendre `internal/pitfall/snapshot.go` (`KindJob`, `KindCronJob`, `KindDaemonSet`, `KindStatefulSet`, `KindHPA` ; maillons de data-model.md § Step) et `cmd/kubectl-telemach/diagnose.go` (routage, message des types, enregistrement `LastDiagnosis.Object`) ; tant qu'un diagnostiqueur manque, `pitfall.ErrNotYet` donne un message propre. Faire passer T011 (dépend de T013, T015)

**Checkpoint**: `diagnose job/x` répond proprement « pas encore pris en charge » ; les
lectures et types sont testés sur le corpus réel.

---

## Phase 3: User Story 1 - Comprendre un objet refusé à l'admission (Priority: P1) 🎯 MVP

**Goal**: quotas, LimitRange et PodSecurity reconnus, à la commande comme depuis un
Deployment, avec version corrigée et avertissement d'une commande réussie.

**Independent Test**: quickstart T1 à T4 sur kind.

### Tests for User Story 1 ⚠️

- [ ] T017 [P] [US1] Ajouter au corpus `testdata/errors/` les attentes de T008 pour les cas d'admission et un test d'ordre dans `internal/diagnose/diagnose_test.go` : un refus de quota ou PodSecurity n'est jamais reconnu par `auth.forbidden` ni par `connection.*` ; captures `quota`, `resource`, `requested`, `used`, `limited`, `level`, `violations` vérifiées
- [X] T018 [P] [US1] Écrire `internal/manifest/podsecurity_test.go` : à partir de la liste de violations d'un relevé réel, un problème `pod-security` par conteneur fautif de `pss-violation.yaml`, avec version corrigée (`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `runAsNonRoot: true`, `seccompProfile.type: RuntimeDefault`) qui passe `manifest-lint`
- [X] T019 [P] [US1] Étendre `cmd/kubectl-telemach/check_test.go` : conteneur sans `resources.limits` → information `missing-limits`, code de sortie inchangé ; mettre à jour les golden files de `check` concernés et relire leur différence
- [X] T020 [P] [US1] Écrire `internal/pitfall/rollout_admission_test.go` sur le corpus `replica-failure` : maillon `admission` en échec, leçon `admission.quota-exceeded` ; condition absente → événement `FailedCreate` lu ; lecture refusée → maillon `unknown`
- [X] T021 [P] [US1] Écrire `cmd/kubectl-telemach/notice_test.go` : stderr d'un relevé `apply-pod-security-warn` → une information ℹ, code 0, `hints.json` retient le namespace ; second appel dans les 24 h → rien ; aucune lecture du cluster (faux kubectl qui échoue s'il est appelé)
- [X] T022 [P] [US1] Étendre `tests/shell/` (bash et zsh, pseudo-terminal, LL-009) : `scale` déclenche `watch` ; `would violate PodSecurity` dans stderr d'une commande réussie déclenche `notice` sans processus supplémentaire ; mesure de latence de l'invite inchangée (SC-006, 100 échantillons, hors `-race`)

### Implementation for User Story 1

- [X] T023 [US1] Rédiger dans `internal/catalog/data/admission.yaml` les entrées `admission.quota-exceeded`, `admission.quota-must-specify`, `admission.limitrange`, `admission.pod-security` (research.md F3), motifs écrits d'après les relevés des trois versions ; corrections `describe resourcequota`, `describe limitrange`, `get ns <ns> --show-labels` ; anti-patterns « gonfler le quota » et « retirer le label pod-security.kubernetes.io/enforce ». Faire passer T017 (dépend de T008)
- [X] T024 [US1] Ajouter le type `pod-security` à `internal/manifest/issue.go`, la règle (appelée seulement par `explain`, jamais par `check`) dans `internal/manifest/podsecurity.go` et la version corrigée dans `internal/manifest/fixes.go` ; brancher dans `cmd/kubectl-telemach/explain.go` (`manifestAnalysis`) quand l'entrée reconnue est `admission.pod-security`. Faire passer T018 (dépend de T023)
- [X] T025 [P] [US1] Ajouter l'information `missing-limits` à `internal/manifest/semantic.go` et `internal/manifest/issue.go` (niveau information de la PR #20) et son entrée `schema.missing-limits` dans `internal/catalog/data/schema.yaml`. Faire passer T019
- [X] T026 [US1] Ajouter le maillon `admission` et le piège `deployment.replica-failure` à `internal/pitfall/rollout.go` (condition `ReplicaFailure` passée à `diagnose.Diagnose` comme stderr, repli sur `ReplicaSetEvents`) et l'entrée `pitfalls: [deployment.replica-failure]` dans `internal/catalog/data/rollout.yaml`. Faire passer T020 (dépend de T016, T023)
- [X] T027 [US1] Créer `cmd/kubectl-telemach/notice.go` (sous-commande cachée, contracts/cli.md § notice, clé `telemach:pod-security:<namespace>` dans `hints.json`) et l'entrée `admission.pod-security-warning` dans `internal/catalog/data/admission.yaml`. Faire passer T021 (dépend de T023)
- [X] T028 [US1] Modifier `internal/shellinit/scripts/bash.sh` et `internal/shellinit/scripts/zsh.sh` : `scale` dans `__telemach_after_success` ; test `[[ $(<"$__telemach_tmp") == *"would violate PodSecurity"* ]]` puis `notice` ; incrémenter `TELEMACH_KUBECTL_SHELL` ; étendre `internal/workload/targets.go` (cible Deployment de `scale`) et `cmd/kubectl-telemach/watch.go` (piège `deployment.replica-failure`). Faire passer T022 (dépend de T026, T027)
- [X] T029 [US1] Écrire les scénarios T1 à T4 dans `tests/integration/training_test.go` (commandes exactes du quickstart, namespace neuf par scénario, bash et zsh) et les valider sur kind (dépend de T023 à T028)

**Checkpoint**: US1 complète et validée seule (quickstart T1 à T4).

---

## Phase 4: User Story 2 - Comprendre un pod qui n'est placé sur aucun nœud (Priority: P2)

**Goal**: causes de placement détaillées par nœud, `drain` expliqué.

**Independent Test**: quickstart T5 et T6 sur kind à trois nœuds.

### Tests for User Story 2 ⚠️

- [X] T030 [P] [US2] Étendre `internal/workload/select_test.go` sur les messages réels de `testdata/objects/` : découpage `0/N nodes are available: …` en causes, partie `preemption:` ignorée, raisons `UntoleratedTaint` (taint extrait), `PodAntiAffinity`, `PodAffinity`, `NodeUnschedulable`, causes existantes inchangées, cause inconnue citée telle quelle, cause principale = plus grand nombre de nœuds, taint `node-role.kubernetes.io/control-plane` marqué normal (data-model.md § PlacementCause)
- [ ] T031 [P] [US2] Étendre `internal/render/pod_golden_test.go` : liste « Nœuds par cause » (exemple de contracts/cli.md § diagnose), noms des nœuds si la lecture des nœuds est permise, diagnostic partiel sinon ; golden files couleur et sans couleur dans `testdata/golden/f004/`
- [ ] T032 [P] [US2] Ajouter les attentes `drain-*` de T008 au corpus et vérifier que `--disable-eviction`, `--force` et `--delete-emptydir-data` n'apparaissent que dans des corrections `safe: false` ou des anti-patterns (`internal/catalog/catalog_test.go`, extension de `TestForcedDeletionIsAntiPattern`)

### Implementation for User Story 2

- [X] T033 [US2] Implémenter le découpage et `Causes []PlacementCause` dans `internal/workload/select.go` (research.md F6). Faire passer T030
- [X] T034 [US2] Rédiger `internal/catalog/data/node.yaml` : `drain.daemonset-pods`, `drain.pdb-blocked`, `drain.local-storage` (research.md F7) ; et dans `internal/catalog/data/workload.yaml` les entrées `pod.untolerated-taint` (cause et approfondissement : « une toleration autorise, elle ne force pas : il faut aussi un nodeSelector ou une affinité »), `pod.pod-anti-affinity`, `pod.node-unschedulable` (`kubectl uncordon`). Faire passer T032 (dépend de T008)
- [ ] T035 [US2] Lire les nœuds (`ListNodes`) dans `internal/workload/diagnose.go` seulement pour `UntoleratedTaint` et `NodeUnschedulable`, et rendre la liste « Nœuds par cause » dans `internal/render/pod.go`. Faire passer T031 (dépend de T013, T033, T034)
- [X] T036 [US2] Écrire les scénarios T5 et T6 dans `tests/integration/training_test.go` (exécution en série : ils modifient des nœuds du cluster jetable, et les remettent en état en fin de scénario même en cas d'échec) et les valider sur kind (dépend de T035)

**Checkpoint**: US2 complète et validée seule.

---

## Phase 5: User Story 3 - Diagnostiquer un Job, un CronJob, un DaemonSet ou un StatefulSet (Priority: P3)

**Goal**: quatre nouveaux types de `diagnose`, syntaxe cron vérifiée hors ligne.

**Independent Test**: quickstart T7 à T9 sur kind.

### Tests for User Story 3 ⚠️

- [X] T037 [P] [US3] Écrire `internal/cron/cron_test.go` et `internal/cron/fuzz_test.go` : 5 champs, listes, intervalles, pas, noms (`JAN`, `MON`), `?`, macros (`@hourly` … `@yearly`), bornes, `TZ=`/`CRON_TZ=` refusés avec renvoi vers `spec.timeZone`, messages en français avec champ et position ; `Next` sur des cas connus (fuseau `Europe/Paris`, passage à l'heure d'été) ; fuzzing sans panique
- [X] T038 [P] [US3] Écrire `internal/pitfall/job_test.go`, `cronjob_test.go`, `daemonset_test.go`, `statefulset_test.go` sur le corpus `testdata/objects/` : chaque piège de data-model.md § Pitfall reconnu, objet sain sans piège (SC-003), lecture refusée → maillon `unknown`
- [X] T039 [P] [US3] Étendre `internal/manifest/corpus_test.go` avec `invalid-cron` (problème, ligne et colonne de `spec.schedule`, exemple corrigé) et `cmd/kubectl-telemach/diagnose_test.go` : `diagnose` sans argument liste un Job en échec (faux kubectl)

### Implementation for User Story 3

- [X] T040 [US3] Implémenter `internal/cron/cron.go` (research.md F9, data-model.md § CronSchedule, sans dépendance). Faire passer T037
- [X] T041 [US3] Rédiger `internal/catalog/data/batch.yaml` : leçons des pièges `job.*`, `cronjob.*`, `daemonset.*`, `statefulset.*` (data-model.md § Pitfall) et l'entrée `manifest.cron-schedule` (motif d'après le relevé `apply-cronjob-invalid`, rangée avant les entrées génériques « is invalid ») avec les 5 champs et un exemple commenté (dépend de T008)
- [X] T042 [P] [US3] Implémenter `internal/pitfall/job.go` (pods par label `batch.kubernetes.io/job-name`, puis `job-name`) et `internal/pitfall/cronjob.go` (Jobs par `ownerReferences`, prochaine échéance par `internal/cron`, enchaînement sur le diagnostic du dernier Job). Faire passer leur part de T038 (dépend de T016, T040, T041)
- [X] T043 [P] [US3] Implémenter `internal/pitfall/daemonset.go` (nœuds exclus et taints, control-plane normal) et `internal/pitfall/statefulset.go` (Service de `serviceName`, `OrderedReady`, PVC `<modèle>-<nom>-<k>` avec la leçon PVC de la 002). Faire passer leur part de T038 (dépend de T016, T041)
- [X] T044 [US3] Ajouter le problème `invalid-cron` à `internal/manifest/semantic.go` et `internal/manifest/issue.go` (CronJob, `spec.schedule`, via `internal/cron`) avec son exemple corrigé dans `internal/manifest/fixes.go` ; ajouter `ListJobs` aux lectures parallèles de `diagnose` sans argument dans `cmd/kubectl-telemach/diagnose.go`, dans la limite globale de 10 objets ; cible `statefulset.no-service` dans `internal/workload/targets.go`. Faire passer T039 (dépend de T040, T042, T043)
- [X] T045 [US3] Écrire les scénarios T7 à T9 dans `tests/integration/training_test.go` et les valider sur kind (dépend de T044)

**Checkpoint**: US3 complète et validée seule.

---

## Phase 6: User Story 4 - Comprendre l'autoscaling et les métriques (Priority: P4)

**Goal**: metrics-server absent expliqué, `diagnose hpa/`, HPA sans request signalé (différé
et hors ligne).

**Independent Test**: quickstart T10 et T11 sur kind.

### Tests for User Story 4 ⚠️

- [X] T046 [P] [US4] Écrire `internal/pitfall/hpa_test.go` sur le corpus : ordre des maillons `metrics-api` → `target` → `requests` → `conditions`, premier maillon cassé nommé, `ScalingLimited` avec `TooManyReplicas` présenté comme `hpa.limited` (information), métriques `Pods`/`Object`/`External` « hors de portée », APIService illisible → `unknown`
- [X] T047 [P] [US4] Étendre `cmd/kubectl-telemach/check_test.go` : HPA et Deployment sans `requests.cpu` dans les mêmes fichiers → information `hpa-missing-request`, code 0 ; HPA seul (cible absente des fichiers) → rien ; ajouter `top-metrics-unavailable` aux attentes du corpus d'erreurs

### Implementation for User Story 4

- [X] T048 [US4] Rédiger `internal/catalog/data/metrics.yaml` : `metrics.api-unavailable` (concept metrics-server, `kubectl get apiservice v1beta1.metrics.k8s.io`, `minikube addons enable metrics-server`, « attendre environ une minute après l'installation » ; `--kubelet-insecure-tls` jamais proposé) et les leçons `hpa.*` (dépend de T008)
- [X] T049 [US4] Implémenter `internal/pitfall/hpa.go` (research.md F10). Faire passer T046 (dépend de T016, T048)
- [X] T050 [US4] Ajouter l'information `hpa-missing-request` à `internal/manifest/semantic.go` (règle inter-documents : HPA et cible dans les fichiers vérifiés) et son entrée dans `internal/catalog/data/schema.yaml` ; ajouter la cible HPA à `internal/workload/targets.go` et à `cmd/kubectl-telemach/watch.go` (2 lectures, budget de 12 inchangé). Faire passer T047 (dépend de T049)
- [X] T051 [US4] Écrire les scénarios T10 et T11 dans `tests/integration/training_test.go` (metrics-server installé seulement pour T11) et les valider sur kind (dépend de T050)

**Checkpoint**: US4 complète et validée seule.

---

## Phase 7: User Story 5 - Vérifier ce que produisent Helm, Kustomize et les générateurs (Priority: P5)

**Goal**: `check -f -` avec template d'origine.

**Independent Test**: quickstart T12 (sans cluster).

### Tests for User Story 5 ⚠️

- [X] T052 [P] [US5] Écrire `internal/manifest/stdin_test.go` : même résultat que le fichier pour chaque cas de `testdata/manifests/` (SC-009) ; `<stdin>` dans les positions ; `Document.Source` retenu depuis `# Source:` ; flux de 1 Mio + 1 octet → `file-unreadable` ; non UTF-8 → `not-utf8` ; vide → `no-manifest-file`
- [X] T053 [P] [US5] Étendre `cmd/kubectl-telemach/check_test.go` : `-f -` deux fois → code 2 ; entrée standard reliée à un terminal (pseudo-terminal de `tests/shell`) → message d'usage immédiat, code 2 ; `helm-rendered/rendu.yaml` par l'entrée standard → template d'origine dans la position ; `last` et `deepen` ne relisent pas `<stdin>`

### Implementation for User Story 5

- [X] T054 [US5] Implémenter `ReadStdin` et la reconnaissance de `-` dans `internal/manifest/read.go` (taille, UTF-8, nettoyage identiques à `Read`), `Document.Source` (dernier `# Source:` avant la première clé du document) et le rendu « template … » dans `internal/render/manifest.go`. Faire passer T052
- [X] T055 [US5] Brancher `-f -` dans `cmd/kubectl-telemach/check.go` (lecture de `a.stdin`, détection du terminal par `term.IsTerminal`, refus du double `-`, enregistrement `Files: ["<stdin>"]`). Faire passer T053 (dépend de T054)
- [X] T056 [P] [US5] Remplacer la recette de `helm-template` et `helm-values` dans `internal/catalog/data/schema.yaml` par `helm template demo {{.suggestion}} | {{.kubectl}} telemach check -f -`, et ajouter au `README.md` la recette (Helm, `kubectl kustomize`, `--dry-run=client -o yaml`) ; mettre à jour les golden files concernés
- [X] T057 [US5] Écrire le scénario T12 dans `tests/integration/training_test.go` (commandes exactes du quickstart, sous bash et zsh) (dépend de T055, T056)

**Checkpoint**: US5 complète et validée seule.

---

## Phase 8: User Story 6 - Comprendre les dernières erreurs courantes des TP (Priority: P6)

**Goal**: `wait`, `rollout undo`, `debug` expliqués ; relevé d'une formation.

**Independent Test**: quickstart T13 sur kind ; T14 par `corpus.yml`.

### Tests for User Story 6 ⚠️

- [X] T058 [P] [US6] Ajouter au corpus les attentes `wait-*`, `rollout-undo-revision-missing`, `debug-ephemeral-disabled` (`source: formation`) avec `fixed_command_contains: telemach diagnose pod/web` pour `wait-condition-timeout` ; test de `suggest.go` : type non pris en charge par `diagnose` → correction `kubectl describe`
- [X] T059 [P] [US6] Écrire `scripts/collect-errors/training_test.go` (étiquette `integration`) sur un faux dossier de formation de deux TP : tableau Markdown (TP, panne, commande, message masqué, entrée ou `unknown`/`unsupported`), namespace neuf par panne, prérequis de `telemach-releve.yaml` appliqués avant

### Implementation for User Story 6

- [X] T060 [US6] Rédiger les entrées `wait.timeout` (enrichisseur `diag_target` dans `internal/diagnose/suggest.go`, `--for=delete` relié à la leçon des finalizers), `rollout.undo-revision-missing` dans `internal/catalog/data/rollout.yaml`, `runtime.ephemeral-disabled` et le rappel `--target` dans le `deepen` de `runtime.exec-not-found` dans `internal/catalog/data/runtime.yaml`. Faire passer T058 (dépend de T008)
- [X] T061 [US6] Implémenter le mode `-training` de `scripts/collect-errors/main.go` (research.md F13) et l'ajouter à `.github/workflows/corpus.yml` (entrée `training_repo`, clone en lecture seule, sortie en artefact). Faire passer T059
- [X] T062 [US6] Écrire le scénario T13 dans `tests/integration/training_test.go` et le valider sur kind (dépend de T060)

**Checkpoint**: US6 complète ; relevé disponible pour SC-004.

---

## Phase 9: Polish & Cross-Cutting Concerns

- [X] T063 [P] Étendre `TestGoldenFrench`, `TestGoldenNoSecret` et `TestCorpusLanguageAndSecrets` (`cmd/kubectl-telemach/language_test.go`) aux nouveaux rendus et au corpus de la feature (FR-031, SC-008)
- [X] T064 [P] Mesurer SC-005 (`diagnose` d'un Job, CronJob, DaemonSet, StatefulSet, HPA < 5 s au p95) dans `cmd/kubectl-telemach/diagnose_perf_test.go`, hors `-race`, 100 échantillons (LL-006, D-LL-012)
- [X] T065 [P] Vérifier SC-007 avec le faux kubectl journalisant : aucune lecture hors liste fermée pendant toute la suite (`internal/kuberead/allowlist_test.go`, `TestNoOtherReads` sur les journaux de `tests/integration/training_test.go`)
- [X] T066 Réévaluer la durée et les `timeout-minutes` de la gate `integration` dans `.github/workflows/ci.yml` avec le cluster à trois nœuds et T1 à T13 (LL-011) ; découper en deux jobs si la durée dépasse le double de l'actuelle
- [ ] T067 Lancer `corpus.yml` en mode `-training` sur kubernetes-formation (quickstart T14), consigner le tableau dans `specs/004-training-feedback/research.md` (SC-004) et ouvrir une issue par message encore non reconnu hors périmètre
- [X] T068 [P] Mettre à jour `README.md` (nouveaux types de `diagnose`, `check -f -`, informations ℹ), `CONTRIBUTING.md` (cluster à trois nœuds, relevé d'une formation) et `docs/dockerhub.md` si l'image documente `check`
- [ ] T069 Relire `LESSON-LEARNED.md`, consigner tout incident survenu pendant la feature, rejouer le quickstart complet, puis cocher chaque tâche et ouvrir la PR « Feature 004 » qui ferme #13, #14, #15, #16, #17, #19

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** : T001 à T004 en parallèle ; T002 + T003 + T004 → T005 ; T002 + T003 → T006 ; T005 + T006 → T007 → T008. Le corpus (T008) bloque tous les motifs et toutes les attentes.
- **Prérequis externe** : fusion de la PR #20 et rebase avant T009.
- **Foundational (Phase 2)** : dépend de la Phase 1 ; bloque toutes les histoires.
- **US1 (Phase 3)** : dépend de la Phase 2.
- **US2 (Phase 4)** : dépend de la Phase 2 ; indépendante de US1.
- **US3 (Phase 5)** : dépend de la Phase 2 ; T044 touche `diagnose.go` et `targets.go`, aussi modifiés par T028 : faire T044 après T028 si les deux histoires avancent en parallèle.
- **US4 (Phase 6)** : dépend de la Phase 2 ; T050 après T028 et T044 (`targets.go`, `watch.go`).
- **US5 (Phase 7)** : dépend de la Phase 1 (T004) seulement : aucune lecture du cluster ; T056 après T025 (`schema.yaml`).
- **US6 (Phase 8)** : dépend de la Phase 2 ; T060 après T026 (`rollout.yaml`).
- **Polish (Phase 9)** : dépend des histoires livrées.

### Dépendances fines notables

- T013 → T016 → (T026, T042, T043, T049) ; T023 → T024, T026, T027 → T028 → T029
- T033 → T035 → T036 ; T040 → T042 → T044 → T045 ; T048 → T049 → T050 → T051
- T054 → T055 → T057 ; T060 → T062 ; T061 → T067

### Within Each User Story

- Tests écrits d'abord, et qui échouent
- Catalogue → diagnostic (`internal/pitfall`, `internal/workload`, `internal/manifest`) → rendu → branchement CLI → scénarios kind
- Histoire validée par son checkpoint avant la priorité suivante

### Parallel Opportunities

- Phase 1 : T001 à T004
- Phase 2 : tests T009 à T012 ; puis T014 et T015 en parallèle de T013
- US1 : T017 à T022, puis T025 en parallèle de T023/T024
- US2 : T030 à T032
- US3 : T037 à T039, puis T042 et T043
- US4 : T046, T047
- US5 : T052, T053, puis T056
- US6 : T058, T059, puis T060 et T061
- Après la Phase 2, US1, US2, US5 et US6 peuvent avancer en parallèle (hors fichiers partagés cités ci-dessus)

---

## Parallel Example: User Story 1

```bash
Task: "Ajouter les attentes d'admission au corpus et le test d'ordre dans internal/diagnose/diagnose_test.go"
Task: "Écrire internal/manifest/podsecurity_test.go"
Task: "Écrire internal/pitfall/rollout_admission_test.go"
Task: "Écrire cmd/kubectl-telemach/notice_test.go"
```

## Parallel Example: User Story 3

```bash
Task: "Écrire internal/cron/cron_test.go et internal/cron/fuzz_test.go"
Task: "Écrire internal/pitfall/job_test.go, cronjob_test.go, daemonset_test.go, statefulset_test.go"
Task: "Implémenter internal/pitfall/daemonset.go et statefulset.go"
```

## Parallel Example: User Story 5

```bash
Task: "Écrire internal/manifest/stdin_test.go"
Task: "Étendre cmd/kubectl-telemach/check_test.go (-f -)"
Task: "Mettre à jour la recette Helm de schema.yaml et du README"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 (corpus réel des trois versions) et Phase 2
2. Phase 3 (US1 : refus d'admission, la cause de bruit la plus forte du TP08)
3. **STOP and VALIDATE** : quickstart T1 à T4 sur kind, sous bash et zsh
4. Livrable seul si nécessaire (PR intermédiaire possible, gates vertes)

### Incremental Delivery

1. Setup + Foundational → corpus et lectures prêts
2. US1 → admission (MVP)
3. US2 → placement et `drain` (TP09)
4. US3 → Jobs, CronJobs, DaemonSets, StatefulSets, cron (TP04, TP05)
5. US4 → métriques et HPA (TP06, TP13)
6. US5 → `check -f -` (TP12), indépendante du cluster : peut être avancée à tout moment
7. US6 → dernières entrées et relevé d'une formation (mesure de SC-004)

### Notes

- [P] = fichiers différents, sans dépendance
- Chaque histoire est testable seule sur kind ; US5 sans cluster
- Toute version d'outil ou d'image est revérifiée au moment de l'ajout (CLAUDE.md, principe III)
- Commit après chaque tâche ou groupe logique, validations enchaînées par `&&`
