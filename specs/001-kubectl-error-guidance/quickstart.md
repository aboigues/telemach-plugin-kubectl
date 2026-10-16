# Guide de validation (Quickstart) : Guidage pédagogique à partir des erreurs kubectl

**Feature** : `001-kubectl-error-guidance` | **Date** : 2026-10-16

Ce guide prouve, de bout en bout, que la feature fonctionne. Il ne décrit pas
l'implémentation : voir [contracts/cli.md](contracts/cli.md) et [data-model.md](data-model.md).

**Règle de sécurité (LL-015, principe III)** : tous les scénarios s'exécutent sur un cluster
**kind jetable** créé pour l'occasion, jamais sur le contexte habituel de la machine.

## Prérequis

- Go **1.27.2** (le `toolchain` du `go.mod` le télécharge si besoin)
- Docker (pour kind) et **kind v0.33.0**
- `kubectl` 1.35, 1.36 ou 1.37 (une autre version fonctionne : les règles 1.37 sont alors
  utilisées et le plugin le signale)
- bash ou zsh

## Installation locale et cluster jetable

```bash
go build -o ~/.local/bin/kubectl-telemach ./cmd/kubectl-telemach   # dossier présent dans le PATH
kind create cluster --name telemach \
  --image kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
kubectl config use-context kind-telemach
kubectl create namespace demo
eval "$(kubectl-telemach shell-init zsh)"      # ou bash
kubectl telemach version                        # le plugin est découvert par kubectl
```

## Scénarios de validation

### V1 — Type de ressource mal orthographié (US1-1, FR-011)

```bash
kubectl get deploymnet -n demo; echo "code: $?"
```

Attendu : message kubectl d'origine, puis explication (concept *resource type*, suggestion
`deployment`, commande corrigée `kubectl get deployment -n demo` qui garde `-n demo`).
`code: 1` : code préservé.

### V2 — Mauvais namespace (US1-2)

```bash
kubectl run web --image=nginxinc/nginx-unprivileged:1.30-alpine -n demo
kubectl get pod web; echo "code: $?"
```

Attendu : concept *namespace*, correction `kubectl get pod web -n demo` et
`kubectl get pods -A`.

### V3 — Cluster injoignable (US1-3)

```bash
kubectl --context inexistant get pods
docker stop telemach-control-plane; kubectl get pods; docker start telemach-control-plane
```

Attendu : client/serveur API, contexte, kubeconfig ; `kubectl config get-contexts` proposé ;
aucun contenu du kubeconfig affiché.

### V4 — Accès refusé (US1-4)

```bash
kubectl get pods -n demo --as=system:serviceaccount:demo:default
```

Attendu : RBAC (sujet, verbe, ressource, namespace), `kubectl auth can-i list pods -n demo
--as=…` proposé ; aucune suggestion de `cluster-admin`.

### V5 — Option avant le nom du plugin (R2, LL-014)

```bash
kubectl -n demo telemach diagnose
```

Attendu : kubectl refuse ; TELEMACH explique la règle et propose
`kubectl telemach diagnose -n demo`.

### V6 — Erreur non reconnue (US1-5, FR-008)

```bash
kubectl-telemach explain --exit-code 1 --stderr-file <(echo "error: something totally new") -- get pods
```

Attendu : « Erreur non reconnue », version de kubectl, lien vers la documentation, aucune
cause inventée.

### V7 — Tabulation dans un manifeste (US2-1)

Fichier `testdata/manifests/tab-indent/deploy.yaml` :

```bash
kubectl apply -f testdata/manifests/tab-indent/deploy.yaml -n demo
```

Attendu : fichier et ligne exacts, règle « pas de tabulation en YAML », version corrigée.

### V8 — Champ inconnu dans le 3e document (US2-2, US2-4, FR-015)

```bash
kubectl apply -f testdata/manifests/unknown-field-multidoc/all.yaml -n demo
```

Attendu : `document 3, Deployment/web`, ligne **dans le fichier** (pas dans le document),
suggestion `imagePullPolicy`.

### V9 — Selector et labels incohérents (US2-3)

```bash
kubectl apply -f testdata/manifests/selector-mismatch/deploy.yaml -n demo
```

Attendu : les deux lignes concernées, explication du lien selector/labels, correction.

### V10 — Secret en clair (US2-5, SC-006)

```bash
kubectl telemach check -f testdata/manifests/plaintext-secret/
```

Attendu : valeurs `••••••` partout (sortie et `~/.local/state/telemach-kubectl/last.json`),
mauvaise pratique signalée, alternative `secretKeyRef`.

### V11 — `check` avant application (US2-6, FR-014a, FR-017, FR-017a)

```bash
kubectl telemach check -f testdata/manifests/valid/; echo "code: $?"
kubectl telemach check; echo "code: $?"
```

Attendu : « Règles Kubernetes 1.37 » puis « ✔ Aucun problème détecté », `code: 0` ; sans
`-f`, explication de l'option et `code: 2`, aucun fichier lu. Avec un kubectl hors plage
(ex. v1.32), la mention « règles 1.37 utilisées » apparaît. Le cluster n'est pas contacté
(vérifiable en arrêtant le cluster).

### V12 — Approfondir (US3)

```bash
kubectl telemach deepen
kubectl telemach deepen --exercise
```

Attendu : concept détaillé, exemple exécutable, exercice dans un namespace à créer soi-même.

### V13 — Diagnostiquer des pods en échec (US4-1 à US4-6, FR-022 à FR-026)

```bash
kubectl apply -n demo -f testdata/pods/scenarios/    # ImagePullBackOff, CrashLoopBackOff,
                                                     # Pending (requests énormes),
                                                     # CreateContainerConfigError, OOMKilled
sleep 30
kubectl telemach diagnose -n demo
kubectl telemach diagnose pod/crash -n demo
kubectl telemach diagnose -n demo --as=system:serviceaccount:demo:default   # lecture refusée
kubectl telemach diagnose -n default
```

Attendu : explication complète du pod le plus récent puis liste courte des autres ; cause
correcte pour chaque état (SC-009, < 5 s) ; journal ≤ 20 lignes masquées ; aucune valeur de
`Secret` ni d'`env` ; diagnostic marqué partiel quand une lecture est refusée ; « aucun pod en
difficulté » pour `default`. État du namespace identique avant et après
(`kubectl get all,events -n demo -o json` comparé, SC-008).

### V14 — Vérification différée (US4-7 à US4-9, FR-028 à FR-031, SC-010)

```bash
kubectl apply -n demo -f testdata/pods/scenarios/bad-image.yaml
# attendre ~10 s, puis appuyer sur Entrée
kubectl apply -n demo -f testdata/manifests/valid/deploy.yaml   # pod sain : rien
TELEMACH_WATCH=0 kubectl apply -n demo -f testdata/pods/scenarios/bad-image-2.yaml   # rien
```

Attendu : à l'invite suivante, une seule ligne « ⚠ Le pod … est en ImagePullBackOff :
kubectl telemach diagnose pod/… -n demo » ; jamais au milieu d'une saisie ; pas de message de
tâche de fond ; invite non retardée.

### V15 — Alias `kubectl` et `k` (R4, FR-001a)

```bash
zsh -ic 'alias kubectl="command kubectl"; alias k=kubectl; eval "$(kubectl-telemach shell-init zsh)"; k get pdos'
zsh -ic 'unalias k 2>/dev/null; eval "$(kubectl-telemach shell-init zsh)"; alias k || echo "k non créé"'
```

Attendu : l'erreur de `k get pdos` est expliquée avec une correction `k get pods` ; sans alias
`k` préalable, « k non créé ».

### V16 — Garanties de l'intégration shell (FR-021)

Exécuter `tests/shell/` sous bash et zsh : code de sortie préservé, stdout intact
(`kubectl get ns -o name | wc -l` identique avec et sans intégration), fichier temporaire
absent après Ctrl-C, aucune sortie dupliquée (LL-002), `TELEMACH_DISABLE=1` sans effet.

### V17 — Répétition et `last` (FR-009a, FR-009b)

```bash
kubectl get pdos; kubectl get pdos; clear; kubectl telemach last
```

Attendu : seconde explication courte avec renvoi à `last` ; `last` réaffiche l'explication
complète.

## Nettoyage

```bash
kind delete cluster --name telemach
```
