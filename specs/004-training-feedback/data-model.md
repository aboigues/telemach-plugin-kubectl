# Modèle de données (Phase 1) : Couvrir les TP d'une vraie formation

**Feature** : `004-training-feedback` | **Date** : 2026-10-29

Complète les modèles des features [001](../001-kubectl-error-guidance/data-model.md) et
[002](../002-silent-pitfalls/data-model.md), qui restent en vigueur. Toutes les valeurs
textuelles affichées ou enregistrées sont masquées (`internal/redact`) avant usage.

## Types lus du cluster (`internal/kuberead`, research.md F1, F2)

| Type | Champs décodés (et seulement eux) |
|------|-----------------------------------|
| `Job` | `metadata.{name, namespace, creationTimestamp, labels, ownerReferences}` ; `spec.{completions, parallelism, backoffLimit, activeDeadlineSeconds, ttlSecondsAfterFinished, suspend, template}` ; `status.{active, succeeded, failed, startTime, completionTime, conditions[type, status, reason, message]}` |
| `CronJob` | `metadata.{name, creationTimestamp}` ; `spec.{schedule, timeZone, suspend, concurrencyPolicy, startingDeadlineSeconds, successfulJobsHistoryLimit, failedJobsHistoryLimit, jobTemplate.spec.template}` ; `status.{active[name], lastScheduleTime, lastSuccessfulTime}` |
| `DaemonSet` | `metadata.name` ; `spec.{selector, template, updateStrategy.type}` ; `status.{desiredNumberScheduled, currentNumberScheduled, numberReady, updatedNumberScheduled, numberMisscheduled}` |
| `StatefulSet` | `metadata.name` ; `spec.{replicas, serviceName, podManagementPolicy, selector, template, volumeClaimTemplates[metadata.name, spec.storageClassName]}` ; `status.{replicas, readyReplicas, currentReplicas}` |
| `HPA` (`autoscaling/v2`) | `metadata.name` ; `spec.{scaleTargetRef, minReplicas, maxReplicas, metrics[type, resource{name, target{type, averageUtilization, averageValue}}]}` ; `status.{currentReplicas, desiredReplicas, currentMetrics, conditions}` |
| `Node` | `metadata.{name, labels}` ; `spec.{taints[key, value, effect], unschedulable}` ; `status.conditions[type=Ready]` — **jamais** `status.addresses` |
| `APIService` | `metadata.name` ; `status.conditions[type=Available]` |
| `Event` (ReplicaSet) | type existant (`reason`, `message`, `lastTimestamp`, `count`) |
| `PodTemplate` (existant, étendu) | + `spec.tolerations[key, operator, value, effect]`, `spec.affinity` (présence seulement), `containers[].resources.requests` |
| `DeploymentCondition` (existant) | lecture de `type=ReplicaFailure` (`reason`, `message`) |

Validation : un objet sans `metadata.name` est ignoré ; un champ numérique absent prend la
valeur par défaut de Kubernetes (`backoffLimit` 6, `concurrencyPolicy` Allow,
`podManagementPolicy` OrderedReady, `minReplicas` 1).

## Kind (types de `diagnose`, étendu)

`service`, `deployment`, `ingress`, `pvc` (002) + `job`, `cronjob`, `daemonset`,
`statefulset`, `hpa`. Alias dans `internal/kubeterms` (research.md F8).

## Step (maillons, étendu)

| Objet | Maillons, dans l'ordre |
|-------|------------------------|
| Deployment | `rollout`, **`admission`**, `image`, `config` |
| Job | `status`, `pods` |
| CronJob | `schedule`, `history`, `last-job` |
| DaemonSet | `nodes`, `rollout` |
| StatefulSet | `service`, `ordinal`, `volumes` |
| HPA | `metrics-api`, `target`, `requests`, `conditions` |

États inchangés : `ok`, `broken`, `unknown` (lecture refusée ou impossible), `skipped`.

## Pitfall (pièges ajoutés)

Chaque identifiant a exactement une entrée du catalogue (`pitfalls:`), vérifié par le test de
cohérence existant.

| Identifiant | Gravité | Transitoire | Vérification différée |
|-------------|---------|-------------|-----------------------|
| `deployment.replica-failure` | erreur | non | oui (après `apply`, `create`, `scale`) |
| `job.backoff-limit`, `job.deadline` | erreur | non | non |
| `job.pods-gone` | info | non | non |
| `job.running`, `job.complete` | ok | — | non |
| `cronjob.suspended` | info | non | non |
| `cronjob.never-run` | info ou erreur (échéance passée) | non | non |
| `cronjob.forbid-skipped`, `cronjob.last-failed` | erreur | non | non |
| `daemonset.excluded-nodes` | info (control-plane) ou erreur (taint d'un worker) | non | non |
| `daemonset.rollout-stuck` | erreur | oui | non |
| `statefulset.no-service` | erreur | non | oui |
| `statefulset.ordered-wait` | erreur | oui | non |
| `statefulset.pvc-pending` | erreur | oui | non |
| `hpa.metrics-unavailable` | erreur | non | non |
| `hpa.target-missing` | erreur | non | oui |
| `hpa.missing-request` | erreur | non | oui |
| `hpa.limited` | info | non | non |
| `hpa.ok` | ok | — | non |

## PlacementCause (pod `Pending`, research.md F6)

| Champ | Description |
|-------|-------------|
| `Reason` | `UntoleratedTaint`, `PodAntiAffinity`, `PodAffinity`, `NodeUnschedulable`, `InsufficientResources`, `NodeSelectorMismatch`, `UnboundPersistentVolumeClaim`, `Unknown` |
| `Nodes` | nombre de nœuds concernés (extrait du message) |
| `Detail` | taint (`dedie=gpu`), ressource (`cpu`) ou texte d'origine pour `Unknown` |
| `Normal` | vrai pour le taint du control-plane |

`Classification` (existant) reçoit `Causes []PlacementCause` ; `Reason` reste la cause
principale (plus grand nombre de nœuds). Le total des nœuds (`0/N`) est conservé.

## ManifestIssue (types ajoutés)

| Type | Niveau | Origine |
|------|--------|---------|
| `invalid-cron` | problème | `check`, `spec.schedule` d'un CronJob (`internal/cron`) |
| `missing-limits` | information | `check`, conteneur sans `resources.limits` |
| `hpa-missing-request` | information | `check`, HPA et cible dans les fichiers vérifiés |
| `pod-security` | problème | `explain` seulement, après un refus PodSecurity, avec version corrigée |

`Document` reçoit `Source string` (template d'origine d'un rendu Helm, `# Source:`) ;
`File.Path` vaut `<stdin>` pour l'entrée standard.

## CronSchedule (`internal/cron`)

| Champ | Description |
|-------|-------------|
| `Expr` | expression d'origine |
| `Fields` | 5 ensembles de valeurs autorisées (minute, heure, jour du mois, mois, jour de semaine) |
| `Err` | erreur en français : `Field` (nom du champ), `Position` (colonne dans l'expression), `Message` |

Opération : `Next(after time.Time, loc *time.Location) time.Time` (prochaine échéance).

## TrainingFinding (relevé d'une formation, research.md F13)

| Champ | Description |
|-------|-------------|
| `TP`, `Panne` | dossier et fichier de la panne |
| `Command` | commande rejouée (masquée) |
| `Message` | première ligne du message (masquée) |
| `Result` | identifiant de l'entrée reconnue, `unknown` ou `unsupported` |

## HintMemory (`hints.json`, étendu)

Nouvelles clés (empreintes SHA-256, purge à 24 h inchangée) :
`telemach:pod-security:<namespace>` (F5). La clé `telemach:auth-can-i-no` existe depuis la
PR #20.

## LastDiagnosis (inchangé)

`check -f -` enregistre `Files: ["<stdin>"]` sans chemin absolu ; `deepen` ne relit pas
`<stdin>`.
