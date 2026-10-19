# Implementation Plan: Diagnostiquer ce qui réussit sans marcher

**Branch**: `002-silent-pitfalls` | **Date**: 2026-10-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-silent-pitfalls/spec.md`

## Summary

TELEMACH sait expliquer les commandes kubectl en échec et les pods qui ne démarrent pas
(feature 001). Cette feature traite les **pièges silencieux** : commandes réussies dont
l'application ne fonctionne pas. `kubectl telemach diagnose` accepte désormais `service/`,
`deployment/` (état du rollout), `ingress/` et `pvc/` ; il suit le chemin du trafic maillon par
maillon et fait de chaque maillon cassé une leçon. La vérification différée signale, à
l'invite suivante, les pièges non transitoires des objets que l'apprenant vient de créer. Le
catalogue gagne les erreurs courantes de `exec`, `logs`, `port-forward`, `rollout status` et
les champs immuables, ainsi que l'anti-pattern de la suppression forcée.

Choix techniques (voir [research.md](research.md)) :
- **Lectures** : liste fermée étendue (S1), types dédiés sans valeur sensible, quatre
  annotations publiques seulement (S2), dates de ConfigMap lues sans leurs valeurs (S3) ;
- **Diagnostics** : chemin du trafic ordonné (S4), état du rollout (S5), Ingress (S6), PVC
  (S7), lectures parallèles sans argument (S8) ;
- **Vérification différée** : cibles sans lecture, pièges non transitoires seulement,
  `expose` couvert, budget de 12 lectures (S9) ;
- **Capture** : stderr capté aussi pour `exec`, `attach`, `debug`, `port-forward`, après
  validation en pseudo-terminal (S10) ;
- **Corpus** : relevé réel sur kind, reproductible par un workflow manuel (S12).

## Technical Context

**Language/Version**: Go 1.27.2 (inchangé ; à revérifier au moment de l'implémentation)

**Primary Dependencies**: inchangées (cobra, yaml.v3, lipgloss, colorprofile, chroma, x/term) ;
**aucune nouvelle dépendance**.

**Storage**: `last.json` étendu (`Object`), nouveau `hints.json` (`0600`, empreintes d'images,
purge à 24 h) dans `$XDG_STATE_HOME/telemach-kubectl/` ; signaux `pending-<pid>` inchangés.

**Testing**: `go test` + golden files ; nouveau corpus `testdata/objects/` relevé sur kind
(SC-001, SC-002) ; corpus d'erreurs `testdata/errors/` complété (SC-003) ; contrôles
`TestGoldenFrench`, `TestGoldenNoSecret`, `TestCorpusLanguageAndSecrets` étendus (SC-006) ;
faux kubectl journalisant ses appels (SC-005) ; tests en pseudo-terminal bash et zsh (S10,
SC-007) ; intégration kind `tests/integration/pitfalls_test.go` (W1 à W14).

**Target Platform**: Linux et macOS (bash, zsh), WSL2 compris (inchangé).

**Project Type**: CLI, plugin kubectl (inchangé).

**Performance Goals**: `diagnose <objet>` < 5 s au p95 (SC-004), y compris sans argument grâce
aux lectures parallèles ; invite retardée < 50 ms au p95 (SC-007) ; chacun mesuré par un test
(D-LL-012).

**Constraints**: lectures limitées à la feature 001 + research.md S1 ; jamais d'objet Secret,
de valeur de ConfigMap, de valeur d'`env` ni d'annotation hors des quatre clés de S2 ; aucune
écriture, aucun `exec`, aucun `port-forward` lancé par le plugin ; `explain`, `check`, `last`,
`deepen` toujours hors ligne.

**Scale/Scope**: 4 nouveaux types d'objets ; ~19 pièges (data-model.md) et ~8 nouvelles
erreurs ; corpus d'au moins un cas réel par piège et par version (1.35, 1.36, 1.37).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principe | Vérification | Statut |
|----------|--------------|--------|
| **I. Vocation pédagogique** | Chaque piège est une entrée du catalogue avec concept, cause, corrections et approfondissement (test de cohérence `Pitfall.ID` ↔ catalogue). Le chemin du trafic enseigne labels → selector → endpoints → ports. Cas normaux (PVC `WaitForFirstConsumer`, Service sans selector) présentés comme tels. Anti-pattern de suppression forcée signalé (FR-028). Aucune cause affirmée sans lecture (port non déclaré, lecture refusée). L'API dépréciée Endpoints n'est plus enseignée (FR-032). | ✅ |
| **II. Langue** | Leçons, messages, documentation et commentaires en français ; code en anglais ; golden files contrôlés par `TestGoldenFrench`. | ✅ |
| **III. Sécurité** | Liste fermée étendue et testée (`allowlist_test.go`) ; aucun Secret lu ; dates de ConfigMap sans leurs valeurs (S3) ; annotations limitées à quatre clés publiques (S2) ; masquage de tout objet lu ; tests sur kind jetable uniquement ; `os/exec` avec liste d'arguments. Manifestes de `testdata/objects/scenarios/` contrôlés par `manifest-lint` (non-root, `resources`), sauf le défaut volontaire de chaque scénario déclaré dans `expected.yaml` ; l'image sans tag de W6 reste un avertissement non bloquant, conformément à la constitution. | ✅ |
| **IV. Gates GitHub Actions** | Gates existantes inchangées et obligatoires ; `integration` rejoue W1 à W14 sur les trois versions. Nouveau workflow **manuel** `corpus.yml` (relevé du corpus) : actions épinglées par SHA, `permissions: contents: read`, aucun secret, cluster kind créé par le workflow ; il n'est pas une gate de fusion. `manifest-lint` étendu à `testdata/objects/scenarios/`. | ✅ |
| **V. LESSON-LEARNED** | Relu avant `/speckit-specify` et `/speckit-plan` ; LL-001 à LL-004 et leçons héritées appliquées (research.md S14), notamment LL-004 (quickstart rejoué tel quel, dossier de scénarios sans fichier d'attentes). | ✅ |

**Re-check post-design** : le modèle de données ne contient aucune valeur sensible (types
dédiés, `KnownAnnotations`, `ConfigMapDates`) ; le contrat de `diagnose` et de `watch` limite les
lectures à S1 ; aucune commande de test ou de correction n'est exécutée ; les pièges
transitoires ne sont jamais signalés par `watch` (SC-002). Aucun écart à justifier hors du
tableau ci-dessous.

## Project Structure

### Documentation (this feature)

```text
specs/002-silent-pitfalls/
├── plan.md              # Ce fichier
├── research.md          # Phase 0 — décisions S1 à S14
├── data-model.md        # Phase 1 — types lus, instantanés, pièges
├── quickstart.md        # Phase 1 — scénarios de validation W1 à W14
├── contracts/
│   └── cli.md           # Phase 1 — extensions de diagnose, watch, explain
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — pas encore créé)
```

### Source Code (repository root)

Fichiers ajoutés (+) ou modifiés (~) par rapport à la feature 001 :

```text
cmd/kubectl-telemach/
├── diagnose.go           ~ types service/deployment/ingress/pvc, noms ambigus, sans argument
├── watch.go              ~ cibles d'objets, pièges non transitoires, budget de lectures
└── last.go               ~ réaffichage d'un ObjectSnapshot

internal/
├── kuberead/
│   ├── read.go           ~ lectures de S1 (et seulement elles)
│   ├── types.go          ~ Pod, Deployment étendus
│   ├── objects.go        + Service, EndpointSlice, ReplicaSet, Ingress, IngressClass, PVC,
│   │                       StorageClass, ConfigMapDates
│   ├── annotations.go    + KnownAnnotations (quatre clés, S2)
│   └── allowlist_test.go ~ liste fermée étendue
├── pitfall/              + diagnostics d'objets : service.go (chemin du trafic), rollout.go,
│                           ingress.go, pvc.go, config.go, image.go (référence OCI), summary.go
├── workload/
│   └── targets.go        ~ ObjectTarget (Service, Ingress, PVC, expose, images)
├── catalog/data/
│   ├── service.yaml      + leçons des pièges de Service
│   ├── rollout.yaml      + rollout, tag mutable, configuration figée
│   ├── ingress.yaml      + Ingress
│   ├── storage.yaml      + PVC
│   ├── portforward.yaml  + port-forward
│   └── cmdline.yaml, notfound.yaml, conflict.yaml, runtime.yaml, workload.yaml, schema.yaml
│                         ~ nouvelles erreurs, motif DNS, EndpointSlice (FR-032)
├── render/object.go      + RenderObject (maillons, leçon)
├── state/
│   ├── state.go          ~ LastDiagnosis.Object
│   └── hints.go          + HintMemory (hints.json)
└── shellinit/scripts/    ~ capture de stderr des commandes interactives (S10), expose

scripts/
├── collect-errors/       ~ nouveaux cas (exec, logs, port-forward, rollout, immuable)
└── collect-objects/      + relevé des objets réels par piège

testdata/
├── objects/<cas>/        + objets JSON réels masqués + expected.yaml
├── objects/scenarios/    + manifestes seulement (W1 à W13)
├── errors/               ~ nouveaux cas
└── golden/f002/          + rendus de référence (couleur, sans couleur)

tests/
├── integration/pitfalls_test.go  + W1 à W14 (bash et zsh, trois versions)
├── shell/                        ~ pseudo-terminal pour exec -it et port-forward capturés
└── manifests/                    ~ testdata/objects/scenarios/ contrôlé

.github/workflows/
└── corpus.yml            + relevé manuel du corpus (workflow_dispatch)

SECURITY.md, README.md    ~ nouvelles lectures et nouveaux diagnostics
```

**Structure Decision**: un nouveau paquet `internal/pitfall` regroupe les diagnostics d'objets.
Il dépend de `kuberead` (lectures), `workload` (diagnostic des pods, réutilisé pour les
nouveaux pods d'un rollout et les consommateurs d'un PVC) et `catalog`, sans toucher au
diagnostic des pods existant. Toute lecture reste dans `kuberead`, ce qui garde la liste
fermée vérifiable par un seul test.

## Complexity Tracking

| Écart | Pourquoi c'est nécessaire | Alternative plus simple écartée parce que |
|-------|---------------------------|--------------------------------------------|
| Lecture de quatre annotations publiques (exception à FR-018 de la feature 001) | La classe par défaut et la révision d'un ReplicaSet ne sont publiées que par ces annotations (S2) | Ne lire aucune annotation : US2 et US3 deviendraient imprécis ; tout décoder puis filtrer : une erreur de filtre fuiterait `last-applied-configuration` |
| Capture de stderr des commandes interactives (révise R3 de la feature 001) | Les erreurs visées par US5 viennent de `exec -it` et `port-forward` (S10) | Garder R3 : ces erreurs resteraient expliquées par le seul code de sortie, ambigu ; repli prévu si un test en pseudo-terminal échoue |
| Workflow manuel `corpus.yml` | Relevé du corpus reproductible sur les trois versions, y compris sans Docker local (S12) | Relevé local seulement : dépend de la machine du mainteneur et n'est pas reproductible |
