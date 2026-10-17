# Politique de sécurité

## Versions prises en charge

Seule la dernière version publiée de TELEMACH PLUGIN KUBECTL reçoit des correctifs de sécurité.

## Signaler une vulnérabilité

Merci de **ne pas ouvrir de ticket public** pour une vulnérabilité.

Utilise le signalement privé de GitHub :
[Security › Report a vulnerability](https://github.com/aboigues/telemach-plugin-kubectl/security/advisories/new).

Indique si possible :

- la version concernée, ton système (Linux, macOS, WSL2) et ta version de kubectl ;
- les étapes pour reproduire le problème ;
- l'impact estimé (fuite de secret ou de kubeconfig, exécution de commande, lecture du cluster
  non prévue, injection de séquences de terminal…).

## Délais

- accusé de réception sous 7 jours ;
- première évaluation sous 14 jours ;
- correctif ou mesure d'atténuation dès que possible, avec mention du signalement si tu le
  souhaites.

## Engagements du projet

- Le plugin ne modifie **aucun fichier** de l'apprenant et **aucune ressource** d'un cluster.
- Il n'exécute qu'une liste fermée de commandes, toutes en lecture, avec la commande `kubectl`
  et les droits de l'apprenant (même kubeconfig, même contexte, et l'identité `--as` seulement
  si l'apprenant la donne à `diagnose`) :
  - `kubectl version --client -o json`, lu une fois par l'intégration shell (aucun contact
    avec le serveur API) ;
  - pour `kubectl telemach diagnose` : `get pods`, `get pod <nom>`, `get deployment <nom>`,
    `get events` (filtrés sur un pod) au format JSON, et `logs --tail=20` d'un conteneur en
    échec ;
  - pour la vérification différée après une création réussie : uniquement le statut des pods
    concernés (`get pods` / `get pod <nom>`).
- Il ne lit jamais d'objet `Secret` et ne conserve jamais la valeur d'une variable
  d'environnement d'un pod.
- `explain`, `check`, `last` et `deepen` fonctionnent sans aucun accès réseau.
- Les valeurs sensibles (jetons, mots de passe, certificats, contenu de kubeconfig, données
  d'un `Secret`) sont masquées avant tout affichage ou toute écriture sur disque.
