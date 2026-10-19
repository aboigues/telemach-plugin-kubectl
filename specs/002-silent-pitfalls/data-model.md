# Modèle de données : Diagnostiquer ce qui réussit sans marcher

**Feature** : `002-silent-pitfalls` | **Date** : 2026-10-17

Entités ajoutées ou étendues par rapport au
[modèle de la feature 001](../001-kubectl-error-guidance/data-model.md). Toutes les données
textuelles sont nettoyées (`sanitize`) et masquées (`redact`) avant d'entrer dans ces entités ;
aucune ne contient de valeur de Secret, de ConfigMap, de variable d'environnement ni
d'annotation hors de la liste fermée de research.md S2.

## Types lus du cluster (`internal/kuberead`)

Types JSON dédiés : seuls les champs listés sont décodés.

| Type | Champs décodés |
|------|----------------|
| `Service` | `metadata.{name,namespace,creationTimestamp,labels}`, `spec.{type,selector,ports[].{name,port,targetPort,nodePort,protocol},externalName}` |
| `EndpointSlice` | `metadata.{name,labels}`, `endpoints[].{conditions.{ready,serving,terminating},targetRef.{kind,name}}`, `ports[].{name,port}` |
| `Deployment` (étendu) | feature 001 + `spec.{replicas,paused,progressDeadlineSeconds,template.metadata.labels,template.spec.containers[].{name,image,ports,env (noms et références),envFrom}}`, `status.{observedGeneration,replicas,updatedReplicas,readyReplicas,availableReplicas,conditions[]}`, `metadata.generation` |
| `ReplicaSet` | `metadata.{name,creationTimestamp,labels,ownerReferences}`, annotation de révision (S2), `spec.template.spec.containers[].image`, `status.{replicas,readyReplicas}` |
| `Ingress` | `metadata.{name,namespace}`, annotation `kubernetes.io/ingress.class` (S2), `spec.{ingressClassName,defaultBackend,rules[].{host,http.paths[].{path,pathType,backend.service.{name,port.{number,name}}}}}`, `status.loadBalancer.ingress[].{ip,hostname}` |
| `IngressClass` | `metadata.name`, annotation de classe par défaut (S2), `spec.controller` |
| `PersistentVolumeClaim` | `metadata.{name,namespace,creationTimestamp}`, `spec.{storageClassName (absent, vide ou nom),accessModes,resources.requests.storage}`, `status.phase` |
| `StorageClass` | `metadata.name`, annotation de classe par défaut (S2), `provisioner`, `volumeBindingMode` |
| `ConfigMapDates` | nom, namespace, dates `managedFields[].time` (S3) ; **aucun** autre champ |
| `Pod` (étendu) | feature 001 + `status.startTime`, `spec.volumes[].{name,configMap.name,secret.secretName,persistentVolumeClaim.claimName}`, `spec.containers[].volumeMounts[].{name,subPath}`, `spec.containers[].ports[].{name,containerPort}` |

Règle de validation : le décodage d'annotations passe par un type `KnownAnnotations` qui
n'expose que les quatre clés de S2 ; un test vérifie qu'aucune autre clé n'atteint
l'instantané, le rendu ni `last.json`.

## ObjectSnapshot

Vue masquée d'un objet diagnostiqué et des objets liés (FR-021).

| Champ | Type | Règle |
|-------|------|-------|
| `Kind` | `service` \| `deployment` \| `ingress` \| `pvc` | — |
| `Name`, `Namespace`, `Context` | texte | masqués |
| `Links` | liste de `Link` | ordre du chemin du trafic (S4) ou des vérifications (S5 à S7) |
| `Pitfalls` | liste de `Pitfall` | le premier `Severity=warning` porte la leçon complète |
| `Pods` | liste de `PodSummary` (feature 001) | pods désignés, au plus 10 |
| `Rollout` | `RolloutState` ou vide | Deployment seulement |
| `Failing` | `FailingPod` ou vide | pod en difficulté lié à l'objet : pod le plus récent du nouveau ReplicaSet (Deployment), pod `Pending` qui utilise le PVC (S7) |
| `DeniedReads` | liste de textes | lectures refusées (FR-020) |
| `Others` | liste de `ObjectSummary` | `diagnose` sans argument (FR-015), au plus 10 |

## Link (maillon)

| Champ | Type | Règle |
|-------|------|-------|
| `Step` | `type` \| `selector` \| `readiness` \| `endpoints` \| `ports` \| `class` \| `address` \| `backend` \| `storageclass` \| `binding` \| `consumer` \| `rollout` \| `config` \| `image` | — |
| `State` | `ok` \| `broken` \| `unknown` \| `skipped` | `unknown` si la lecture nécessaire est refusée : jamais `broken` sur une information non lue (FR-020) |
| `Observed` | liste de paires libellé/valeur | ex. selector `app=web`, labels proches `app=frontend` ; masquées |

## Pitfall (piège silencieux)

| Champ | Type | Règle |
|-------|------|-------|
| `ID` | identifiant d'entrée du catalogue (`service.no-matching-pods`, `service.pods-not-ready`, `service.targetport-name`, `service.targetport-number`, `service.ports-undeclared`, `rollout.stalled`, `rollout.paused`, `image.mutable-tag`, `config.env-stale`, `config.secret-env`, `ingress.no-class`, `ingress.unknown-class`, `ingress.no-controller`, `ingress.no-address`, `ingress.backend-missing`, `ingress.backend-port`, `pvc.unknown-class`, `pvc.no-default-class`, `pvc.waiting-first-consumer`) | entrée existante du catalogue (test de cohérence) |
| `Severity` | `warning` \| `info` | `info` : tag mutable, Secret en variable d'environnement, PVC en attente normale |
| `Transient` | booléen | vrai : pods pas encore Ready, rollout dans les délais, PVC en attente d'un premier pod ; **jamais signalé par `watch`** (FR-024) |
| `Captures` | dictionnaire | valeurs insérées dans la leçon (`service`, `selector`, `closest`, `port`, `targetPort`…), masquées |

## RolloutState

| Champ | Type | Règle |
|-------|------|-------|
| `Phase` | `complete` \| `progressing` \| `stalled` \| `paused` | S5 |
| `NewImage`, `OldImage` | textes | images des ReplicaSets nouveau et précédent |
| `NewReady`, `NewDesired`, `OldReady` | entiers | — |
| `Deadline` | durée | `progressDeadlineSeconds` (600 s par défaut) |

## FailingPod

Pod en difficulté lié à un objet, diagnostiqué comme dans la feature 001 (S5, S7).

| Champ | Type | Règle |
|-------|------|-------|
| `Pod` | `PodSnapshot` (feature 001) | masqué |
| `Entry`, `Origin`, `Captures` | textes, dictionnaire | leçon du pod, réaffichée par `last` sans relire le cluster |

Quand l'objet ne porte aucun piège (PVC qui attend un pod impossible à démarrer), la leçon du
pod est la leçon principale du diagnostic, de `last` et de `deepen`.

## ObjectTarget (vérification différée)

Extension de `Target` (feature 001), déduite **sans lecture** de la commande et des manifestes
locaux.

| Champ | Type | Règle |
|-------|------|-------|
| `Kind` | `pod` \| `selector` \| `service` \| `ingress` \| `pvc` | — |
| `Name`, `Namespace` | textes | — |
| `CoveredBySameApply` | booléen | Service dont le selector est satisfait par les labels d'un Pod ou Deployment de la même commande (S9) : pas de signal « aucun pod » |
| `Images` | liste de textes | images des manifestes, pour le tag mutable (FR-023) |

## HintMemory (`hints.json`)

| Champ | Type | Règle |
|-------|------|-------|
| `SchemaVersion` | entier | — |
| `Seen` | dictionnaire empreinte → date | empreinte SHA-256 de la référence d'image ; entrées de plus de 24 h purgées |

Fichier `$XDG_STATE_HOME/telemach-kubectl/hints.json`, `0600`, écrit de façon atomique ;
aucun argument de commande ni nom de fichier n'y est conservé.

## LastDiagnosis (étendu)

Champ ajouté `Object` (instantané `ObjectSnapshot` encodé, déjà masqué) et `Source` ajoute la
valeur `object`. `last` réaffiche l'instantané sans relire le cluster, précédé de sa date
(comme pour un pod, FR-027 de la feature 001).

## Transitions d'état

- **Rollout** : `progressing` → `complete` (tous les pods à jour prêts) ou → `stalled`
  (échéance dépassée) ; `paused` ↔ `progressing` par `rollout pause`/`resume` ; un nouvel
  `apply` repasse en `progressing`.
- **PVC** : `Pending` (attente normale `WaitForFirstConsumer`, `Transient`) → `Bound` dès
  qu'un pod l'utilise ; `Pending` avec une classe inexistante reste bloqué (non transitoire).
- **Maillon** : `unknown` ne devient jamais `broken` sans nouvelle lecture réussie.
