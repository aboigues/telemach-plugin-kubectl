# Guide de validation (Quickstart) : Couvrir les TP d'une vraie formation

**Feature** : `004-training-feedback` | **Date** : 2026-10-29

Ce guide prouve, de bout en bout, que la feature fonctionne. Il ne décrit pas
l'implémentation : voir [contracts/cli.md](contracts/cli.md) et [data-model.md](data-model.md).

**Règles** :
- tous les scénarios s'exécutent sur un cluster **kind jetable à trois nœuds** (D-LL-015),
  jamais sur le contexte habituel de la machine ;
- chaque scénario est rejoué tel quel par `tests/integration/training_test.go` sur les trois
  images kind, sous bash et zsh (LL-004, FR-030), chacun dans un namespace neuf (LL-008) ;
- `testdata/objects/scenarios/` ne contient **que** des manifestes ; les défauts volontaires
  sont déclarés dans `testdata/objects/scenarios.expected.yaml` (LL-005) ;
- les attentes s'appuient sur les relevés des trois versions, jamais sur un message supposé
  (LL-008).

## Prérequis et installation

Ceux de la [feature 002](../002-silent-pitfalls/quickstart.md), puis :

```bash
go install sigs.k8s.io/kind@v0.33.0
go build -o ~/.local/bin/kubectl-telemach ./cmd/kubectl-telemach
kind create cluster --name telemach --config testdata/kind/three-nodes.yaml \
  --image kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
kubectl config use-context kind-telemach
eval "$(kubectl-telemach shell-init zsh)"      # ou bash
```

## Scénarios

### T1 — Quota dépassé et quota qui impose les limites (US1-1, US1-2)

```bash
kubectl create namespace t1
kubectl apply -n t1 -f testdata/objects/scenarios/quota.yaml
kubectl run gros -n t1 --image=nginxinc/nginx-unprivileged:1.30-alpine \
  --overrides='{"spec":{"containers":[{"name":"gros","image":"nginxinc/nginx-unprivileged:1.30-alpine","resources":{"limits":{"cpu":"2","memory":"64Mi"}}}]}}'
kubectl run nu -n t1 --image=nginxinc/nginx-unprivileged:1.30-alpine
```

Attendu : première commande → `admission.quota-exceeded` (quota, `limits.cpu`, demandé 2,
limite 1), `kubectl describe resourcequota -n t1` proposé, anti-pattern « gonfler le quota » ;
seconde → `admission.quota-must-specify` (`limits.cpu`, conteneur `nu`).

### T2 — LimitRange (US1)

```bash
kubectl create namespace t2
kubectl apply -n t2 -f testdata/objects/scenarios/limitrange.yaml
kubectl apply -n t2 -f testdata/objects/scenarios/limitrange-pod.yaml
```

Attendu : `admission.limitrange` (maximum `cpu` par conteneur, valeur et limite demandée),
`kubectl describe limitrange -n t2` proposé.

### T3 — PodSecurity `restricted`, refus et avertissement (US1-3, US1-4)

```bash
kubectl create namespace t3
kubectl label namespace t3 pod-security.kubernetes.io/enforce=restricted
kubectl apply -n t3 -f testdata/objects/scenarios/pss-violation.yaml
kubectl create namespace t3w
kubectl label namespace t3w pod-security.kubernetes.io/warn=restricted
kubectl apply -n t3w -f testdata/objects/scenarios/pss-violation.yaml
```

Attendu : refus → `admission.pod-security` (niveau `restricted`, champs à corriger), bloc
« 💡 Version corrigée » sur `pss-violation.yaml`, anti-pattern « retirer le label
enforce » ; dans `t3w`, la commande réussit, l'avertissement de kubectl est suivi d'une
information ℹ ; rejouée une seconde fois, l'information ne réapparaît pas.

### T4 — Deployment « scaled » sans pods (US1-5, US1-6)

```bash
kubectl create namespace t4
kubectl apply -n t4 -f testdata/objects/scenarios/quota.yaml
kubectl apply -n t4 -f testdata/objects/scenarios/quota-deployment.yaml
kubectl scale deployment api -n t4 --replicas=3
sleep 12; true                                  # invite suivante : vérification différée
kubectl telemach diagnose deployment/api -n t4
```

Attendu : à l'invite, un signal « pods refusés » ; le diagnostic montre le maillon
`admission` en échec avec la leçon `admission.quota-exceeded`.

### T5 — Pod `Pending` : taint, anti-affinité, cordon (US2-1 à US2-3)

```bash
kubectl create namespace t5
kubectl taint node telemach-worker dedie=gpu:NoSchedule
kubectl cordon telemach-worker2
kubectl apply -n t5 -f testdata/objects/scenarios/pending-placement.yaml
kubectl telemach diagnose pod/place -n t5
kubectl uncordon telemach-worker2
kubectl taint node telemach-worker dedie=gpu:NoSchedule-
```

Attendu : nœuds par cause (taint `dedie=gpu` non toléré, nœud non planifiable, taint du plan
de contrôle présenté comme normal), rappel « une toleration autorise, elle ne force pas »,
`kubectl uncordon` proposé.

### T6 — `drain` arrêté (US2-4 à US2-6)

```bash
kubectl create namespace t6
kubectl label node telemach-worker formation=drain
kubectl apply -n t6 -f testdata/objects/scenarios/drain-pdb.yaml
kubectl wait -n t6 --for=condition=Available deployment/web deployment/cache --timeout=90s
kubectl drain telemach-worker
kubectl drain telemach-worker --ignore-daemonsets
kubectl drain telemach-worker --ignore-daemonsets --delete-emptydir-data --timeout=20s
kubectl uncordon telemach-worker
kubectl label node telemach-worker formation-
```

Attendu (ordre des refus confirmé par le relevé) : première commande → `drain.daemonset-pods`
(`--ignore-daemonsets`) ; deuxième → `drain.local-storage` (`--delete-emptydir-data` présenté
avec la perte des données) ; troisième → `drain.pdb-blocked` (`kubectl get pdb -A`,
anti-patterns `--disable-eviction` et `--force`).

### T7 — Job en échec, CronJob suspendu (US3-1, US3-4, US3-8)

```bash
kubectl create namespace t7
kubectl apply -n t7 -f testdata/objects/scenarios/job-fails.yaml
kubectl wait -n t7 --for=condition=Failed job/calc --timeout=120s
kubectl telemach diagnose job/calc -n t7
kubectl apply -n t7 -f testdata/objects/scenarios/cronjob-suspended.yaml
kubectl telemach diagnose cj/sauvegarde -n t7
kubectl telemach diagnose -n t7
```

Attendu : `job.backoff-limit` puis le code de sortie du dernier pod ; `cronjob.suspended` ;
sans argument, le Job `calc` est listé.

### T8 — DaemonSet et StatefulSet (US3-5, US3-6)

```bash
kubectl create namespace t8
kubectl apply -n t8 -f testdata/objects/scenarios/daemonset.yaml
kubectl rollout status -n t8 daemonset/agent --timeout=90s
kubectl telemach diagnose ds/agent -n t8
kubectl apply -n t8 -f testdata/objects/scenarios/statefulset-no-service.yaml
kubectl telemach diagnose sts/base -n t8
```

Attendu : `daemonset.excluded-nodes` (plan de contrôle, normal) ; `statefulset.no-service`
(`serviceName` `base` absent, Service headless expliqué).

### T9 — Syntaxe cron (US3-7)

```bash
kubectl telemach check -f testdata/manifests/invalid-cron/cronjob.yaml; echo "code: $?"
kubectl create namespace t9
kubectl apply -n t9 -f testdata/manifests/invalid-cron/cronjob.yaml
```

Attendu : `check` → problème `invalid-cron` sur `spec.schedule`, `code: 1` ; `apply` →
`manifest.cron-schedule` avec les 5 champs et un exemple commenté.

### T10 — Métriques indisponibles et HPA sans request (US4-1, US4-2, US4-5)

```bash
kubectl create namespace t10
kubectl top pods -n t10
kubectl apply -n t10 -f testdata/objects/scenarios/hpa-no-request.yaml
sleep 12; true
kubectl telemach diagnose hpa/web -n t10
```

Attendu : `metrics.api-unavailable` ; à l'invite, signal « cible sans request CPU » ; le
diagnostic nomme d'abord l'API de métriques absente, puis le conteneur sans request.

### T11 — HPA sain plafonné (US4-4)

```bash
kubectl apply -f testdata/kind/metrics-server.yaml
kubectl wait -n kube-system --for=condition=Available deployment/metrics-server --timeout=180s
kubectl create namespace t11
kubectl apply -n t11 -f testdata/objects/scenarios/hpa-limited.yaml
kubectl wait -n t11 --for=jsonpath='{.status.currentMetrics}' hpa/web --timeout=180s
kubectl telemach diagnose hpa/web -n t11
```

Attendu : tous les maillons `ok` ; `ScalingLimited` expliqué comme un plafond (`hpa.limited`),
sans erreur.

### T12 — Vérifier un rendu sur l'entrée standard (US5)

```bash
kubectl telemach check -f - < testdata/manifests/helm-rendered/rendu.yaml; echo "code: $?"
kubectl create deployment web --image=nginxinc/nginx-unprivileged:1.30-alpine --dry-run=client -o yaml | kubectl telemach check -f -; echo "code: $?"
kubectl telemach check -f - < /dev/null; echo "code: $?"
```

Attendu : premier rendu → problème cité `<stdin>:ligne:colonne` avec
`template telemarche/templates/api.yaml`, `code: 1` ; deuxième → informations seulement
(`missing-limits`), `code: 0` ; troisième → « aucun manifeste », `code: 2`.

### T13 — `wait`, `rollout undo`, `debug` (US6-1 à US6-3)

```bash
kubectl create namespace t13
kubectl run web -n t13 --image=nginxinc/nginx-unprivileged:0.0.0
kubectl wait -n t13 --for=condition=Ready pod/web --timeout=10s
kubectl create deployment api -n t13 --image=nginxinc/nginx-unprivileged:1.30-alpine
kubectl rollout undo deployment/api -n t13 --to-revision=5
```

Attendu : `wait.timeout` avec `kubectl telemach diagnose pod/web -n t13` en première
correction ; `rollout.undo-revision-missing` avec `kubectl rollout history`. Le cas `debug`
(conteneurs éphémères désactivés) n'est pas reproductible sur kind : il est couvert par le
corpus (`debug-ephemeral-disabled`, source formation, research.md F12).

### T14 — Relevé d'une formation (US6-5)

```bash
git clone --depth 1 https://github.com/aboigues/kubernetes-formation /tmp/formation
TELEMACH_KIND_IMAGE=kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5 \
  go run -tags integration ./scripts/collect-errors -training /tmp/formation -out /tmp/releve.md
```

Attendu : un tableau par TP ; aucune ligne « non reconnue » pour les familles couvertes par
cette feature (SC-004). Ce scénario est un outil du mainteneur : il n'est pas rejoué par la
gate `integration` (dépôt externe), mais par le workflow manuel `corpus.yml`.
