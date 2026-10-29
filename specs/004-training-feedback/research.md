# Recherche (Phase 0) : Couvrir les TP d'une vraie formation

**Feature** : `004-training-feedback` | **Date** : 2026-10-29

Décisions F1 à F15. Chaque décision : choix, justification, alternatives écartées. Les
messages cités viennent des issues #13 à #19 (relevés pendant la formation, kubectl 1.37) ;
**aucun motif n'est écrit avant le relevé sur kind des trois versions** (LL-008, F12).

## Sources

- Issues #13, #14, #15, #16, #17, #19 (confrontation de la 0.1.0 aux 14 TP de
  kubernetes-formation, 2026-10-29) et les corrections déjà livrées par la PR #20 (#10, #11,
  #12, #18 ; LL-014 à LL-017).
- Dépôt de formation : `tpNN-*/pannes/*.yaml` sont des **manifestes** à appliquer par-dessus
  l'application déployée par le TP (`apps/`), et non des commandes : un Job qui échoue, un HPA
  sans request, etc. (TP03, TP04, TP06, TP07, TP13).
- Code existant : `internal/workload/select.go` (raisons d'un pod `Pending`),
  `internal/pitfall/` (diagnostics d'objets, maillons), `internal/kuberead/commands.go`
  (liste fermée), `cmd/kubectl-telemach/watch.go` (vérification différée),
  `internal/manifest/` (check, niveau information ajouté par la PR #20).

## F1. Lectures ajoutées à la liste fermée (FR-027, FR-028)

**Décision** : ajouter à `internal/kuberead/commands.go`, et seulement :

| Lecture | Usage |
|---------|-------|
| `get job <nom> -o json`, `get jobs -o json` | diagnose job/, Jobs d'un CronJob, sans argument |
| `get cronjob <nom> -o json` | diagnose cronjob/ |
| `get daemonset <nom> -o json` | diagnose daemonset/ |
| `get statefulset <nom> -o json` | diagnose statefulset/, cible d'un HPA |
| `get hpa <nom> -o json` | diagnose hpa/, vérification différée d'un HPA |
| `get nodes -o json` | taints, `unschedulable` (pod `Pending`, DaemonSet) |
| `get apiservice v1beta1.metrics.k8s.io -o json` | disponibilité de l'API de métriques |
| `get events --field-selector involvedObject.kind=ReplicaSet,involvedObject.name=<rs> -o json` | refus `FailedCreate` sans message de condition |

Les lectures de pods, de PVC, de Services et de Deployments existent déjà. Chaque nouvelle
lecture est décodée dans un type dédié qui ne retient que les champs utiles (F2).

**Justification** : get/list seulement, sur les types que l'apprenant manipule dans les TP ;
nœuds et APIService sont des objets du cluster, lisibles par la plupart des rôles de
formation, et leur refus rend le diagnostic partiel (FR-028), sans jamais demander de droits.

**Alternatives écartées** : `kubectl describe` (texte non stable, contient des valeurs
d'`env`) ; lire les PodDisruptionBudgets pour `drain` (le message suffit, F7) ; lire les
ResourceQuota/LimitRange (le message contient déjà les valeurs ; la correction propose
`describe` à l'apprenant).

## F2. Types décodés sans valeur sensible (principe III)

**Décision** : `Job`, `CronJob`, `DaemonSet`, `StatefulSet`, `HPA`, `Node`, `APIService`
décodés dans des types qui ne contiennent ni `env`, ni annotations (hors liste de S2 de la
002), ni adresses : pour `Node`, seuls `metadata.name`, `metadata.labels` (rôle
control-plane), `spec.taints`, `spec.unschedulable`, `status.conditions[type=Ready]` ; jamais
`status.addresses`. Les modèles de pods de Job, CronJob, DaemonSet et StatefulSet réutilisent
le décodage de modèle de pod existant (ressources, images, securityContext, tolérances).

**Justification** : le principe « un champ non décodé ne peut pas fuir » de la 002 ; les
adresses internes des nœuds n'ont aucune valeur pédagogique ici.

## F3. Refus d'admission (FR-001 à FR-003)

**Décision** : nouvelles entrées dans `internal/catalog/data/admission.yaml` (créé par la
PR #20), avec groupes nommés :

| Entrée | Forme du message (à relever, F12) | Groupes |
|--------|-----------------------------------|---------|
| `admission.quota-exceeded` | `exceeded quota: <q>, requested: <r>=<v>, used: <r>=<u>, limited: <r>=<l>` | `quota`, `resource`, `requested`, `used`, `limited` |
| `admission.quota-must-specify` | `failed quota: <q>: must specify <r> for: <conteneurs>` | `quota`, `resources`, `containers` |
| `admission.limitrange` | `(maximum|minimum) <r> usage per (Container|Pod) is <v>, but (limit|request) is <w>` | `bound`, `resource`, `scope`, `value`, `actual` |
| `admission.pod-security` | `violates PodSecurity "<niveau>:<version>": <liste>` | `level`, `version`, `violations` |

Ces messages arrivent après « is forbidden: » : `admission.yaml` est lu avant `auth.yaml`, ce
qui évite tout recouvrement avec `auth.forbidden` (dont le motif exige `User "…" cannot`).
Pour PodSecurity, la liste des violations est découpée (`allowPrivilegeEscalation != false`,
`unrestricted capabilities`, `runAsNonRoot != true`, `seccompProfile`) pour produire la
**version corrigée** (FR-003) : quand la commande désigne un fichier local, l'analyse locale
d'`explain` ajoute un problème `pod-security` par conteneur fautif, avec les lignes à ajouter
(`fixes.go`). `check` n'applique **pas** ces règles d'office : le niveau dépend du
namespace, inconnu hors ligne.

**Information « sans limits » (FR-004)** : `check` ajoute une information `missing-limits`
(niveau ℹ de la PR #20) par conteneur sans `resources.limits` ; elle ne change pas le code de
sortie. Les rendus de référence existants qui l'afficheraient sont mis à jour.

**Alternatives écartées** : appliquer le profil `restricted` dans `check` (faux positifs sur
des namespaces `baseline`) ; une seule entrée pour tous les quotas (perd la leçon « must
specify », cause n° 1 au TP08).

## F4. Refus vu depuis un Deployment (FR-005, FR-006)

**Décision** : `diagnose deployment/<nom>` lit la condition `ReplicaFailure` du Deployment ;
son message est **celui du refus** (`pods "api-…" is forbidden: exceeded quota: …`). Il est
passé au moteur de correspondance du catalogue comme une sortie d'erreur : la leçon est la
même que pour la commande refusée. Si la condition est absente mais que le ReplicaSet courant
a moins de pods que voulu, l'événement `FailedCreate` le plus récent du ReplicaSet est lu
(F1). Nouveau maillon `admission` du diagnostic de Deployment, entre `rollout` et `image`.

La vérification différée couvre `scale` : l'intégration shell ajoute `scale` à la liste des
verbes suivis (`__telemach_after_success`) ; `watch` lit le Deployment (une lecture) et
signale `ReplicaFailure`. Délai inchangé (10 s) : la condition apparaît en moins d'une
seconde après le refus.

## F5. Avertissement PodSecurity d'une commande réussie (FR-007)

**Décision** : quand la commande réussit, l'intégration shell teste la présence de
`would violate PodSecurity` dans le fichier temporaire de stderr **sans lancer de processus**
(`[[ $(<fichier) == *…* ]]`, intégré à bash et zsh) ; si elle est présente, elle appelle
`kubectl-telemach notice --stderr-file … -- <args>` (sous-commande cachée), qui affiche une
information (ℹ) et retient `hints.json` (clé : namespace), au plus une fois par 24 h.

**Justification** : le message n'est pas une erreur (code 0) ; l'information arrive juste
après l'avertissement de kubectl, au moment où l'apprenant le lit. Le test sans processus
préserve SC-006 (50 ms au p95).

**Alternatives écartées** : passer par `watch` (arrive à l'invite suivante, après 10 s, alors
que le fichier temporaire est déjà supprimé) ; `grep` (un processus de plus à chaque
commande réussie).

## F6. Causes de placement d'un pod `Pending` (FR-009, FR-010)

**Décision** : `workload.Classify` découpe le message du scheduler
`0/N nodes are available: <cause>, <cause>. preemption: …` en causes élémentaires
(« <n> node(s) <texte> » ou « <n> Insufficient <r> ») ; la partie `preemption:` est ignorée.
Chaque cause reçoit une raison :

| Texte | Raison | Capture |
|-------|--------|---------|
| `had untolerated taint {<clé>: <valeur>}` | `UntoleratedTaint` | `taint` |
| `didn't match pod anti-affinity rules` | `PodAntiAffinity` | — |
| `didn't match pod affinity rules` | `PodAffinity` | — |
| `were unschedulable` | `NodeUnschedulable` | — |
| `Insufficient <r>` | `InsufficientResources` (existant) | `resource` |
| `didn't match Pod's node affinity/selector` | `NodeSelectorMismatch` (existant) | — |
| PVC non lié | `UnboundPersistentVolumeClaim` (existant) | — |

La raison principale est la cause qui concerne le plus de nœuds (à égalité : l'ordre du
tableau) ; toutes les causes sont rendues dans une liste « nœuds par cause ». Une cause
inconnue est citée telle quelle, sans explication (« aucune cause inventée »). Le taint
`node-role.kubernetes.io/control-plane` est signalé comme normal : le plan de contrôle
n'accueille pas les charges de travail.

Lecture des nœuds (F1) seulement pour `UntoleratedTaint` et `NodeUnschedulable` : nommer les
nœuds concernés et leurs taints. Refusée : diagnostic partiel, taint extrait du message.

Le rappel « une toleration autorise, elle ne force pas » figure dans la cause et
l'approfondissement de `pod.untolerated-taint`.

## F7. `drain` (FR-008)

**Décision** : trois entrées `drain.*` dans un nouveau fichier `node.yaml` (catégorie
`node`), sans lecture du cluster : `cannot delete DaemonSet-managed Pods`, `Cannot evict pod
as it would violate the pod's disruption budget`, `cannot delete Pods with local storage`.
`--disable-eviction` et `--force` sont des anti-patterns ; `--delete-emptydir-data` est une
correction non sûre (`safe: false`) qui avertit de la perte des données. `drain` affiche
« continuing command… » puis réessaie : l'explication ne s'affiche qu'à la sortie de la
commande (inchangé).

## F8. Job, CronJob, DaemonSet, StatefulSet (FR-011, FR-012, FR-014)

**Décision** : quatre diagnostiqueurs dans `internal/pitfall/` (`job.go`, `cronjob.go`,
`daemonset.go`, `statefulset.go`), sur le modèle de `rollout.go` : instantané, maillons,
pièges (catalogue `workload.yaml` complété, ou nouveau `batch.yaml`), puis pod en difficulté
le plus récent avec les causes de la feature 001.

| Objet | Maillons | Pièges |
|-------|----------|--------|
| Job | `status`, `pods` | `job.backoff-limit` (condition `Failed`, raison `BackoffLimitExceeded`), `job.deadline` (`DeadlineExceeded`), `job.pods-gone` (terminé, aucun pod : pods supprimés à la main) ; un Job **introuvable**
dont le nom correspond à une erreur récente rappelle que `ttlSecondsAfterFinished` supprime le
Job **et** ses pods (correction du 2026-10-29 : le TTL ne laisse pas un Job sans pods), `job.running`, `job.complete` |
| CronJob | `schedule`, `history`, `last-job` | `cronjob.suspended`, `cronjob.never-run` (aucune exécution depuis la création, en distinguant « première échéance pas encore arrivée »), `cronjob.forbid-skipped` (`concurrencyPolicy: Forbid` et Job actif plus ancien que l'intervalle), `cronjob.last-failed` (enchaîne sur le Job) |
| DaemonSet | `nodes`, `rollout` | `daemonset.excluded-nodes` (voulus < nœuds : nœuds exclus et leurs taints, control-plane présenté comme normal), `daemonset.rollout-stuck` |
| StatefulSet | `service`, `ordinal`, `volumes` | `statefulset.no-service` (Service de `serviceName` absent), `statefulset.ordered-wait` (`OrderedReady` et pod `-k` pas prêt), `statefulset.pvc-pending` (PVC `<modèle>-<nom>-<k>` `Pending`, leçon PVC de la 002) |

Les Jobs d'un CronJob sont trouvés par `ownerReferences` dans `get jobs` ; les pods d'un Job
par le label `batch.kubernetes.io/job-name` (et `job-name`, plus ancien). Le prochain
déclenchement d'un CronJob est calculé par le module cron de F9 (fuseau `spec.timeZone` ou
UTC par défaut du contrôleur).

`diagnose` sans argument (FR-014) ajoute `get jobs` aux lectures parallèles existantes et
liste les Jobs dont la condition `Failed` est vraie, dans la limite globale de 10 objets.

Alias (kubeterms) : `job`, `jobs`, `cronjob`, `cj`, `daemonset`, `ds`, `statefulset`, `sts`,
`hpa`, `horizontalpodautoscaler`.

## F9. Syntaxe cron (FR-013)

**Décision** : petit analyseur interne `internal/cron` (sans dépendance) qui suit le format
accepté par le contrôleur CronJob (format standard à 5 champs de robfig/cron v3 : minutes,
heures, jour du mois, mois, jour de la semaine ; listes, intervalles, pas, noms `JAN`/`MON`,
`?` ; macros `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`), avec un message par
défaut : nombre de champs, champ hors limites, nom inconnu, préfixe `TZ=`/`CRON_TZ=` refusé
(renvoi vers `spec.timeZone`). Il sert à `check` (problème `invalid-cron`, avec la ligne et
un exemple corrigé) et au calcul de la prochaine échéance (F8). L'entrée
`manifest.cron-schedule` reconnaît le refus du serveur (`spec.schedule: Invalid value: …`) et
est rangée avant les entrées génériques « is invalid ».

**Alternatives écartées** : dépendre de robfig/cron (dépendance nouvelle pour ~150 lignes, à
justifier et suivre) ; une simple expression régulière (ne vérifie pas les bornes).

## F10. Métriques et HPA (FR-015 à FR-018)

**Décision** :
- Entrée `metrics.api-unavailable` (`Metrics API not available`, `metrics not available
  yet`) dans un nouveau `metrics.yaml` (catégorie `metrics`).
- `internal/pitfall/hpa.go`, maillons dans l'ordre : `metrics-api` (APIService
  `v1beta1.metrics.k8s.io` présent et condition `Available=True` ; refus de lecture →
  inconnu), `target` (`scaleTargetRef` : Deployment ou StatefulSet lus ; autre kind →
  « non vérifié »), `requests` (chaque conteneur du modèle de pod déclare une request pour
  chaque ressource visée par une métrique `Resource` en `Utilization`), `conditions`
  (`AbleToScale`, `ScalingActive` ; `ScalingLimited=True` avec `TooManyReplicas` présenté
  comme un plafond, pas une panne). Métriques `Pods`, `Object`, `External` : « hors de
  portée », sans cause.
- Vérification différée : un HPA appliqué devient une cible d'objet (`ObjectTargets`) ;
  `watch` lit le HPA puis sa cible (2 lectures, budget de 12 inchangé) et signale le seul
  piège non transitoire `hpa.missing-request`.
- `check` hors ligne : quand un HPA et sa cible figurent dans les fichiers vérifiés, une
  information `hpa-missing-request` par conteneur sans request.

**Scénarios de test** : metrics-server est installé dans le cluster kind **jetable** pour les
seuls scénarios d'HPA sain (version et digest vérifiés à l'implémentation ; v0.9.1 au
2026-10-29). Son option `--kubelet-insecure-tls`, nécessaire avec kind, est réservée au
cluster de test et n'apparaît dans aucune correction proposée.

## F11. `check -f -` (FR-019 à FR-022)

**Décision** : `manifest.Expand` reconnaît `-` ; `manifest.ReadStdin` lit au plus
`MaxFileSize` (1 Mio) + 1 octet pour détecter le dépassement, applique les mêmes contrôles
que `Read` (UTF-8, nettoyage), et produit un `File` de chemin `<stdin>`. Un second `-f -` est
refusé (kubectl : « can only use - once »). Si l'entrée standard est un terminal
(`term.IsTerminal`, dépendance existante), message d'usage immédiat, code 2. Vide : problème
`no-manifest-file`, code 2. Pour chaque document, le dernier commentaire `# Source: <chemin>`
situé avant sa première clé est retenu (`Document.Source`) ; la position rendue devient
`<stdin>:24:3 (document 2, Deployment/api, template telemarche/templates/api.yaml)`.
`last`/`deepen` ne relisent jamais `<stdin>` (rien n'est enregistré comme fichier).

La recette du README et les informations `helm-template`/`helm-values` de la PR #20
deviennent `helm template demo ./chart | kubectl telemach check -f -`.

## F12. Corpus et relevés (FR-029, LL-008)

**Décision** :
- Nouveaux cas `scripts/collect-errors` (quota, LimitRange, PodSecurity, drain ×3, top,
  wait ×4, rollout undo, CronJob invalide) relevés sur kind 1.35, 1.36, 1.37 (variantes
  `@1.35`, `@1.36` si le message diffère) ; le cluster de relevé est créé à **plusieurs
  nœuds** (un control-plane, deux workers) pour `drain`, taints et anti-affinité.
- Nouveaux cas `scripts/collect-objects` pour les objets de F6, F8, F10.
- `debug` sans conteneurs éphémères n'est pas reproductible sur kind 1.35 à 1.37 (toujours
  activés) : cas `debug-ephemeral-disabled` tiré du message de l'issue #19, marqué
  `source: formation` dans `expected.yaml` (nouveau champ facultatif, ignoré par le test).
  Même règle pour `apply-webhook-unreachable` (PR #20).
- kind v0.33.0 (dernière version, déjà épinglée par la CI) s'installe en local par
  `go install sigs.k8s.io/kind@v0.33.0` ; Docker est disponible dans WSL.

## F13. Relevé d'une formation (FR-026)

**Décision** : mode `-training <dossier>` de `scripts/collect-errors` (outil du
mainteneur, étiquette `integration`, cluster kind jetable). Pour chaque `tpNN-*/pannes/*.yaml`
du dossier : namespace neuf, application des prérequis déclarés par le TP (fichier
`telemach-releve.yaml` facultatif à la racine du TP : manifestes à appliquer avant,
attente), application de la panne, attente de 60 s, puis `kubectl telemach diagnose` sans
argument et `explain` des commandes en échec. Sortie : tableau Markdown (panne, commande,
message masqué, entrée reconnue ou « non reconnue » / « pas pris en charge »). Le relevé
n'écrit que dans son cluster jetable et dans le fichier de sortie.

**Justification** : les pannes sont des manifestes, pas des commandes ; leur effet se lit
avec `diagnose`. SC-004 se mesure avec ce relevé.

### Résultat du relevé (2026-10-29, T067, SC-004)

Run `corpus.yml` 40943021545 (kind 1.37, `training_repo=aboigues/kubernetes-formation`, sans
fichier `telemach-releve.yaml` dans les TP) : 10 pannes, 11 lignes, **0 message non reconnu,
0 type non pris en charge**.

| TP | Panne | Ce que TELEMACH a expliqué |
|----|-------|----------------------------|
| tp03 | panne-1, panne-2, panne-3 | image introuvable, redémarrage en boucle (code 1), mémoire insuffisante |
| tp03 | panne-4 | rien signalé au bout de 60 s (sonde de disponibilité : dépend de l'application du TP) |
| tp04 | import | rien signalé au bout de 60 s (Job qui réessaie plusieurs minutes, API du TP absente) |
| tp06 | export, inventaire, racine | OOMKilled, CPU insuffisant, configuration de conteneur invalide |
| tp07 | promo | refus d'application reconnu (`manifest.no-kind-match` : Gateway API non installé) |
| tp13 | stock | redémarrage en boucle (code 1) |

Limites : sans les prérequis des TP (application `apps/`, Gateway API, Traefik), trois pannes ne
se manifestent pas pendant le relevé ; SC-004 est donc vérifié sur les seules familles qui se
sont produites. Pour aller plus loin, le dépôt de formation peut déclarer ses prérequis dans
`tpNN-*/telemach-releve.yaml` (`apply`, `wait`). La première version du rapport classait « rien
signalé » comme « reconnue » : l'outil distingue désormais ce cas.

## F14. `wait`, `rollout undo`, `debug` (FR-023 à FR-025)

**Décision** : entrées `wait.timeout` (catégorie `rollout`, ou nouvelle catégorie `wait`)
avec motif `timed out waiting for the condition on (?P<restype>[a-z.]+)/(?P<objname>…)` ;
la première correction est `{{.kubectl}} telemach diagnose {{.diag_target}}{{if .scope}}
{{.scope}}{{end}}`, où `diag_target` est calculé par un enrichisseur (`suggest.go`) : type
pris en charge par `diagnose` (pod, deployment, statefulset, daemonset, job, service, pvc,
ingress, hpa), sinon la correction est `kubectl describe`. `--for=delete` : leçon sur les
finalizers (anti-pattern de la 002 réutilisé). `rollout.undo-revision-missing` et
`runtime.ephemeral-disabled` ; le `deepen` de `runtime.exec-not-found` mentionne
`kubectl debug -it <pod> --image=busybox:1.38 --target=<conteneur>`.

## F15. Leçons appliquées (principe V)

| Leçon | Application |
|-------|-------------|
| LL-004, LL-005 | Chaque scénario W du quickstart est rejoué tel quel par `tests/integration/training_test.go` ; `testdata/objects/scenarios/` ne reçoit que des manifestes |
| LL-006, D-LL-012 | SC-005 et SC-006 mesurés par des tests hors `-race`, p95 sur 100 échantillons |
| LL-008 | Aucun motif avant relevé des trois versions (F12) |
| LL-009 | Le test sans processus de F5 est vérifié en pseudo-terminal sous bash et zsh |
| LL-014 | Entrées d'admission rangées avant `auth.yaml` et `connection.yaml` ; test d'ordre |
| LL-015 | Nouveaux types décodés : `TestSchemaSensitiveFieldsReviewed` couvre les schémas embarqués de Job, CronJob, DaemonSet, StatefulSet (et HPA ajouté aux schémas) |
| LL-016 | Toute vérification partielle de `check` (limits, HPA, cron inconnu) est une information |
| LL-017 | Codes de sortie de `drain`, `wait`, `top`, `debug` relus dans le code de kubectl avant d'écrire les attentes |
| D-LL-002, D-LL-003 | Modification de l'intégration shell (F4, F5) testée sous bash et zsh, stdout et stderr |

## Versions

| Outil | Version | Vérifiée le |
|-------|---------|-------------|
| Go | 1.27.3 (`toolchain` de go.mod) | 2026-10-29 |
| kind | v0.33.0 (dernière publiée) | 2026-10-29 |
| metrics-server (cluster de test seulement) | v0.9.1, `sha256:9b0ba1926e1d3089673f45461c3c1416109dea33b3ba3434048defd380df329b` | 2026-10-29 (T001) |
| kubectl (CI, relevés) | v1.35.10, v1.36.6, v1.37.2 (correctifs sortis depuis v1.35.9, v1.36.5, v1.37.1) | 2026-10-29 (T001) |
| Images kind | celles de la CI (v1.35.8, v1.36.4, v1.37.0, par digest) | inchangées |
| Nouvelles dépendances Go | aucune | — |
