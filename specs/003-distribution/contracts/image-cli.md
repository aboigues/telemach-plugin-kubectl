# Contrat — Interface de l'image `telemachlearning/telemach-kubectl`

Point d'entrée : `/usr/local/bin/telemach-image`. Utilisateur par défaut : `65532:65532`.
Aucun kubeconfig ni `KUBECONFIG` dans l'image : aucun cluster ne peut être contacté.

## Commandes

| Arguments | Effet |
|---|---|
| *(aucun)*, `--help`, `-h`, `help` | aide en français : les deux usages, avec les commandes complètes à copier (FR-019) ; code 0 |
| `install` | installe le plugin dans `/sortie` (ci-dessous) |
| `install --help` | aide détaillée de l'installation ; code 0 |
| `check` sans `-f` | équivaut à `check -f /manifestes -R` |
| `check …` dont le dossier visé n'existe pas ou ne contient aucun fichier `.yaml`/`.yml` (sous-dossiers compris) | message propre à l'image en français (aucun manifeste trouvé, concept de montage, commande d'essai complète à copier) ; code 2 ; le plugin n'est pas exécuté |
| toute autre commande (`check -f …`, `version`, `explain …`) | exécution directe de `/opt/telemach/linux/kubectl-telemach` avec les mêmes arguments ; sortie et code identiques au plugin installé (FR-016) |

## Usages documentés (forme canonique, à reprendre telle quelle dans le README)

Essai sans installation, depuis le dossier des manifestes :

```bash
docker run --rm -t -v "$PWD":/manifestes:ro telemachlearning/telemach-kubectl:<version> check
```

Installation, Linux et WSL2 :

```bash
mkdir -p ~/.local/bin
docker run --rm --user "$(id -u):$(id -g)" \
  -v ~/.local/bin:/sortie \
  telemachlearning/telemach-kubectl:<version> install
```

Installation, macOS (Docker Desktop) :

```bash
mkdir -p ~/.local/bin
docker run --rm --user "$(id -u):$(id -g)" -e HOST_OS=darwin \
  -v ~/.local/bin:/sortie \
  telemachlearning/telemach-kubectl:<version> install
```

## `install` — entrées

| Entrée | Valeurs | Défaut |
|---|---|---|
| montage `/sortie` | dossier de l'hôte qui est dans le `PATH` (documenté : `~/.local/bin`) | obligatoire |
| `HOST_OS` | `linux`, `darwin` (insensible à la casse) | `linux` |
| identité (`--user`) | celle de l'utilisateur hôte | `65532:65532` (échoue en général sous Linux, avec explication) |

## `install` — sorties

Sur stdout en cas de succès, sur stderr en cas de refus ; texte en français ; couleurs selon les
mêmes règles que le plugin (`NO_COLOR`, terminal).

Succès (exemple) :

```text
✔ TELEMACH 0.2.0 installé dans ~/.local/bin (Linux, x86-64 — linux/amd64).
  Version remplacée : 0.1.0.

📘 Ce qui vient de se passer
   Le conteneur a écrit dans un dossier de ta machine grâce au montage
   « -v ~/.local/bin:/sortie » (bind mount), avec ton identité (--user).
   kubectl trouve un plugin par son nom (kubectl-telemach) dans les dossiers du PATH.

➜ Étapes suivantes
   1. Vérifie que ~/.local/bin est dans ton PATH :  kubectl plugin list
      sinon : echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
   2. Active l'explication automatique des erreurs :
      echo 'eval "$(kubectl-telemach shell-init zsh)"' >> ~/.zshrc
   puis relance ton terminal et vérifie : kubectl telemach version
```

Le chemin affiché côté hôte est générique (`~/.local/bin`) : le conteneur ne connaît que
`/sortie`.

Refus obligatoires (FR-018), chacun avec concept et correction : dossier non monté ; dossier
appartenant à root ; dossier non accessible en écriture ; fichier existant qui n'est pas un
binaire TELEMACH ; **lien symbolique** (jamais remplacé ; le message propose `ls -l` et
`kubectl plugin list` pour repérer les installations) ; `HOST_OS` non pris en charge
(`windows` → WSL2). Le message de succès rappelle qu'une seconde installation (Krew, dans
`~/.krew/bin`) serait signalée comme masquée par `kubectl plugin list`.

## Codes de sortie de `install`

| Code | Signification |
|---|---|
| 0 | installé ou déjà à jour |
| 1 | refus lié à l'environnement (montage, droits, fichier existant) ; rien n'a été écrit |
| 2 | erreur d'utilisation (`HOST_OS` invalide, argument inconnu) |

## Garanties

- `install` n'écrit que `/sortie/kubectl-telemach` (et un fichier temporaire dans `/sortie`,
  renommé ou supprimé avant la fin) ; jamais ailleurs.
- `check` lit le dossier monté en lecture seule, n'écrit rien et ne contacte aucun cluster ;
  le plugin est lancé sans `HOME` ni `XDG_STATE_HOME`, donc sans dossier d'état : il
  n'enregistre ni `last.json` ni `hints.json` (pas même dans un dossier monté sans `:ro`).
- Aucun accès réseau, aucune élévation de privilèges, aucun shell dans l'image.
