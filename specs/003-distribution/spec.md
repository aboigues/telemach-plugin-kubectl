# Feature Specification: Distribution du plugin sans Go

**Feature Branch**: `003-distribution`

**Created**: 2026-10-19

**Status**: Draft

**Input**: User description: "003-distribution — Diffuser le plugin kubectl-telemach sans exiger Go chez l'apprenant, sur le modèle de la feature 002-distribution du plugin TELEMACH Docker, transposé à kubectl. 1. GitHub Releases = source de référence : à chaque tag vX.Y.Z posé sur main (gates vertes), binaires kubectl-telemach linux/darwin (amd64, arm64), version injectée, sommes SHA-256, SBOM, signature keyless (Sigstore) et attestation de provenance SLSA ; Release immuable, notes en français ; README d'installation depuis la Release (le binaire se place dans un dossier du PATH, kubectl trouve les plugins par leur nom kubectl-*). 2. Index Krew hébergé par le projet : à chaque Release, un manifeste Krew du plugin « telemach » est généré et publié dans un index propre au projet ; installation par « kubectl krew index add » puis « kubectl krew install » ; l'index officiel kubernetes-sigs/krew-index est hors périmètre. 3. Image Docker Hub dédiée telemachlearning/telemach-kubectl : essai sans installation (check de manifestes sur un dossier monté en lecture seule, sans contacter de cluster) et installation du plugin sur l'hôte (install, HOST_OS=darwin pour macOS) dans un dossier du PATH monté ; image minimale, non-root, épinglée par digest, multi-architecture, signée, SBOM et provenance, gate de scan. 4. Jeton Docker Hub créé et stocké par le mainteneur dans un environnement GitHub protégé « release » restreint aux tags v*. Première Release : v0.1.0. Hors périmètre : index Krew officiel, Homebrew, Windows natif, deb/rpm."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Installer le plugin depuis une Release, sans Go (Priority: P1)

Un apprenant (ou un formateur qui prépare une salle de TP) veut installer TELEMACH sur une
machine Linux, macOS ou WSL2 qui a déjà kubectl. Il ouvre la page de la dernière version du
projet, télécharge le binaire correspondant à son système et à son processeur, vérifie qu'il
n'a pas été altéré (somme de contrôle, signature), le place dans un dossier de son `PATH` sous
le nom `kubectl-telemach`, et constate avec `kubectl telemach version` qu'il a bien la version
publiée et que kubectl le reconnaît comme plugin.

**Why this priority**: c'est la voie de référence. Aujourd'hui, installer TELEMACH exige Go
1.27 et une compilation, ce qui exclut une grande partie du public débutant. À elle seule,
cette histoire lève ce frein et constitue un MVP ; les autres voies (Krew, image) reposent sur
les mêmes fichiers publiés.

**Independent Test**: sur une machine propre sans Go (Linux amd64, Linux arm64, macOS arm64),
suivre uniquement la procédure du README et vérifier que `kubectl telemach version` affiche la
version de la Release, que `kubectl plugin list` cite le plugin, et que l'explication
automatique d'une erreur fonctionne une fois l'intégration shell activée.

**Acceptance Scenarios**:

1. **Given** un tag de version `vX.Y.Z` poussé sur un commit de `main` dont les gates sont
   vertes, **When** la publication s'exécute, **Then** une Release `vX.Y.Z` apparaît avec, pour
   chacune des quatre plateformes (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64), le
   binaire seul et une archive le contenant, un fichier de sommes SHA-256 couvrant tous ces
   fichiers, un inventaire des composants (SBOM), une signature vérifiable et une attestation
   de provenance.
2. **Given** un binaire téléchargé depuis la Release, **When** l'apprenant lance la commande de
   vérification de somme documentée, **Then** elle confirme l'intégrité du fichier ; **and**
   la vérification de signature documentée confirme qu'il a été produit par le workflow
   officiel du dépôt.
3. **Given** le binaire installé sous le nom `kubectl-telemach` dans un dossier du `PATH`,
   **When** l'apprenant exécute `kubectl telemach version`, **Then** la version affichée est
   exactement `X.Y.Z`, et `kubectl plugin list` cite le plugin sans avertissement.
4. **Given** un binaire volontairement modifié, **When** l'apprenant lance la vérification de
   somme ou de signature, **Then** la vérification échoue de façon visible, et le README
   explique qu'il ne faut pas installer ce fichier.

---

### User Story 2 - Installer et mettre à jour le plugin avec Krew (Priority: P2)

Un apprenant qui utilise déjà Krew, le gestionnaire de plugins de kubectl, ajoute l'index du
projet une fois, puis installe TELEMACH en une commande ; plus tard, une seule commande le met
à jour vers la dernière version publiée. Krew vérifie lui-même l'intégrité de ce qu'il
télécharge.

**Why this priority**: Krew est la façon habituelle d'installer un plugin kubectl ; il évite de
choisir la bonne archive, place le plugin dans le `PATH` et gère les mises à jour. Il s'appuie
sur les fichiers de la Release (P1).

**Independent Test**: sur une machine où Krew est installé, suivre les deux commandes du
README (ajout de l'index, installation), vérifier que `kubectl telemach version` affiche la
dernière version publiée, puis qu'après la publication d'une version plus récente, la commande
de mise à jour l'installe.

**Acceptance Scenarios**:

1. **Given** une Release stable `vX.Y.Z` publiée, **When** la publication se termine, **Then**
   l'index Krew du projet propose le plugin `telemach` en version `vX.Y.Z`, avec l'adresse et
   la somme SHA-256 de l'archive de chaque plateforme, identiques à celles de la Release.
2. **Given** une machine avec Krew, **When** l'apprenant ajoute l'index du projet puis installe
   le plugin avec les commandes documentées, **Then** `kubectl telemach version` affiche
   `X.Y.Z` et Krew affiche un message en français qui donne la prochaine étape
   (activation de l'intégration shell).
3. **Given** une version plus récente publiée, **When** l'apprenant lance la mise à jour
   documentée, **Then** Krew installe la nouvelle version.
4. **Given** une pré-version (`vX.Y.Z-rc.N`), **When** elle est publiée, **Then** l'index Krew
   n'est pas modifié : seules les versions stables y sont proposées.
5. **Given** une archive dont le contenu ne correspond pas à la somme annoncée par l'index,
   **When** Krew l'installe, **Then** l'installation échoue sans rien installer.

---

### User Story 3 - Installer le plugin avec Docker seulement (Priority: P3)

Un apprenant qui a Docker (souvent déjà présent pour faire tourner un cluster local comme kind)
installe le plugin en une seule commande `docker run`, à partir de l'image officielle
`telemachlearning/telemach-kubectl`. L'image copie dans un dossier de son `PATH` le binaire
adapté à son système, puis lui explique en français ce qu'elle a fait et comment activer
l'explication automatique.

**Why this priority**: c'est la voie la plus simple pour un public qui a déjà Docker mais pas
Krew. Elle s'appuie sur les mêmes binaires que la Release (P1).

**Independent Test**: sur Linux (et sous macOS avec Docker Desktop), lancer la commande
d'installation documentée, puis vérifier que `kubectl telemach version` affiche la version de
l'image et que le fichier installé appartient à l'utilisateur.

**Acceptance Scenarios**:

1. **Given** une machine Linux avec Docker, **When** l'apprenant exécute la commande
   d'installation documentée (dossier de destination monté, tag de version explicite),
   **Then** le binaire linux de la bonne architecture est copié sous le nom `kubectl-telemach`
   dans ce dossier, exécutable, appartenant à l'utilisateur, et un message en français résume
   ce qui a été fait et la prochaine étape (`shell-init`).
2. **Given** un Mac avec Docker Desktop, **When** l'apprenant exécute la commande documentée
   pour macOS, **Then** c'est le binaire darwin de l'architecture du Mac qui est copié, et
   `kubectl telemach version` fonctionne sur le Mac.
3. **Given** l'apprenant oublie de monter le dossier de destination, **When** il lance
   l'installation, **Then** rien n'est copié et un message en français explique la notion de
   montage (bind mount) et donne la commande complète à copier.
4. **Given** une version du plugin déjà installée, **When** l'apprenant relance l'installation
   avec une image plus récente, **Then** le binaire est remplacé et le message indique
   l'ancienne et la nouvelle version.
5. **Given** un dossier de destination absent du `PATH` de l'apprenant, **When** l'installation
   se termine, **Then** le message rappelle que kubectl ne trouve un plugin que dans un dossier
   du `PATH` et montre comment l'y ajouter.

---

### User Story 4 - Essayer TELEMACH sans rien installer (Priority: P4)

Une personne curieuse, ou un formateur pendant une démonstration, veut voir ce que TELEMACH dit
de manifestes Kubernetes sans rien installer ni contacter de cluster. Elle lance l'image en
montant son dossier de manifestes en lecture seule et obtient la vérification pédagogique des
manifestes.

**Why this priority**: utile pour découvrir le projet et pour les démonstrations, mais la
valeur principale du plugin (l'explication automatique des erreurs kubectl et le diagnostic
d'un cluster) nécessite l'installation.

**Independent Test**: dans un dossier contenant un manifeste fautif, lancer la commande
d'essai documentée et vérifier que le résultat est identique à celui de
`kubectl telemach check` installé, qu'aucun fichier n'est modifié et qu'aucun cluster n'est
contacté.

**Acceptance Scenarios**:

1. **Given** un dossier contenant un manifeste comportant une erreur, **When** l'apprenant
   exécute la commande d'essai documentée, **Then** il obtient la même vérification
   pédagogique qu'avec le plugin installé, en couleur si son terminal le permet.
2. **Given** le dossier monté en lecture seule, **When** la vérification s'exécute, **Then**
   aucun fichier n'est créé ni modifié, et aucune connexion à un cluster n'est tentée.
3. **Given** aucun manifeste dans le dossier monté, **When** l'apprenant lance l'essai,
   **Then** un message en français explique qu'aucun fichier n'a été trouvé et rappelle
   comment monter le bon dossier.

---

### User Story 5 - Publier une version en toute confiance (Priority: P2)

Le mainteneur décide de publier une version : il pose un tag `vX.Y.Z` sur `main` et le pousse.
Sans autre manipulation, la Release, l'index Krew et l'image sont produits, signés et publiés.
Il n'a jamais à confier son jeton Docker Hub à quelqu'un d'autre, et une version publiée ne
peut pas être écrasée silencieusement.

**Why this priority**: sans publication automatisée et sûre, les histoires P1 à P4 n'ont pas de
contenu fiable à distribuer.

**Independent Test**: pousser un tag de pré-version et vérifier que Release et image
apparaissent, signées, que l'index Krew n'est pas modifié, et qu'une seconde exécution sur le
même tag n'écrase rien.

**Acceptance Scenarios**:

1. **Given** le secret Docker Hub configuré par le mainteneur dans l'environnement protégé
   « release », **When** un tag `vX.Y.Z` est poussé, **Then** la Release, l'image
   `telemachlearning/telemach-kubectl:X.Y.Z` et l'entrée de l'index Krew sont publiées sans
   intervention supplémentaire.
2. **Given** un tag qui pointe sur un commit absent de `main`, ou dont le nom n'est pas une
   version valide, **When** il est poussé, **Then** rien n'est publié et la raison est indiquée
   dans le journal de l'exécution.
3. **Given** une Release `vX.Y.Z` déjà publiée, **When** la publication est relancée pour le
   même tag, **Then** elle échoue sans modifier les artefacts existants ni l'index Krew.
4. **Given** un pull request qui modifie l'image, **When** les gates s'exécutent, **Then**
   l'image est construite et analysée (vulnérabilités, bonnes pratiques du Dockerfile), et une
   vulnérabilité critique ou haute bloque la fusion.
5. **Given** un pull request qui modifie le modèle du manifeste Krew, **When** les gates
   s'exécutent, **Then** un manifeste d'essai est généré et validé, et un manifeste invalide
   bloque la fusion.

---

### Edge Cases

- **Dossier de destination inexistant** (installation par l'image, Linux) : si Docker crée
  lui-même le dossier monté, il appartient à root et l'utilisateur ne pourra plus le gérer.
  La commande documentée crée donc le dossier avant de lancer l'image, et l'installation
  refuse d'écrire dans un dossier qui appartient à root alors qu'elle ne s'exécute pas en root
  (signe que Docker l'a créé), en expliquant pourquoi. La règle ne compare pas le propriétaire
  à l'utilisateur, car Docker Desktop (macOS) peut présenter les fichiers montés sous une autre
  identité.
- **Droits d'écriture** : l'image s'exécute sans privilèges root ; la commande documentée la
  lance avec l'identité de l'utilisateur, pour que le fichier installé lui appartienne. Sans
  droit d'écriture, l'installation s'arrête avec une explication en français (droits Unix,
  identité du processus dans le conteneur). Aucune procédure documentée n'installe le plugin
  dans un dossier système qui exige `sudo`.
- **Dossier hors du `PATH`** : un plugin kubectl n'est trouvé que s'il est dans un dossier du
  `PATH` ; la procédure documentée utilise un dossier personnel (par exemple `~/.local/bin`),
  et chaque voie d'installation rappelle comment vérifier qu'il est dans le `PATH`.
- **Deux installations concurrentes** (Krew et installation manuelle) : kubectl prend le
  premier plugin du `PATH` et `kubectl plugin list` signale l'autre comme masqué ; le README
  explique comment le repérer et n'en garder qu'une.
- **Système non pris en charge** : `HOST_OS` absent sur macOS → binaire linux copié par
  erreur. L'installation rappelle dans son message final quel système a été ciblé et comment
  cibler macOS ; une valeur de `HOST_OS` inconnue (ex. `windows`) est refusée avec un message
  qui renvoie vers WSL2.
- **Architecture** : l'image est multi-architecture ; sur un Mac Apple Silicon, l'image arm64
  est utilisée et copie le binaire darwin/arm64. Si l'apprenant force une autre plateforme
  d'image, le message final indique l'architecture installée.
- **Fichier existant** dans le dossier de destination qui n'est pas un binaire TELEMACH (ou un
  lien symbolique, par exemple celui que Krew a créé) : l'installation par l'image ne le
  remplace pas et explique pourquoi.
- **Essai sans kubectl** : dans le conteneur d'essai, aucune version de kubectl n'est
  disponible ; la vérification applique alors les règles de la version la plus récente prise
  en charge et le dit en première ligne, comme le fait déjà `check`.
- **Index Krew en retard** : si la mise à jour de l'index échoue après la publication de la
  Release, la Release reste valable et les autres voies fonctionnent ; l'échec est visible
  dans le journal et la mise à jour de l'index peut être relancée sans republier la Release.
- **Tag `latest`** : la documentation utilise toujours un tag de version explicite ; si
  l'apprenant utilise `latest` ou aucun tag, l'image fonctionne, et le README rappelle pourquoi
  un tag explicite est préférable (principe III).
- **Téléchargement par navigateur sous macOS** : un binaire téléchargé par un navigateur peut
  être bloqué par le système ; le README privilégie un téléchargement en ligne de commande et
  explique le message éventuel.
- **Pré-versions** (`vX.Y.Z-rc.N`) : publiées comme pré-versions, sans déplacer le tag
  `latest` de l'image, ni la « dernière version » de la page des Releases, ni l'index Krew.

## Requirements *(mandatory)*

### Functional Requirements

**Publication des binaires (GitHub Releases)**

- **FR-001**: La publication MUST se déclencher à chaque push d'un tag au format `vX.Y.Z`
  (versionnement sémantique, pré-versions comprises) et pouvoir être relancée manuellement pour
  un tag existant.
- **FR-002**: La publication MUST refuser un tag qui ne pointe pas sur un commit de `main`, et
  MUST NOT publier si les gates obligatoires (lint, tests, vulnérabilités, secrets) échouent sur
  ce commit.
- **FR-003**: Chaque Release MUST contenir, pour linux/amd64, linux/arm64, darwin/amd64 et
  darwin/arm64, le binaire `kubectl-telemach` seul et une archive qui le contient avec les
  fichiers `LICENSE` et `NOTICE` ; `kubectl telemach version` MUST afficher exactement la version du tag (sans
  le `v`).
- **FR-004**: Chaque Release MUST contenir un fichier de sommes SHA-256 couvrant tous les
  binaires et archives, un SBOM, une signature sans clé gérée par le mainteneur (keyless,
  Sigstore) et une attestation de provenance (SLSA) liant chaque fichier publié au commit et
  au workflow qui l'ont produit.
- **FR-005**: Les binaires MUST être reproductibles à partir du tag (même source, même chaîne
  d'outils épinglée) et ne dépendre d'aucune bibliothèque système.
- **FR-006**: Une Release existante MUST NOT être écrasée ; une nouvelle exécution sur un tag
  déjà publié MUST échouer sans modifier les artefacts.
- **FR-007**: Les notes de Release MUST être rédigées en français et lister les changements
  depuis la version précédente, avec les commandes d'installation (Release, Krew, image) et de
  vérification.

**Index Krew du projet**

- **FR-008**: Chaque version stable MUST mettre à jour automatiquement l'index Krew du projet
  avec un manifeste du plugin `telemach` qui référence, pour chaque plateforme, l'archive de la
  Release et sa somme SHA-256 (identiques à celles du fichier de sommes) ; une pré-version
  MUST NOT modifier l'index.
- **FR-009**: Le manifeste Krew MUST porter une description courte et une description en
  français, l'adresse du projet et un message affiché après l'installation qui indique la
  prochaine étape (activation de l'intégration shell avec `kubectl-telemach shell-init`).
- **FR-010**: L'index Krew MUST être hébergé par le projet, dans un emplacement que seule la
  publication automatisée met à jour ; il MUST NOT exiger de modification manuelle à chaque
  version, et sa mise à jour MUST pouvoir être relancée sans republier la Release.
- **FR-011**: Le modèle du manifeste MUST être validé par les gates de chaque pull request qui
  le modifie (manifeste d'essai généré puis vérifié par l'outil de validation de Krew).

**Image Docker Hub `telemachlearning/telemach-kubectl`**

- **FR-012**: Chaque version MUST publier l'image sous les tags `X.Y.Z` et `X.Y` ; seule une
  version stable (hors pré-version) met à jour `latest`. L'image MUST être disponible pour
  linux/amd64 et linux/arm64.
- **FR-013**: L'image MUST contenir les binaires issus de la même Release (mêmes sommes
  SHA-256) : ceux de son architecture pour linux et pour darwin.
- **FR-014**: L'image MUST être minimale (aucun shell ni gestionnaire de paquets), s'exécuter
  avec un utilisateur non-root, partir d'une base épinglée par digest et ne contenir aucun
  secret ni aucun kubeconfig.
- **FR-015**: L'image MUST être signée (keyless) et accompagnée d'un SBOM et d'une attestation
  de provenance vérifiables.
- **FR-016**: Lancée avec `check`, l'image MUST produire la même vérification qu'un plugin
  installé, sur un dossier monté en lecture seule, sans écrire nulle part et sans contacter de
  cluster.
- **FR-017**: Lancée avec `install`, l'image MUST copier dans le dossier monté de destination,
  sous le nom `kubectl-telemach`, le binaire correspondant au système demandé (`HOST_OS`, linux
  par défaut ; darwin) et à l'architecture de l'image, avec les droits d'exécution, puis
  afficher en français : version installée (et version remplacée le cas échéant), système et
  architecture ciblés, rappel du `PATH`, prochaine étape (`shell-init`).
- **FR-018**: `install` MUST échouer avec une explication pédagogique en français, sans rien
  modifier, quand : le dossier de destination n'est pas monté, n'est pas accessible en
  écriture, appartient à root alors que l'installation ne s'exécute pas en root (dossier créé
  par Docker), contient un fichier homonyme qui n'est pas un binaire TELEMACH (lien symbolique
  compris), ou quand `HOST_OS` a une valeur non prise en charge.
- **FR-019**: Lancée sans argument ou avec `--help`, l'image MUST afficher en français ses deux
  usages (essai et installation) avec les commandes complètes à copier.

**Gates et sécurité de la chaîne de publication**

- **FR-020**: Chaque pull request MUST construire l'image et l'analyser (vulnérabilités connues
  et bonnes pratiques du Dockerfile) ; une vulnérabilité critique ou haute MUST bloquer la
  fusion (principe IV), et cette gate MUST devenir un check obligatoire de `main`.
- **FR-021**: Le jeton Docker Hub MUST être créé et enregistré par le mainteneur lui-même,
  limité au dépôt `telemach-kubectl` (lecture et écriture), et accessible uniquement depuis un
  environnement GitHub protégé « release » restreint aux tags `v*`. Il MUST NOT apparaître dans
  le dépôt, les journaux ni les artefacts. Toute autre autorisation nécessaire à la mise à jour
  de l'index Krew suit les mêmes règles.
- **FR-022**: Les workflows de publication MUST respecter le principe IV : actions épinglées par
  SHA complet, permissions minimales déclarées par job (écriture seulement là où elle est
  nécessaire), aucune exécution de code non fiable.
- **FR-023**: Tous les outils et images de base ajoutés MUST être épinglés sur la dernière
  version stable vérifiée au moment de l'ajout (règle globale et D-LL-004), et les outils
  compilés avec Go MUST l'être avec la chaîne d'outils du projet (D-LL-001).

**Documentation**

- **FR-024**: Le README MUST présenter, en français et dans cet ordre : installation par la
  Release (avec vérification de somme et de signature), installation par Krew, installation
  par l'image, essai sans installation, puis compilation depuis les sources (procédure
  actuelle).
- **FR-025**: Chaque commande documentée MUST être exécutable telle quelle et utiliser un tag
  de version explicite ; les procédures documentées MUST être testées de bout en bout sans
  raccourci d'environnement (D-LL-006, LL-004).
- **FR-026**: `CONTRIBUTING.md` MUST décrire la procédure de publication (poser et pousser un
  tag, relance manuelle, relance de la seule mise à jour de l'index Krew, que faire en cas
  d'échec) et les prérequis côté dépôt et Docker Hub (environnement « release », secrets,
  réglages), conformément à D-LL-005.

### Key Entities

- **Version** : un tag `vX.Y.Z` sur `main` ; détermine la Release, les tags de l'image, l'entrée
  de l'index Krew et le texte de `kubectl telemach version`.
- **Release** : page de publication d'une version ; contient les binaires, les archives, les
  sommes, le SBOM, les signatures, l'attestation de provenance et les notes en français.
- **Binaire de plateforme** : `kubectl-telemach` pour un couple système/architecture ; identifié
  par sa somme SHA-256, identique dans la Release, dans l'archive et dans l'image.
- **Archive de plateforme** : fichier compressé qui contient le binaire et la licence ; c'est
  ce que Krew télécharge et vérifie.
- **Manifeste Krew** : description du plugin `telemach` pour Krew (version, plateformes,
  adresses et sommes des archives, textes en français) ; publié dans l'index du projet.
- **Index Krew du projet** : emplacement, propre au projet, où Krew trouve le manifeste.
- **Image** : `telemachlearning/telemach-kubectl`, multi-architecture, porteuse des binaires et
  des deux usages (`check`, `install`) ; identifiée par son digest, signée.
- **Environnement « release »** : contexte protégé qui seul donne accès aux secrets de
  publication.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Sur une machine qui a kubectl mais pas Go, un débutant installe le plugin depuis
  la Release, vérification comprise, en moins de 5 minutes en suivant uniquement le README.
- **SC-002**: Avec Krew déjà présent, l'installation tient en deux commandes à copier (ajout de
  l'index, installation) et la mise à jour en une seule.
- **SC-003**: L'installation par l'image tient en une seule commande à copier (plus la création
  du dossier) et dure moins de 1 minute une fois l'image téléchargée.
- **SC-004**: 100 % des binaires, archives et images publiés sont vérifiables (somme,
  signature, provenance) avec les commandes du README, et toute altération est détectée.
- **SC-005**: Une version est disponible par les trois voies (Release, Krew, image) moins de
  30 minutes après le push de son tag, sans autre action du mainteneur.
- **SC-006**: Aucune image n'est publiée avec une vulnérabilité connue critique ou haute.
- **SC-007**: Le téléchargement de l'image représente moins de 15 Mo par architecture
  (téléchargement rapide en salle de TP, y compris quand vingt postes le font en même temps).
- **SC-008**: La procédure d'installation est validée sur les quatre plateformes cibles avant la
  première Release ; `kubectl telemach version` y affiche la version publiée.

## Assumptions

- Public visé : Linux (dont WSL2) et macOS, avec kubectl déjà installé ; Docker n'est requis que
  pour l'installation par l'image et l'essai sans installation, et Krew que pour sa propre voie.
  Windows natif, Homebrew, les paquets deb/rpm et l'index Krew officiel
  (`kubernetes-sigs/krew-index`) sont hors périmètre ; l'index officiel est prévu plus tard.
- La version provient uniquement du tag git, injectée à la compilation comme le prévoit déjà le
  binaire ; aucun numéro n'est écrit en dur.
- La première Release sera `v0.1.0`, tag posé sur `main` après la fusion de cette feature ;
  aucun tag n'existe encore dans le dépôt.
- Le mainteneur crée lui-même le dépôt Docker Hub `telemachlearning/telemach-kubectl`, le jeton,
  les secrets GitHub et, si l'hébergement retenu l'exige, l'emplacement de l'index Krew et son
  autorisation d'écriture ; Claude Code ne manipule jamais ces secrets.
- L'environnement « release » est restreint aux tags `v*` ; le push du tag par le mainteneur
  vaut décision de publier (pas d'approbation supplémentaire, cohérent avec le workflow de PR
  du projet).
- Les binaires macOS ne sont ni signés ni notarisés par Apple (compte développeur payant) ;
  l'installation en ligne de commande, par Krew ou par l'image évite le blocage lié au
  téléchargement par navigateur.
- La description de la page Docker Hub est rédigée et mise à jour manuellement par le
  mainteneur à partir d'un texte fourni dans le dépôt.
- La chaîne de publication reprend celle du plugin TELEMACH Docker (feature 002-distribution),
  déjà éprouvée, en l'adaptant : nom et emplacement du binaire de plugin kubectl, archives pour
  Krew, index Krew, image dédiée.
- Les termes GitHub Releases, Krew, Docker Hub, SBOM, Sigstore et SLSA relèvent ici du besoin
  exprimé (où et comment l'apprenant obtient et vérifie le plugin), pas d'un choix
  d'implémentation.
