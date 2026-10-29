# Contrat CLI (extensions) : `kubectl telemach`

**Feature** : `004-training-feedback` | **Date** : 2026-10-29

Ce contrat complète ceux des features [001](../../001-kubectl-error-guidance/contracts/cli.md)
et [002](../../002-silent-pitfalls/contracts/cli.md), qui restent en vigueur pour tout ce qui
n'est pas cité ici. Options globales, sortie sur stderr, codes de sortie et règle « aucune
option avant `telemach` » inchangés.

---

## `diagnose` (FR-005, FR-009 à FR-012, FR-014, FR-016)

| Argument | Types acceptés | Comportement |
|----------|----------------|--------------|
| *(aucun)* | — | comme en 002, plus les Jobs en échec du namespace (limite globale de 10 objets) |
| `pod/<nom>` | pod | inchangé ; un pod `Pending` liste désormais les nœuds par cause (F6) |
| `deployment/<nom>` | Deployment | + maillon `admission` (`ReplicaFailure`, F4) |
| `job/<nom>`, `jobs/<nom>` | Job | condition, pods, puis pod en difficulté le plus récent |
| `cronjob/<nom>`, `cj/<nom>` | CronJob | planification, historique, dernier Job (puis diagnostic du Job) |
| `daemonset/<nom>`, `ds/<nom>` | DaemonSet | nœuds couverts et exclus, rollout |
| `statefulset/<nom>`, `sts/<nom>` | StatefulSet | Service headless, ordre de démarrage, volumes |
| `hpa/<nom>`, `horizontalpodautoscaler/<nom>` | HPA (`autoscaling/v2`) | API de métriques, cible, requests, conditions |
| autre type | — | message listant tous les types pris en charge, code `2` |

Lectures : celles de la 002 plus research.md F1, uniquement.

Exemple (`--color=never`) :

```text
✖ Le pod « web » n'est placé sur aucun nœud : 1 nœud porte un taint qu'il ne tolère pas.
   pod web · namespace demo · Pending

   Nœuds par cause (0/3 disponibles) :
   ✖ 1 nœud    taint dedie=gpu:NoSchedule non toléré (kind-worker)
   ✖ 1 nœud    anti-affinité de pods non satisfaite
   ℹ 1 nœud    taint du plan de contrôle (normal)

📘 Concept — taint and toleration
   Un taint sur un nœud repousse les pods qui ne le tolèrent pas. Une toleration autorise un
   pod à aller sur ce nœud, sans l'y forcer : pour l'y envoyer, il faut aussi un
   nodeSelector ou une affinité.
…
```

## `check` (FR-004, FR-013, FR-018, FR-019 à FR-022)

```text
kubectl telemach check -f <fichier|dossier|-> [-f …] [-R]
```

| Entrée | Comportement |
|--------|--------------|
| `-f -` | lit l'entrée standard (1 Mio au plus) ; positions `<stdin>:ligne:colonne` ; template d'origine si `# Source:` |
| `-f -` deux fois | refus, code `2` |
| `-f -` sur un terminal | message d'usage (`helm template … \| kubectl telemach check -f -`), code `2`, sans attente |
| `-f -` vide | « aucun manifeste », code `2` |
| `-f -` trop gros ou non UTF-8 | problème bloquant, code `1` (comme un fichier) |

Nouveaux contrôles : `invalid-cron` (problème) ; `missing-limits` et `hpa-missing-request`
(informations, sans effet sur le code de sortie, PR #20).

Exemple :

```text
✖ Problème 1/1 — <stdin>:31:13 (document 3, CronJob/sauvegarde, template telemarche/templates/cron.yaml)
   champ « spec.schedule »
   L'expression cron « */5 * * * » a 4 champs ; il en faut 5 : minute, heure, jour du mois,
   mois, jour de la semaine.
```

## `explain` (FR-001 à FR-003, FR-008, FR-013, FR-015, FR-023 à FR-025)

Nouvelles entrées reconnues (identifiants indicatifs, motifs écrits après relevé) :
`admission.quota-exceeded`, `admission.quota-must-specify`, `admission.limitrange`,
`admission.pod-security`, `drain.daemonset-pods`, `drain.pdb-blocked`, `drain.local-storage`,
`metrics.api-unavailable`, `manifest.cron-schedule`, `wait.timeout`,
`rollout.undo-revision-missing`, `runtime.ephemeral-disabled`.

Pour `admission.pod-security` avec `-f <fichier local>` : bloc « 💡 Version corrigée » du
`securityContext` de chaque conteneur cité.

Pour `wait.timeout` : première correction `kubectl telemach diagnose <type>/<nom>` avec la
portée de la commande, si le type est pris en charge par `diagnose`.

## `notice` (usage interne, FR-007)

```text
kubectl-telemach notice --stderr-file <fichier> -- <args kubectl…>
```

Appelée par l'intégration shell après une commande **réussie** dont la sortie d'erreur
contient `would violate PodSecurity`. Affiche une information (ℹ) sur stderr, au plus une fois
par namespace et par 24 h ; code toujours `0` ; aucune lecture du cluster.

## `watch` (usage interne, FR-006, FR-017)

Verbes suivis par l'intégration shell : `apply`, `create`, `run`, `expose`, **`scale`**.
Nouvelles cibles : Deployment visé par `scale` (piège `deployment.replica-failure`), HPA
appliqué (`hpa.target-missing`, `hpa.missing-request`), StatefulSet appliqué
(`statefulset.no-service`). Budget de 12 lectures et délai de 10 s inchangés.

## Intégration shell

- `__telemach_after_success` : ajoute `scale`.
- Après une commande réussie : test sans processus de `would violate PodSecurity` dans le
  fichier temporaire, puis `notice` (F5). Versions bash et zsh ; `TELEMACH_KUBECTL_SHELL`
  incrémentée (D-LL-013 : `last`/`deepen` signalent une intégration périmée).
