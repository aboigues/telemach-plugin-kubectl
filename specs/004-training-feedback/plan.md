# Implementation Plan: Couvrir les TP d'une vraie formation

**Branch**: `004-training-feedback` | **Date**: 2026-10-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-training-feedback/spec.md`

## Summary

La version 0.1.0, confrontée aux 14 TP de la formation Kubernetes du projet, laisse six
familles de situations sans explication (issues #13 à #17, #19 ; les quatre bugs #10, #11,
#12, #18 sont corrigés à part par la PR #20). Cette feature :

- reconnaît les **refus d'admission** (quota, LimitRange, PodSecurity), y compris vus depuis
  un Deployment (`ReplicaFailure`) et après `scale`, et explique l'avertissement PodSecurity
  d'une commande réussie ;
- détaille le **placement** d'un pod `Pending` cause par cause (taint, anti-affinité, cordon)
  et explique les arrêts de `drain` ;
- étend `diagnose` aux **Jobs, CronJobs, DaemonSets, StatefulSets et HPA**, et valide la
  syntaxe cron hors ligne ;
- explique l'absence de **metrics-server** et la chaîne d'un HPA ;
- accepte **`check -f -`** (rendus Helm, Kustomize, `--dry-run`) avec le template d'origine ;
- complète le catalogue (`wait`, `rollout undo`, `debug`) et outille le **relevé d'une
  formation**.

Choix techniques (voir [research.md](research.md)) : liste fermée étendue de 8 lectures
get/list (F1) dans des types sans valeur sensible (F2) ; leçon d'admission partagée entre la
commande refusée et le diagnostic du Deployment, par le moteur de correspondance existant
(F3, F4) ; avertissement PodSecurity détecté par l'intégration shell sans processus (F5) ;
découpage du message du scheduler (F6) ; analyseur cron interne sans dépendance (F9) ;
relevés sur kind à trois nœuds avant toute écriture de motif (F12, LL-008).

## Technical Context

**Language/Version**: Go 1.27.3 (`toolchain` de go.mod, vérifiée le 2026-10-29)

**Primary Dependencies**: inchangées (cobra, yaml.v3, lipgloss, colorprofile, chroma,
x/term) ; **aucune nouvelle dépendance** (analyseur cron interne, F9).

**Storage**: `hints.json` : nouvelle clé par namespace pour l'avertissement PodSecurity (F5) ;
`last.json` inchangé (`<stdin>` sans chemin absolu).

**Testing**: `go test` + golden files ; corpus `testdata/errors/` (≈ 15 cas, trois versions)
et `testdata/objects/` (Jobs, CronJobs, DaemonSets, StatefulSets, HPA, pods `Pending`)
relevés sur kind à trois nœuds ; `testdata/manifests/` (`invalid-cron`, `helm-rendered`) ;
tests unitaires de `internal/cron` (bornes, noms, macros, prochaine échéance, fuzzing) ;
intégration kind `tests/integration/training_test.go` (T1 à T13) sous bash et zsh ;
pseudo-terminal pour l'intégration shell modifiée (LL-009).

**Target Platform**: Linux et macOS (bash, zsh), WSL2 compris (inchangé).

**Project Type**: CLI, plugin kubectl (inchangé).

**Performance Goals**: `diagnose <objet>` < 5 s au p95 (SC-005) ; invite retardée < 50 ms au
p95 après le test PodSecurity et l'ajout de `scale` (SC-006) ; mesurés hors `-race` (LL-006).

**Constraints**: lectures limitées aux features 001, 002 et research.md F1 ; jamais d'objet
Secret, ni d'adresse de nœud, ni de valeur d'`env` ; aucune écriture ; `explain`, `check`,
`notice`, `last`, `deepen` toujours hors ligne ; un message n'est reconnu qu'après relevé
(LL-008) ; toute vérification partielle de `check` est une information (LL-016).

**Scale/Scope**: 5 nouveaux types de `diagnose` ; ≈ 20 pièges ; ≈ 13 nouvelles entrées
d'erreur ; 4 types de problèmes de manifeste ; 1 sous-commande interne (`notice`) ; 1 mode
du relevé (`-training`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principe | Vérification | Statut |
|----------|--------------|--------|
| **I. Vocation pédagogique** | Chaque piège et chaque erreur ont une entrée complète (concept, cause, corrections, approfondissement). Cas normaux présentés comme tels (taint du plan de contrôle, `ScalingLimited`, CronJob pas encore échu). Anti-patterns signalés : gonfler le quota, retirer le label `enforce`, `drain --force`/`--disable-eviction`, perte de données de `--delete-emptydir-data`. Le piège « une toleration autorise sans forcer » est enseigné. Aucune cause inventée : cause du scheduler inconnue citée telle quelle, métriques personnalisées hors de portée. | ✅ |
| **II. Langue** | Leçons, messages, documentation, commentaires en français ; code en anglais ; golden files contrôlés par `TestGoldenFrench`. | ✅ |
| **III. Sécurité** | Liste fermée étendue de get/list seulement (F1), testée par `allowlist_test.go` ; types décodés sans `env`, sans adresse de nœud (F2) ; aucun droit supplémentaire demandé, lecture refusée → diagnostic partiel ; tests sur kind jetable uniquement. metrics-server installé dans le seul cluster de test, avec `--kubelet-insecure-tls` jamais proposé à l'apprenant. Manifestes de scénarios contrôlés par `manifest-lint`, défauts volontaires déclarés hors du dossier. `notice` ne lit que le fichier temporaire de stderr, déjà masqué. | ✅ |
| **IV. Gates GitHub Actions** | Gates inchangées ; `integration` rejoue T1 à T13 sur les trois versions avec un cluster à trois nœuds (durée et `timeout-minutes` réévalués, LL-011). Le relevé d'une formation (T14) s'ajoute au workflow manuel `corpus.yml` (actions épinglées par SHA, `contents: read`, aucun secret) ; il n'est pas une gate. | ✅ |
| **V. LESSON-LEARNED** | Relu au début de la session (2026-10-29), avant `/speckit-specify` et `/speckit-plan`, y compris LL-014 à LL-017 de la PR #20 ; leçons appliquées dans research.md F15. | ✅ |

**Re-check post-design** : le modèle de données ne contient aucune valeur sensible (types
dédiés, `Node` sans adresses, `PlacementCause` extraite du message déjà masqué) ; le contrat
de `diagnose`, `watch` et `notice` limite les lectures à F1 ; `check -f -` applique la même
taille maximale et le même nettoyage qu'un fichier ; les ajouts à `check` sont des
informations sauf `invalid-cron`, erreur avérée. Seul écart : T14, voir ci-dessous.

## Project Structure

### Documentation (this feature)

```text
specs/004-training-feedback/
├── plan.md              # Ce fichier
├── research.md          # Phase 0 — décisions F1 à F15
├── data-model.md        # Phase 1 — types lus, pièges, causes de placement, cron
├── quickstart.md        # Phase 1 — scénarios de validation T1 à T14
├── contracts/
│   └── cli.md           # Phase 1 — diagnose, check -f -, explain, notice, watch
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — pas encore créé)
```

### Source Code (repository root)

Fichiers ajoutés (+) ou modifiés (~) :

```text
cmd/kubectl-telemach/
├── diagnose.go           ~ types job, cronjob, daemonset, statefulset, hpa ; Jobs sans argument
├── check.go              ~ -f -, terminal, double « - »
├── explain.go            ~ version corrigée PodSecurity (analyse locale)
├── notice.go             + avertissement PodSecurity d'une commande réussie (F5)
└── watch.go              ~ scale, HPA, StatefulSet

internal/
├── kuberead/
│   ├── commands.go       ~ lectures de F1 (et seulement elles)
│   ├── objects.go        ~ Job, CronJob, DaemonSet, StatefulSet, HPA, Node, APIService
│   └── allowlist_test.go ~ liste fermée étendue
├── kubeterms/            ~ alias job, cj, ds, sts, hpa
├── cron/                 + analyseur cron (5 champs, macros, prochaine échéance)
├── workload/select.go    ~ causes de placement (F6)
├── pitfall/              ~ rollout.go (maillon admission) ; + job.go, cronjob.go,
│                           daemonset.go, statefulset.go, hpa.go
├── manifest/             ~ read.go (stdin, # Source:), semantic.go (cron, limits, HPA),
│                           fixes.go (securityContext), issue.go (nouveaux types)
├── catalog/
│   ├── schema.go         ~ catégories node, metrics, batch
│   └── data/             ~ admission.yaml (quotas, LimitRange, PodSecurity) ;
│                           + node.yaml (drain, placement), batch.yaml (Job, CronJob, cron),
│                           metrics.yaml (API, HPA) ; ~ rollout.yaml (undo, wait,
│                           ReplicaFailure), runtime.yaml (debug, --target),
│                           workload.yaml (raisons de placement), schema.yaml (Helm -f -)
├── render/               ~ object.go (nœuds par cause), manifest.go (template d'origine)
└── shellinit/scripts/    ~ scale, test PodSecurity sans processus, version d'intégration

scripts/
├── collect-errors/       ~ nouveaux cas, cluster à trois nœuds, mode -training (F13)
├── collect-objects/      ~ Jobs, CronJobs, DaemonSets, StatefulSets, HPA, pods Pending
└── gen-schemas/          ~ HPA (autoscaling/v2) ajouté aux schémas embarqués

testdata/
├── kind/                 + three-nodes.yaml, metrics-server.yaml (épinglé par digest)
├── errors/               ~ nouveaux cas (+ variantes @1.35, @1.36)
├── objects/<cas>/        ~ nouveaux objets relevés
├── objects/scenarios/    ~ manifestes T1 à T13 seulement
├── manifests/            + invalid-cron/, helm-rendered/
└── golden/f004/          + rendus de référence

tests/integration/
└── training_test.go      + T1 à T13, bash et zsh, trois versions

README.md                 ~ recette helm template … | kubectl telemach check -f -
```

**Structure Decision**: projet Go unique existant ; la feature étend les paquets en place
(`pitfall`, `workload`, `manifest`, `catalog`) et n'ajoute qu'un paquet, `internal/cron`,
isolé pour être testé et fuzzé seul.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| T14 (relevé d'une formation) n'est pas rejoué par la gate `integration` (écart à LL-004 / FR-030) | Il clone un dépôt externe et déploie l'application de chaque TP : long (plusieurs dizaines de minutes) et dépendant d'un contenu hors de ce dépôt | Le rejouer à chaque PR rendrait la fusion dépendante d'un dépôt tiers ; il est rejoué par le workflow manuel `corpus.yml`, avec la commande exacte du guide |
| `debug-ephemeral-disabled` validé par un message relevé en formation, sans relevé kind (écart à LL-008) | Les conteneurs éphémères sont toujours actifs de 1.35 à 1.37 : le message n'est pas reproductible sur kind | Renoncer à l'entrée laisserait sans explication un message réellement vu en formation ; le cas est marqué `source: formation` |
