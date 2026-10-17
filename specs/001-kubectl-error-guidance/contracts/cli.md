# Contrat CLI : `kubectl telemach`

**Feature** : `001-kubectl-error-guidance` | **Date** : 2026-10-16

Le binaire `kubectl-telemach` est appelé soit par kubectl (`kubectl telemach <cmd>`), soit
directement (`kubectl-telemach <cmd>`), notamment par l'intégration shell. Les messages destinés
à l'apprenant sont en français ; les noms de commandes et d'options sont en anglais.

**Règle kubectl** : aucune option ne peut précéder `telemach` (`kubectl -n x telemach …` est
refusé par kubectl, R2). Les options se placent après la sous-commande.

## Options globales

| Option | Valeurs | Défaut | Effet |
|--------|---------|--------|-------|
| `--color` | `auto`, `always`, `never` | `auto` | `auto` : couleur si la sortie est un terminal et que `NO_COLOR` n'est pas défini ; `CLICOLOR_FORCE=1` équivaut à `always` |
| `--width` | entier ≥ 40 | largeur du terminal, sinon 80 | Largeur de mise en page |

Sortie : explications sur **stderr** ; seuls `shell-init` et `version` écrivent sur stdout.

---

## `explain` — expliquer une erreur capturée (FR-001 à FR-016)

```text
kubectl-telemach explain --exit-code <n> [--stderr-file <chemin>] [--interactive]
                         [--kubectl-version <v>] [--invoked-as kubectl|k] -- <args kubectl…>
```

| Option | Obligatoire | Description |
|--------|-------------|-------------|
| `--exit-code` | oui | Code de sortie (1 à 255) |
| `--stderr-file` | non | Lu une fois (1 Mio max.), jamais modifié ni supprimé par `explain` |
| `--interactive` | non | stderr non capturé : explication fondée sur le code de sortie |
| `--kubectl-version` | non | Version du client (FR-001b) ; ignorée si mal formée |
| `--invoked-as` | non | `k` si l'apprenant a utilisé l'alias : corrections écrites `k …` |
| `-- <args>` | oui | Arguments d'origine, sans `kubectl` |

Comportement :
- reconnue → résumé, concept, cause, corrections (commande corrigée qui conserve tous les
  autres arguments, contexte et namespace, FR-011, FR-012), anti-pattern éventuel, message
  d'origine atténué ;
- erreur liée à un manifeste local → relecture des fichiers de `-f`, problèmes localisés
  (fichier, document, ligne, colonne, ressource, champ) avec extrait et version corrigée ;
- `-f -`, URL ou `-k` → explication à partir du seul message, et mention que la ligne ne peut
  pas être localisée ;
- non reconnue → bloc « Erreur non reconnue », version de kubectl, lien vers la documentation,
  **sans cause inventée** ;
- même empreinte depuis moins de 10 min → rendu court + renvoi à `last`.

Codes de sortie : `0` dès qu'une explication (même « non reconnue ») est produite ; `2` usage
invalide. La fonction shell ignore ce code.

Exemple (`--color=never`) :

```text
✖ Le type de ressource « deploymnet » n'existe pas.

📘 Concept — resource type
   Chaque objet Kubernetes a un type (pod, deployment, service…). kubectl ne connaît que les
   types publiés par le cluster : « kubectl api-resources » les liste.

🔎 Cause probable
   Faute de frappe : le type le plus proche est « deployment » (nom court : deploy).

💡 Corrections
   1. Corriger le type :  kubectl get deployment -n demo
   2. Lister les types :  kubectl api-resources

── Message kubectl d'origine ──
error: the server doesn't have a resource type "deploymnet"

➜ Pour aller plus loin : kubectl telemach deepen
```

---

## `check` — vérifier des manifestes sans cluster (FR-014, FR-014a, FR-017, FR-017a)

```text
kubectl telemach check -f <fichier|dossier> [-f …] [-R]
```

Pas d'option pour forcer une version de règles (option écartée lors de la clarification Q3).

- Lecture seule des fichiers `.yaml`, `.yml`, `.json` désignés ; `-R` parcourt les
  sous-dossiers ; aucun accès au cluster ni au réseau.
- Version de règles : celle de la mineure du client kubectl (lue par l'intégration shell ou,
  en appel direct, par `TELEMACH_KUBECTL_VERSION`), sinon la plus récente embarquée ; la
  première ligne du résultat l'indique : « Règles Kubernetes 1.37 ».
- Aucun problème → « ✔ Aucun problème détecté dans deploy.yaml (3 documents) ».
- **Sans `-f`** → aucun fichier lu ; message pédagogique (pas de fichier par défaut dans
  Kubernetes, rôle de `-f`, dossier, `-R`, exemple à copier).

Codes de sortie : `0` aucun problème ; `1` au moins un problème ; `2` usage invalide (dont
absence de `-f`) ou fichier introuvable.

Exemple (`--color=never`) :

```text
Règles Kubernetes 1.37
✖ Problème 1/1 — deploy.yaml:14:11 (document 2, Deployment/web)
   champ « spec.template.spec.containers[0].imagePullPolicyy »
   ce champ n'existe pas — tu voulais sans doute écrire « imagePullPolicy »

    13 │       - name: web
   ➤14 │         imagePullPolicyy: IfNotPresent
                  ^
    15 │         image: nginxinc/nginx-unprivileged:1.30-alpine

   💡 Version corrigée
    14 │         imagePullPolicy: IfNotPresent
```

---

## `diagnose` — diagnostiquer les pods en difficulté (FR-022 à FR-027)

```text
kubectl telemach diagnose [pod/<nom> | deployment/<nom> | <nom de pod>] [-n <ns>] [--context <ctx>] [--kubeconfig <chemin>] [--as <identité>]
```

- Sans argument : pods en échec du namespace (R10) ; explication complète du plus récent, puis
  liste courte des autres (au plus 10 : nom, état, commande `diagnose pod/<nom>` à copier).
- Lectures autorisées : **uniquement** celles de R8 (`get pods|pod|deployment|events -o json`,
  `logs --tail=20 [--previous]`), via la commande kubectl de l'apprenant
  (`TELEMACH_KUBECTL`, sinon `kubectl` du `PATH`), avec ses `--context`, `-n`, `--kubeconfig`,
  `--as` (pour vérifier ce qu'une autre identité peut lire), et `--request-timeout=10s`.
- Lecture refusée (RBAC) → diagnostic partiel, lecture refusée nommée ; cluster injoignable →
  explication du catalogue (`connection.*`).
- Aucun pod en difficulté → « ✔ Aucun pod en difficulté dans le namespace demo », avec le
  rappel de `-n`.
- Le résultat devient le dernier diagnostic (`last`, `deepen`).

Codes de sortie : `0` diagnostic produit ou aucun pod en difficulté ; `1` lecture impossible
(cluster injoignable) ; `2` usage invalide ou ressource introuvable (avec noms proches).

---

## `watch` — vérification différée (FR-028 à FR-031, usage interne)

```text
kubectl-telemach watch --shell-pid <pid> -- <args kubectl…>
```

Lancé en arrière-plan par l'intégration shell après une création réussie. Sort aussitôt si la
commande n'est pas concernée (`--dry-run`, `-f -`, URL, `-k`, autre sous-commande). Sinon :
cibles déduites des manifestes locaux ou du nom (aucune lecture), attente de 10 s, lecture du
**seul statut** des pods ciblés, écriture d'un `PendingSignal` si un pod est en échec et que
le shell est vivant. Jamais de sortie sur le terminal ; code de sortie toujours `0`.

---

## `last` — réafficher le dernier diagnostic (FR-009a, FR-027)

```text
kubectl telemach last
```

Rendu complet (jamais la version courte) de `LastDiagnosis` ; manifestes réanalysés dans leur
état actuel ; un diagnostic de pod est réaffiché **sans relire le cluster**, précédé de sa date.
Aucun diagnostic → message expliquant comment en obtenir un. Code `0`.

---

## `deepen` — approfondir (FR-009)

```text
kubectl telemach deepen [--exercise]
```

Concept détaillé et exemple exécutable du dernier diagnostic ; `--exercise` ajoute un exercice
court à faire dans un namespace que l'apprenant crée lui-même. Code `0`.

---

## `shell-init` — intégration shell (R3, R4, R11)

```text
kubectl-telemach shell-init bash|zsh
```

À placer dans le fichier rc : `eval "$(kubectl-telemach shell-init zsh)"`.

Garanties du code généré (vérifiées par les tests) :
1. définit uniquement des fonctions et variables préfixées `__telemach_`, la fonction ou
   l'alias `kubectl` (R4), éventuellement l'alias `k` **s'il existait déjà** comme alias de
   kubectl, et exporte `TELEMACH_KUBECTL` (commande kubectl réelle),
   `TELEMACH_KUBECTL_VERSION` (version du client, si connue) et
   `TELEMACH_KUBECTL_SHELL=<shell>:<version du plugin>` ;
2. stdin et stdout de kubectl ne sont jamais redirigés ;
3. le **code de sortie d'origine** est toujours renvoyé ;
4. le fichier temporaire est créé en `0600` et supprimé dans tous les cas (y compris Ctrl-C) ;
5. `TELEMACH_DISABLE=1` ou binaire absent → comportement identique à kubectl ;
6. commandes interactives ou longues → pas de redirection de stderr, `--interactive` ;
7. version du client lue **une seule fois**, à l'évaluation, par
   `<kubectl réel> version --client -o json` (aucun contact avec le serveur API) ;
8. si `k` n'est pas un alias de kubectl, il n'est ni créé ni modifié ;
9. après une création réussie dans un shell interactif, `watch` est lancé en arrière-plan sans
   message de tâche et sans retarder l'invite (< 50 ms au p95, SC-010) ; `TELEMACH_WATCH=0`
   le désactive seul ;
10. le crochet d'invite affiche puis supprime le fichier de signaux ; s'il n'existe pas, aucun
    processus n'est lancé ; ajout idempotent (rechargement du rc sans doublon) ;
11. comportement identique sous bash et zsh, sans sortie dupliquée (LL-002).

Shell non pris en charge → message en français, code `2`.

---

## `version`

```text
kubectl telemach version
```

Version du plugin, licence (Apache-2.0) et site du projet, puis versions de règles Kubernetes
embarquées (`1.35, 1.36, 1.37`). Code `0`.

---

## Sécurité transverse (FR-018 à FR-021)

- Aucune sous-commande ne modifie un fichier de l'apprenant ni une ressource d'un cluster.
  Exécutions autorisées : `kubectl version --client` (script `shell-init`), les lectures de
  `diagnose` et de `watch` (R8), rien d'autre ; vérifié par un test avec un faux kubectl.
- Corrections **affichées**, jamais exécutées.
- Toute chaîne affichée ou persistée passe par le masquage (R9) et la neutralisation des
  séquences de contrôle.
- Aucun accès réseau en dehors des lectures du cluster faites par la commande kubectl de
  l'apprenant.
