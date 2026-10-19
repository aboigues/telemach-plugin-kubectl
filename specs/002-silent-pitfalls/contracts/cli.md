# Contrat CLI (extensions) : `kubectl telemach`

**Feature** : `002-silent-pitfalls` | **Date** : 2026-10-17

Ce contrat complète [celui de la feature 001](../../001-kubectl-error-guidance/contracts/cli.md),
qui reste en vigueur pour tout ce qui n'est pas cité ici. Options globales, sortie sur stderr
et règle « aucune option avant `telemach` » inchangées.

---

## `diagnose` (FR-001 à FR-021)

```text
kubectl telemach diagnose [<type>/<nom> | <nom>] [-n <ns>] [--context <ctx>] [--kubeconfig <chemin>] [--as <identité>]
```

| Argument | Types acceptés | Comportement |
|----------|----------------|--------------|
| *(aucun)* | — | pods en échec du namespace (feature 001), puis liste des autres pièges : Services sans pod prêt, rollouts bloqués, Ingress sans classe prise en charge, PVC en attente anormale (au plus 10 au total, avec la commande à copier) |
| `pod/<nom>`, `po/<nom>` | pod | inchangé (feature 001) |
| `service/<nom>`, `svc/<nom>` | Service | chemin du trafic (research.md S4) |
| `deployment/<nom>`, `deploy/<nom>` | Deployment | état du rollout, tag d'image, configuration figée, puis pod en échec du nouveau ReplicaSet (S5, S3) |
| `ingress/<nom>`, `ing/<nom>` | Ingress | classe, adresse, règles et Services visés (S6) |
| `pvc/<nom>`, `persistentvolumeclaim/<nom>` | PVC | classe, classe par défaut, mode de liaison, pod consommateur (S7) |
| `<nom>` | pod, Service, Deployment | un seul objet trouvé : diagnostic direct ; plusieurs : liste des objets et des commandes à copier, code `2` ; aucun : noms proches, code `2` |
| autre type | — | message listant les types pris en charge, code `2` |

Lectures : uniquement celles de research.md S1 (liste fermée), par la commande kubectl de
l'apprenant, avec sa portée et `--request-timeout=10s`.

Codes de sortie : inchangés (`0` diagnostic produit ou rien à signaler, `1` cluster
injoignable, `2` usage invalide, objet introuvable ou nom ambigu).

Exemple (`--color=never`) :

```text
✖ Le Service « web » ne désigne aucun pod : son selector ne correspond aux labels d'aucun pod
  du namespace demo.
   service web · namespace demo · ClusterIP · port 80 → targetPort 8080

   ✖ selector    app=web : aucun pod
   · readiness   non vérifié
   · endpoints   aucun
   · ports       non vérifié

📘 Concept — label selector
   Un Service ne connaît pas ses pods par leur nom : il envoie le trafic aux pods dont les
   labels satisfont son selector. Si aucun pod ne porte ces labels, le Service existe mais
   n'a aucun endpoint.

🔎 Cause probable
   Les pods du namespace portent « app=frontend », proche de « app=web » : le selector du
   Service ou les labels du modèle de pod du Deployment ne sont pas écrits pareil.

💡 Corrections
   1. Comparer les labels des pods au selector du Service.
      kubectl get pods --show-labels -n demo
   2. Vérifier les endpoints du Service après correction.
      kubectl get endpointslices -l kubernetes.io/service-name=web -n demo

➜ Pour aller plus loin : kubectl telemach deepen
```

Un objet sain est confirmé sur une ligne (« ✔ Le Service « web » envoie le trafic à 2 pods
prêts ») suivie des noms DNS et d'une commande de test (FR-005).

---

## `watch` (FR-022 à FR-025, usage interne)

```text
kubectl-telemach watch --shell-pid <pid> -- <args kubectl…>
```

Lancé par l'intégration shell après un `apply`, `create`, `run` ou **`expose`** réussi.
Ajouts par rapport à la feature 001 :

- cibles déduites sans lecture : Service, Ingress, PVC des manifestes locaux ; Service de
  `create service` ou `expose` ; images des manifestes ;
- au plus 12 lectures (S1) après l'attente de 10 s ; rien n'est signalé pour un objet dont une
  lecture a échoué ou dépasse ce budget ;
- une ligne par piège **non transitoire**, au format :

  ```text
  ⚠ Le Service web ne désigne aucun pod : kubectl telemach diagnose service/web -n demo
  ⚠ L'Ingress web n'a aucune classe prise en charge : kubectl telemach diagnose ingress/web -n demo
  ⚠ Le PVC data demande la classe « rapide », qui n'existe pas : kubectl telemach diagnose pvc/data -n demo
  ℹ L'image nginx n'a pas de tag explicite : un nouvel apply ne la mettra pas à jour (kubectl telemach deepen)
  ```

- la ligne ℹ du tag mutable : au plus une fois par image et par 24 h (`hints.json`), sans
  lecture du cluster.

Toujours : aucune sortie sur le terminal, code de sortie `0`.

---

## `explain` (FR-026 à FR-028)

Nouvelles entrées reconnues (messages exacts relevés sur kind, research.md S12) :

| Commande | Message (forme) | Entrée |
|----------|-----------------|--------|
| `exec`, `attach` | `unable to upgrade connection: container not found ("…")`, `container … is not valid for pod …` | `notfound.container-exec` |
| `logs` | `a container name must be specified for pod …, choose one of: [...]` | `cmdline.container-required` |
| `port-forward` | port local occupé, pod non `Running`, `an error occurred forwarding … connection refused` | `portforward.*` |
| `rollout status` | `deployment "…" exceeded its progress deadline` | `rollout.progress-deadline` |
| `apply`, `replace` | `field is immutable` | `conflict.field-immutable` |

**Capture de stderr** (research.md S10) : étendue à `exec`, `attach`, `debug` et
`port-forward`, stdin et stdout intacts, après validation par les tests en pseudo-terminal ;
`edit`, `proxy`, `logs -f`, `get -w` restent sans capture. Une commande interactive dont
stderr n'est pas reconnu garde l'explication par le code de sortie. Après Ctrl-C (code 130)
sur `port-forward`, une explication n'est produite que si une erreur de transfert reconnue a
été captée.

**Anti-pattern** (FR-028) : toute correction qui pourrait conduire à
`kubectl delete … --force --grace-period=0` ou à la suppression des finalizers est
accompagnée du bloc ⚠ Anti-pattern et de la démarche d'identification de ce qui bloque.

---

## `last`, `deepen`

Un diagnostic d'objet devient le dernier diagnostic ; `last` le réaffiche **sans relire le
cluster**, précédé de sa date ; `deepen [--exercise]` approfondit le concept du premier piège
(labels et selectors, endpoints, ports, rollout, classes, mode de liaison, configuration
figée, tag mutable).

---

## Sécurité transverse (FR-017 à FR-020)

- Lectures : liste fermée de la feature 001 + research.md S1 ; jamais d'objet Secret, jamais
  de valeur de ConfigMap ni d'annotation hors des quatre clés de S2.
- Aucune commande de test (`port-forward`, `curl`, `exec`) n'est exécutée : elles sont
  affichées.
- `SECURITY.md` et le README listent les nouvelles lectures dans la même PR que leur code.
