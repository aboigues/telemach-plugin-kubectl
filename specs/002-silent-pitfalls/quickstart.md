# Guide de validation (Quickstart) : Diagnostiquer ce qui réussit sans marcher

**Feature** : `002-silent-pitfalls` | **Date** : 2026-10-17

Ce guide prouve, de bout en bout, que la feature fonctionne. Il ne décrit pas
l'implémentation : voir [contracts/cli.md](contracts/cli.md) et [data-model.md](data-model.md).

**Règles** :
- tous les scénarios s'exécutent sur un cluster **kind jetable** (D-LL-015), jamais sur le
  contexte habituel de la machine ;
- chaque scénario est rejoué tel quel par `tests/integration/pitfalls_test.go` sur les trois
  images kind, sous bash et zsh (LL-004, FR-030) ;
- `testdata/objects/scenarios/` ne contient **que** des manifestes : les attentes des tests
  sont dans `testdata/objects/<cas>/expected.yaml`, et les défauts volontaires des scénarios
  dans `testdata/objects/scenarios.expected.yaml`, hors du dossier (LL-004, LL-005).

## Prérequis et installation

Ceux de la [feature 001](../001-kubectl-error-guidance/quickstart.md) (Go, kind, kubectl 1.35
à 1.37, bash ou zsh), puis :

```bash
go build -o ~/.local/bin/kubectl-telemach ./cmd/kubectl-telemach
kind create cluster --name telemach \
  --image kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
kubectl config use-context kind-telemach
kubectl create namespace demo
eval "$(kubectl-telemach shell-init zsh)"      # ou bash
```

## Scénarios

### W1 — Service dont le selector ne désigne aucun pod (US1-1)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/selector-typo.yaml
kubectl wait -n demo --for=condition=Available deployment/frontend --timeout=90s
kubectl telemach diagnose service/web -n demo; echo "code: $?"
```

Attendu : maillon `selector` en échec, labels proches `app=frontend`, concept *label
selector*, `kubectl get pods --show-labels -n demo` proposé, aucune commande `get endpoints`
(FR-032) ; `code: 0`.

### W2 — Pods désignés mais pas Ready (US1-2)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/not-ready.yaml
kubectl wait -n demo --for=jsonpath='{.status.replicas}'=1 deployment/api --timeout=90s
kubectl telemach diagnose service/api -n demo
```

Attendu : maillon `readiness` en échec, pods nommés, `kubectl telemach diagnose pod/<nom>`
proposé.

### W3 — `targetPort` incohérent (US1-3)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/targetport-name.yaml
kubectl wait -n demo --for=condition=Available deployment/shop --timeout=90s
kubectl telemach diagnose service/shop -n demo
kubectl apply -n demo -f testdata/objects/scenarios/targetport-number.yaml
kubectl wait -n demo --for=condition=Available deployment/cart --timeout=90s
kubectl telemach diagnose service/cart -n demo
kubectl apply -n demo -f testdata/objects/scenarios/targetport-undeclared.yaml
kubectl wait -n demo --for=condition=Available deployment/blog --timeout=90s
kubectl telemach diagnose service/blog -n demo
```

Attendu : `targetPort` nommé `http` introuvable (cause affirmée) ; pour `cart`, `targetPort`
8081 absent des ports déclarés (8080), avec la mention « déclaration facultative » ; pour
`blog`, conteneur sans port déclaré : **aucune cause affirmée**, démarche de vérification
expliquée.

### W4 — Service sans selector et Service sain (US1-4, US1-5)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/no-selector.yaml
kubectl telemach diagnose service/external -n demo
kubectl apply -n demo -f testdata/objects/scenarios/healthy.yaml
kubectl wait -n demo --for=condition=Available deployment/hello --timeout=90s
kubectl telemach diagnose service/hello -n demo
```

Attendu : `external` présenté comme cas particulier ; `hello` confirmé sain, noms DNS
`hello.demo` et `hello.demo.svc.cluster.local`, commande `kubectl port-forward service/hello`.

### W5 — Rollout bloqué (US2-1, US2-2)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/rollout-stalled.yaml
kubectl wait -n demo --for=condition=Available deployment/stalled --timeout=90s
kubectl set image -n demo deployment/stalled app=nginxinc/nginx-unprivileged:0.0.0-inexistant
kubectl wait -n demo --for=condition=Progressing=False deployment/stalled --timeout=120s
kubectl telemach diagnose deployment/stalled -n demo
```

Attendu : rollout bloqué (échéance de 30 s dépassée), ancienne version encore servie, nouveau
pod en `ImagePullBackOff` expliqué avec les causes de la feature 001,
`kubectl rollout undo deployment/stalled -n demo` proposé comme choix de l'apprenant.

### W6 — Tag mutable (US2-3, US4-2)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/mutable-tag.yaml
# appuyer sur Entrée après ~10 s
kubectl wait -n demo --for=condition=Available deployment/latest-app --timeout=90s
kubectl telemach diagnose deployment/latest-app -n demo
```

Attendu : à l'invite suivante, une ligne ℹ sur l'image sans tag ; réappliquer le même fichier
ne la réaffiche pas dans les 24 h ; `diagnose` donne l'information non bloquante.

### W7 — ConfigMap modifiée après le démarrage des pods (US2-5, US2-6)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/config-env.yaml
kubectl wait -n demo --for=condition=Available deployment/conf --timeout=90s
sleep 2   # les dates de Kubernetes sont à la seconde : la modification doit suivre le démarrage
kubectl apply -n demo -f testdata/objects/scenarios/config-env-update.yaml
kubectl telemach diagnose deployment/conf -n demo
```

Attendu : ConfigMap `conf-settings` modifiée après le démarrage des pods, variables figées,
`kubectl rollout restart deployment/conf -n demo` proposé ; information sur le Secret
référencé, **sans** affirmer qu'il a changé ; ni la valeur `prod` ni aucune valeur de Secret
dans la sortie ou `last.json`.

### W8 — Ingress sans classe, classe inconnue, backend absent (US3-1 à US3-3)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/ingress-no-class.yaml
kubectl telemach diagnose ingress/web -n demo
kubectl apply -n demo -f testdata/objects/scenarios/ingress-backend-missing.yaml
kubectl telemach diagnose ingress/shop -n demo
```

Attendu : aucune classe d'Ingress dans le cluster → « aucun contrôleur d'Ingress installé »,
adresse vide expliquée ; `shop` vise un Service inexistant, règle et Service nommés.

### W9 — PVC (US3-4, US3-5)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/pvc-unknown-class.yaml
kubectl telemach diagnose pvc/fast-data -n demo
kubectl apply -n demo -f testdata/objects/scenarios/pvc-waiting.yaml
kubectl telemach diagnose pvc/data -n demo
```

Attendu : `fast-data` demande une classe inexistante, classes disponibles listées avec
`standard (par défaut)` ; `data` (`WaitForFirstConsumer`, aucun pod) présenté comme **normal**.

### W10 — Diagnostic sans argument et nom ambigu (FR-015, FR-016)

```bash
kubectl telemach diagnose -n demo
kubectl telemach diagnose hello -n demo; echo "code: $?"
```

Attendu : pods en échec expliqués, puis liste courte des autres pièges des scénarios
précédents ; `hello` (Deployment et Service) → liste des deux objets, `code: 2`.

### W11 — Signal après création (US4-1, US4-3, US4-4)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/watch-selector.yaml     # Service seul, selector sans pod
kubectl apply -n demo -f testdata/objects/scenarios/watch-together.yaml     # Service + Deployment ensemble
TELEMACH_WATCH=0 kubectl apply -n demo -f testdata/objects/scenarios/watch-selector-2.yaml
kubectl expose deployment hello -n demo --name=hello-typo --port=80 --target-port=web
# attendre ~10 s, puis appuyer sur Entrée
```

Attendu : une ligne ⚠ pour `watch-selector`, une pour `hello-typo` (`targetPort` nommé
introuvable), rien pour `watch-together` (pods pas encore créés : transitoire) ni pour
`watch-selector-2` ; invite non retardée.

### W12 — Nouvelles erreurs du catalogue (US5-1 à US5-5)

```bash
kubectl exec -it -n demo deploy/hello -c helo -- sh; echo "code: $?"
kubectl logs -n demo pod/two-containers; echo "code: $?"
kubectl port-forward -n demo service/hello 8080:80 & sleep 2; kubectl port-forward -n demo service/hello 8080:80; kill %1
kubectl apply -n demo -f testdata/objects/scenarios/rollout-stalled.yaml
kubectl rollout status -n demo deployment/stalled --timeout=90s
kubectl set image -n demo deployment/stalled app=nginxinc/nginx-unprivileged:0.0.0-inexistant
kubectl rollout status -n demo deployment/stalled --timeout=120s; echo "code: $?"   # échéance de 30 s dépassée
kubectl apply -n demo -f testdata/objects/scenarios/immutable-selector.yaml; echo "code: $?"
```

Attendu : conteneur inconnu (liste des conteneurs proposée), conteneur à préciser (une
commande par conteneur), port local occupé, échéance de rollout (`diagnose deployment/…`
proposé), champ immuable (deux voies, aucune suppression exécutée) ; codes d'origine
préservés ; la session `exec -it` d'un nom correct reste utilisable (tests en
pseudo-terminal, S10).

### W13 — DNS d'un autre namespace et anti-pattern `Terminating` (US5-6, US5-7)

```bash
kubectl apply -n demo -f testdata/objects/scenarios/dns-short-name.yaml
kubectl wait -n demo --for=jsonpath='{.status.phase}'=Failed pod/dns-client --timeout=60s
kubectl telemach diagnose pod/dns-client -n demo
```

Attendu : le pod appelle le Service `kubernetes` (namespace `default`) par son nom court
depuis `demo` ; la portée d'un nom court est expliquée et `kubernetes.default` ainsi que
`kubernetes.default.svc.cluster.local` sont proposés. L'anti-pattern de suppression forcée est vérifié par les golden files et par un test
du catalogue : toute leçon dont une correction mène à `--force --grace-period=0` ou à la
suppression des finalizers porte le bloc ⚠ Anti-pattern.

### W14 — Sécurité et état du cluster (FR-017 à FR-020, SC-005, SC-006)

Rejoué par les tests de W1 à W13 : état du namespace (`kubectl get all,configmaps,pvc,ingress,endpointslices -n demo -o json`,
hors champs volatils) identique avant et après chaque `diagnose` et `watch` ; journal des
appels d'un faux kubectl limité à la liste de research.md S1 ; aucune valeur du corpus de
secrets ni de ConfigMap dans les sorties et `last.json` ; lecture refusée via
`--as=system:serviceaccount:demo:default` → diagnostic partiel.

## Nettoyage

```bash
kind delete cluster --name telemach
```
