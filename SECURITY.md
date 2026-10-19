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
  - pour `kubectl telemach diagnose` : `get pods`, `get pod <nom>`, `get deployment(s)`,
    `get replicasets -l`, `get service(s)`, `get endpointslices`, `get ingress(es)`,
    `get ingressclasses`, `get pvc`, `get storageclasses` et `get events` (filtrés sur un pod)
    au format JSON, `logs --tail=20` d'un conteneur en échec, et les seules dates de
    modification d'une ConfigMap (`managedFields[].time`, jamais ses valeurs) ;
  - pour la vérification différée après une création réussie (`apply`, `create`, `run`,
    `expose`) : le statut des pods concernés et les Services, Ingress et PVC créés, au plus
    12 lectures.
- Il ne lit jamais d'objet `Secret` et ne conserve jamais la valeur d'une variable
  d'environnement d'un pod ni celle d'une ConfigMap. Des annotations, il ne garde que quatre
  clés publiques (classes par défaut d'Ingress et de stockage, `kubernetes.io/ingress.class`,
  révision d'un ReplicaSet).
- La sortie d'erreur de `exec`, `attach`, `debug` et `port-forward` est copiée comme celle des
  autres commandes (fichier privé `0600`, supprimé aussitôt), sans toucher à leur entrée ni à
  leur sortie standard.
- `hints.json` (`0600`) ne contient que l'empreinte SHA-256 des images déjà signalées.
- `explain`, `check`, `last` et `deepen` fonctionnent sans aucun accès réseau.
- Les valeurs sensibles (jetons, mots de passe, certificats, contenu de kubeconfig, données
  d'un `Secret`) sont masquées avant tout affichage ou toute écriture sur disque.
