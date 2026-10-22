# LESSON-LEARNED — TELEMACH PLUGIN KUBECTL

Registre des incidents du projet et des leçons qui en découlent (principe V de la
[constitution](.specify/memory/constitution.md)).

## Règles d'usage

- **Relecture obligatoire** : ce fichier MUST être relu au démarrage de chaque session de
  travail et avant chaque étape Spec Kit (`/speckit-specify`, `/speckit-plan`,
  `/speckit-tasks`, `/speckit-implement`).
- **Quoi consigner** : tout incident — bug en production ou en CI, faille de sécurité,
  régression, gate contournée, erreur de processus, mauvaise manipulation.
- **Quand** : l'entrée est ajoutée dans la pull request qui corrige l'incident.
- **Ordre** : les entrées les plus récentes en haut de la section « Incidents ».
- **Automatisation** : quand une leçon peut être vérifiée mécaniquement, elle SHOULD devenir
  une gate GitHub Actions ou un test de non-régression ; le noter dans le champ « Suivi ».
- **Pas de secret** : aucune entrée ne contient de jeton, mot de passe, contenu de kubeconfig
  ou donnée personnelle.

## Modèle d'entrée

```markdown
### LL-XXX — Titre court de l'incident

- **Date** : AAAA-MM-JJ
- **Catégorie** : sécurité | CI/gates | bug | régression | processus | pédagogie | langue
- **Contexte** : ce qui était en cours au moment de l'incident.
- **Cause racine** : pourquoi c'est arrivé (pas seulement le symptôme).
- **Impact** : conséquences observées ou évitées de justesse.
- **Correctif** : ce qui a été fait (lien vers la PR / le commit).
- **Règle préventive** : la règle à appliquer désormais, formulée de façon actionnable.
- **Suivi** : gate ou test ajouté (ou « non automatisable » avec justification).
```

## Leçons héritées du projet Docker

Leçons du projet frère [TELEMACH PLUGIN DOCKER](../telemach-plugin-docker/LESSON-LEARNED.md)
qui s'appliquent ici (research.md R14 de la feature 001). Le détail de chaque incident est
dans le registre d'origine ; seule la règle préventive, transposée à kubectl, est reprise.

- **D-LL-001 — Version de Go des outils d'analyse** : à chaque montée de version de Go,
  vérifier que golangci-lint, govulncheck… sont compilés avec une version de Go au moins
  égale ; sinon les compiler avec la chaîne d'outils du projet (`install-mode: goinstall`).
- **D-LL-002 — zsh duplique stdout avec MULTIOS** : tout code shell partagé entre bash et zsh
  est testé sous les deux shells, avec des assertions sur stdout **et** stderr
  (`setopt NO_MULTIOS` dans la fonction zsh).
- **D-LL-003 — Fichier temporaire et Ctrl-C** : installer le `trap` de nettoyage **avant** de
  créer la ressource à nettoyer, et tester l'interruption à plusieurs moments.
- **D-LL-005 — Prérequis d'une gate** : à la création d'une gate CI, vérifier ses prérequis côté
  dépôt (Dependency graph pour `dependency-review`), les documenter dans `CONTRIBUTING.md` et
  ouvrir une PR de test dès que les gates sont en place.
- **D-LL-004 — Tags d'images d'exemple** : consulter le registre avant d'écrire un tag
  d'image ; pour nginx, la branche stable a un numéro mineur pair.
- **D-LL-006 et D-LL-013 — Intégration shell inactive ou périmée** : les tests de bout en bout
  reproduisent exactement la procédure d'installation documentée ; l'état chargé dans le shell
  porte sa version (`TELEMACH_KUBECTL_SHELL`) et `last`/`deepen` signalent un écart.
- **D-LL-007 — Tests à étiquette de build** : toute étiquette de build est ajoutée à
  `run.build-tags` de `.golangci.yml` dans la même PR.
- **D-LL-008 — Commit malgré un lint en échec** : enchaîner validation et commit uniquement par
  `&&` ; un commit n'est créé que si chaque validation a réussi.
- **D-LL-009 — Vérification qui n'a rien comparé** : un script de vérification ad hoc s'exécute
  dans `bash -c` et échoue si une valeur comparée est vide.
- **D-LL-012 — Objectif chiffré sans test** : tout objectif chiffré (délai, taille) reçoit un
  test qui le mesure dans la tâche même où il est annoncé.
- **D-LL-014 — Forme d'appel supposée** : chaque point d'entrée appelé par un autre programme
  est testé avec la forme exacte que ce programme lui transmet, observée sur le vrai programme
  (ici : kubectl refuse les options placées avant le nom du plugin).
- **D-LL-015 — Objets de l'utilisateur touchés par un essai** : un essai ne vise que des objets
  qu'il a créés lui-même ; ici, les tests ne s'exécutent que sur un cluster kind jetable, jamais
  sur le contexte kubectl courant de la machine.

## Incidents

### LL-013 — Index Krew non mis à jour : secret d'environnement non transmis au workflow appelé

- **Date** : 2026-10-22
- **Catégorie** : CI/gates
- **Contexte** : feature 003, publication de `v0.1.0` (T049), run 39814782519.
- **Cause racine** : `krew-index.yml` est appelé par `release.yml` (`workflow_call`) et déclare
  l'environnement `release`, mais GitHub ne donne au workflow appelé que les secrets que
  l'appelant lui transmet, même pour un secret d'environnement (documentation « Reuse
  workflows »). `KREW_INDEX_DEPLOY_KEY` est donc arrivé vide. La pré-version, qui n'écrit pas
  dans l'index, ne pouvait pas le révéler ; la relance directe (`workflow_dispatch`), seule
  testée à la conception, lit bien le secret.
- **Impact** : Release `v0.1.0` et images `0.1.0`, `0.1`, `latest` publiées et signées, mais
  index Krew non mis à jour ; run marqué en échec. Le contrôle explicite du secret vide a
  arrêté le job avant toute écriture.
- **Correctif** : `secrets: KREW_INDEX_DEPLOY_KEY` transmis nommément par `release.yml` et
  déclaré dans `on.workflow_call.secrets` de `krew-index.yml` ; index mis à jour par la
  relance directe documentée (quickstart P3).
- **Règle préventive** : un chemin qui ne s'exécute qu'à la première version stable est testé
  sous sa forme réelle d'appel (ici `workflow_call`), pas seulement par une forme voisine ; tout
  secret lu par un workflow appelé est transmis par son nom.
- **Suivi** : `TestCalledWorkflowsReceiveTheirSecrets` dans `tests/manifests/hygiene_test.go`.

### LL-012 — Image signée d'une pré-version écrasée par la publication de la version stable

- **Date** : 2026-10-22
- **Catégorie** : sécurité
- **Contexte** : feature 003, publication de `v0.1.0` (T049) sur le commit déjà tagué
  `v0.1.0-rc.2` (pré-version de répétition réussie et vérifiée).
- **Cause racine** : GoReleaser détermine lui-même la version parmi les tags du commit ; il a
  retenu `v0.1.0-rc.2` au lieu du tag `v0.1.0` qui avait déclenché le workflow. Le job
  `verify`, lui, avait contrôlé l'absence de l'image `0.1.0` (`github.ref_name`) : le contrôle
  et la publication ne portaient pas sur la même version.
- **Impact** : l'image `telemachlearning/telemach-kubectl:0.1.0-rc.2` a été repoussée
  (14:24:59 UTC) vers un digest non signé et non attesté (`sha256:bd389f…`) au lieu de
  `sha256:4a2030…` ; `cosign verify` échoue sur ce tag. La Release `v0.1.0-rc.2`, immuable, a
  bloqué la suite : ni Release `v0.1.0`, ni image `0.1.0`, ni index Krew modifié.
- **Correctif** : `GORELEASER_CURRENT_TAG: ${{ github.ref_name }}` sur l'étape GoReleaser de
  `release.yml` ; tag `0.1.0-rc.2` remis sur le digest signé par le mainteneur.
- **Règle préventive** : chaque outil de publication reçoit explicitement la version contrôlée
  par `verify` ; ne jamais le laisser la déduire de l'état du dépôt (plusieurs tags sur un
  commit est un cas normal : pré-version puis version stable).
- **Suivi** : `TestReleaseUsesTriggeringTag` dans `tests/manifests/hygiene_test.go`.

### LL-011 — Publication figée plus d'une heure par un job sans délai maximal

- **Date** : 2026-10-22
- **Catégorie** : CI/gates
- **Contexte** : feature 003, pré-version de répétition `v0.1.0-rc.1` (T048).
- **Cause racine** : aucun job des workflows ne déclarait `timeout-minutes` ; GitHub attend
  alors jusqu'à 6 heures. Le job `integration-kind (v1.37.1)` est resté bloqué sur
  `apt-get install zsh` (miroir apt sans réponse) alors que les deux autres versions avaient
  fini en 9 minutes. Dans le même run, la gate `vuln` a échoué sur GO-2026-6809
  (`crypto/x509` de Go 1.27.2), publiée après la dernière CI verte de `main` : ce refus-là
  était le comportement attendu.
- **Impact** : publication en attente plus d'une heure pour un run déjà condamné par `vuln` ;
  rien n'a été publié (ni Release ni image). Run annulé, tag `v0.1.0-rc.1` sans effet.
- **Correctif** : `timeout-minutes` sur chaque job (environ trois fois la durée habituelle),
  5 minutes sur les étapes `apt-get` ; Go 1.27.3.
- **Règle préventive** : tout job et toute étape qui dépend du réseau (apt, téléchargements)
  déclare un délai maximal ; avant de poser un tag, relancer `govulncheck` sur `main` (la base
  de vulnérabilités évolue sans commit).
- **Suivi** : `TestWorkflowsTimeouts` dans `tests/manifests/hygiene_test.go`.

### LL-010 — Dossier d'état relatif au dossier courant sans HOME

- **Date** : 2026-10-19
- **Catégorie** : bug
- **Contexte** : feature 003, vérification de `check` dans l'image `scratch` (T043) : le
  plugin y tourne sans `HOME` ni `XDG_STATE_HOME`.
- **Cause racine** : depuis la feature 001, `state.DefaultDir` construisait
  `filepath.Join(HOME, ".local", "state", …)` sans vérifier que `HOME` était défini et absolu :
  sans `HOME`, le chemin devenait relatif et `last.json` (ou `hints.json`) s'écrivait dans le
  dossier courant. Dans l'image, ce dossier est celui des manifestes de l'apprenant : monté
  sans `:ro`, il aurait reçu un dossier `.local/state/`.
- **Impact** : aucun en production (détecté avant la première version publiée) ; écriture
  inattendue dans un dossier de l'apprenant, contraire à « `check` n'écrit rien ».
- **Correctif** : `DefaultDir` ne retient qu'un chemin absolu (sinon aucun dossier) ; un
  `Store` sans dossier ne lit ni n'écrit rien (`ErrNoStateDir`) ; l'image lance le plugin sans
  `HOME` ni `XDG_STATE_HOME`.
- **Règle préventive** : tout chemin construit à partir d'une variable d'environnement est
  vérifié absolu avant usage ; un programme qui peut tourner dans un conteneur est testé sans
  `HOME`.
- **Suivi** : `TestNoStateDir` (`internal/state/state_test.go`, dossier courant vérifié vide)
  et `TestPassThrough` (`cmd/telemach-image/main_test.go`, environnement du plugin sans
  `HOME`).

### LL-009 — Code de sortie perdu par zsh après un trap

- **Date** : 2026-10-19
- **Catégorie** : bug
- **Contexte** : feature 002, capture de stderr de `exec`, `attach`, `debug` et `port-forward`
  (US5, research.md S10) : le sous-shell remplace son trap `exit 130` par `:` pour expliquer
  une erreur captée avant un Ctrl-C.
- **Cause racine** : en zsh, une fois le trap exécuté, `pipestatus` du pipeline interrompu
  vaut 0 ; le code 130 de kubectl était perdu, et `explain` n'était pas appelé. bash garde le
  bon code. Aucun test ne lançait le shell dans un vrai terminal : les tests « interactifs »
  de la feature 001 tournaient sans terminal, où Ctrl-C n'existe pas.
- **Impact** : aucun en production (détecté par le premier test en pseudo-terminal) ; une
  erreur de transfert suivie d'un Ctrl-C n'aurait jamais été expliquée sous zsh.
- **Correctif** : le pipeline écrit lui-même le code de kubectl dans
  `<fichier temporaire>.rc`, supprimé par le même trap de sortie ; un pseudo-terminal de test
  (`tests/shell/pty_test.go`, sans dépendance nouvelle) vérifie session, écho, code et Ctrl-C
  sous bash et zsh.
- **Règle préventive** : tout comportement shell qui dépend d'un terminal (Ctrl-C, `-t`, écho)
  se teste dans un vrai pseudo-terminal, sous bash et zsh ; ne jamais se fier à
  `$?`/`PIPESTATUS`/`pipestatus` après l'exécution d'un trap.
- **Suivi** : `TestInteractiveSessionCaptured` et `TestInteractiveInterrupt`
  (`tests/shell/interactive_test.go`), répétés 15 fois sous `-race` sans échec.

### LL-008 — Attentes écrites d'après un message supposé ou une seule version

- **Date** : 2026-10-19
- **Catégorie** : processus
- **Contexte** : feature 002, scénarios W8 à W13 et entrées du catalogue de l'US5.
- **Cause racine** : plusieurs attentes avaient été rédigées avant les relevés, ou d'après la
  seule version 1.37 : W8 supposait la classe d'Ingress `demo` présente, W10 supposait les
  scénarios précédents dans le même namespace, W12 et research.md S11 citaient des messages
  qui n'existent plus en 1.35 à 1.37 (« unable to upgrade connection », « a container name
  must be specified »), et W12 exigeait la liste des conteneurs, absente du message de 1.35.
- **Impact** : un `integration-kind` rouge (1.35) sur la PR #4 ; sans les relevés, deux
  entrées du catalogue auraient reconnu des messages qu'aucun apprenant ne verra.
- **Correctif** : scénarios réécrits d'après les relevés, attentes valables sur les trois
  versions, entrée `cmdline.container-required` abandonnée (research.md S11, écarts).
- **Règle préventive** : avant d'écrire une attente de test ou un motif, relire le relevé de
  **chaque** version (`<cas>`, `<cas>@1.35`, `<cas>@1.36`) ; un scénario de guide se rejoue
  seul, dans un namespace neuf, sans dépendre d'un autre scénario ni d'un objet du cluster
  qu'il n'a pas créé.
- **Suivi** : gate `integration-kind` sur les trois versions (elle a détecté l'écart de 1.35) ;
  la relecture des variantes reste humaine.

### LL-007 — Binaire compilé versionné par « git add -A »

- **Date** : 2026-10-19
- **Catégorie** : processus
- **Contexte** : feature 002, US3 ; `go build ./scripts/...` lancé pour vérifier une
  modification du collecteur, puis `git add -A` avant un commit poussé (eb011ea).
- **Cause racine** : `go build` d'un paquet `main` dépose son exécutable dans le dossier
  courant ; seul `/kubectl-telemach` était ignoré par `.gitignore`, et la liste des fichiers
  ajoutés n'a pas été relue avant le commit.
- **Impact** : un exécutable de 10 Mo dans l'historique de la branche (aucune donnée
  sensible) ; retiré au commit suivant, absent de `main` grâce à la fusion par squash.
- **Correctif** : binaire retiré ; `.gitignore` couvre les exécutables de `cmd/` et
  `scripts/`.
- **Règle préventive** : vérifier une compilation sans écrire de fichier
  (`go vet`, ou `go build -o /dev/null`) ; relire `git status` avant tout `git add -A`.
- **Suivi** : `TestNoBinaryCommitted` (`tests/manifests/hygiene_test.go`) échoue sur tout
  fichier versionné à en-tête ELF, Mach-O ou PE.

### LL-006 — Objectif de latence mesuré sous le détecteur de courses

- **Date** : 2026-10-17
- **Catégorie** : CI/gates
- **Contexte** : feature 002, ajout de contrôles au chargement du catalogue (FR-032).
- **Cause racine** : `TestExplainLatency` (objectif interne de SC-003 : 200 ms au p95) tournait
  dans la gate `test`, compilée avec `-race`, qui ralentit l'exécution d'un facteur 5 à 10.
  La marge était déjà presque épuisée ; quelques entrées et contrôles de plus ont suffi à
  dépasser le seuil (225 ms en CI), alors que le binaire réel reste à 7,5 ms.
- **Impact** : gate `test` rouge sur la PR #3 ; risque de « corriger » par une hausse du
  seuil, ou d'optimiser à l'aveugle un code qui n'est pas lent.
- **Correctif** : mesure ignorée sous `-race` (étiquette de build `race`) et lancée dans une
  étape dédiée de la gate `test`, sans `-race` ; pré-filtre `strings.Contains` avant
  l'expression régulière du contrôle FR-032.
- **Règle préventive** : un objectif de performance se mesure sur le binaire tel qu'il est
  livré, jamais sous un outil d'instrumentation ; avant d'ajuster un seuil ou d'optimiser,
  comparer avant et après avec un benchmark et un profil.
- **Complément (2026-10-19)** : `TestWatchLatency` (SC-007) a dépassé 50 ms en CI (bash,
  58 ms au p95) alors qu'il mesure 4 à 6 ms en local, avant comme après le changement en
  cause : il tournait en même temps que la mesure de SC-004, qui lance des processus pendant
  20 s. Les mesures de latence s'exécutent paquet par paquet (`go test -p 1`), jamais en
  parallèle d'une autre charge.
- **Complément 2 (2026-10-19)** : `TestWatchLatency` a de nouveau dépassé 50 ms en CI, avec
  `-p 1` (78 ms sur `main`, puis 103 et 130 ms sur la PR #5), contre 3 à 19 ms en local et à la
  relance. Cause : sur 20 mesures, l'indice du « p95 » (`20*95/100`) désignait le **maximum** ;
  un seul pic du runner suffisait. La mesure porte désormais sur 100 itérations, dont le p95
  écarte les 5 plus lentes ; le seuil est inchangé. Règle : un percentile se calcule sur assez
  d'échantillons pour ne pas se confondre avec le maximum (au moins 100 pour un p95).
- **Suivi** : `internal/diagnose/race_on_test.go`, `latencyRuns` dans
  `tests/shell/watch_test.go` et étape « Objectifs de latence » de `.github/workflows/ci.yml`.

### LL-005 — Dossier de scénarios appliqué en entier avec son fichier d'attentes

- **Date** : 2026-10-17
- **Catégorie** : processus
- **Contexte** : préparation des scénarios de la feature 002 (T002), en relisant ceux de la
  feature 001 à la lumière de LL-004.
- **Cause racine** : `testdata/pods/scenarios/` contenait `expected.yaml` (défauts tolérés par
  manifest-lint). Le scénario V13 du quickstart 001 lance
  `kubectl apply -f testdata/pods/scenarios/` : kubectl lit aussi ce fichier, sans `apiVersion`
  ni `kind`, et sort en code 1 alors que les pods sont créés. Le test d'intégration appliquait
  les fichiers un par un : la commande du guide n'était donc pas celle du test (même cause que
  LL-004, détectée pendant sa correction mais pas cherchée ailleurs).
- **Impact** : aucun en production ; un apprenant qui suit le guide voit une erreur parasite.
- **Correctif** : fichier déplacé en `testdata/pods/scenarios.expected.yaml` (hors du dossier),
  contrôle `manifest-lint` adapté ; même règle pour `testdata/objects/scenarios/`.
- **Règle préventive** : un dossier de scénarios ne contient que des objets Kubernetes ; quand
  une leçon révèle un défaut, chercher le même défaut dans tout le dépôt avant de clore
  l'incident.
- **Suivi** : `TestScenarioDirsOnlyManifests` (`tests/manifests/lint_test.go`) échoue si un
  fichier d'un dossier de scénarios n'a pas d'`apiVersion` et de `kind`.

### LL-004 — Commande du quickstart jamais exécutée telle qu'écrite

- **Date** : 2026-10-17
- **Catégorie** : processus
- **Contexte** : automatisation des scénarios du quickstart restés manuels (T087).
- **Cause racine** : V10 lançait `kubectl telemach check -f testdata/manifests/plaintext-secret/`.
  Ce dossier contient aussi `expected.yaml`, les attentes du test, que `check` analyse comme un
  manifeste : un problème parasite (« apiVersion manque ») précédait le résultat attendu.
  Aucun test ne rejouait cette commande exacte ; seuls V1, V2, V4, V5, V13, V14 et V15 l'étaient.
- **Impact** : aucun en production ; la personne qui suivait le guide aurait vu un faux problème.
- **Correctif** : V10 vise `secret.yaml` ; V3, V6 à V12, V15 et V17 sont automatisés dans
  `tests/integration/quickstart_test.go`, sur les trois images kind, sous bash et zsh.
- **Règle préventive** : chaque commande d'un guide de validation (quickstart, README) est
  rejouée par un test, avec la forme exacte écrite dans le guide ; un dossier de `testdata/` qui
  mêle manifestes et attentes n'est jamais donné en entier à `check` ou `apply`.
- **Suivi** : `tests/integration/quickstart_test.go` (gate `integration`).

### LL-003 — Clé sensible non masquée au milieu d'un message d'erreur

- **Date** : 2026-10-17
- **Catégorie** : sécurité
- **Contexte** : vérification de SC-006 sur `last.json` (T088), en passant chaque cas du corpus
  de masquage comme message d'erreur précédé de « Error from server (BadRequest): ».
- **Cause racine** : deux motifs de `internal/redact` n'étaient reconnus qu'en début de ligne :
  `NOM_SENSIBLE: valeur` (ancrage voulu pour ne pas masquer « error: You must be logged
  in… ») et `kind: Secret`. Recopiés par kubectl après le « : » d'un message, ils passaient
  en clair : `… (BadRequest): Api_Key: fake-1`, ou les `stringData` d'un Secret.
- **Impact** : aucun en production (détecté avant publication) ; fuite possible d'une valeur
  sensible dans l'explication et dans `last.json`.
- **Correctif** : les deux motifs acceptent aussi une position juste après « : » ; « error:
  invalid token: expired » reste intact. Cas `inline-key` et `inline-secret-kind` ajoutés au
  corpus ; 75 s de fuzzing (`FuzzRedact`, `FuzzMatch`) sans échec.
- **Règle préventive** : un motif de masquage ancré en début de ligne est aussi testé avec un
  préfixe de message kubectl ; chaque cas du corpus de masquage est rejoué de bout en bout,
  jusqu'à `last.json`.
- **Suivi** : `TestCorpusLanguageAndSecrets` (`cmd/kubectl-telemach/language_test.go`) rejoue
  tout `testdata/secrets/` à travers `explain`, `last`, `check` et `diagnose`.

### LL-002 — Gate « secrets » rouge pendant trois commits

- **Date** : 2026-10-17
- **Catégorie** : CI/gates
- **Contexte** : PR #1 de la feature 001 ; la gate est passée au rouge au commit eb1988d (US2),
  et trois commits ont été poussés ensuite sans qu'elle soit relue.
- **Cause racine** : faux positif de gitleaks (`generic-api-key`) sur la table des noms de
  schémas OpenAPI de `scripts/gen-schemas/main.go` (`"v1/Secret": "io.k8s.api.core.v1.…"`) ;
  aucune vérification locale ne rejouait la gate, et l'état des checks n'était pas relu après
  chaque push.
- **Impact** : PR non fusionnable ; risque qu'un vrai secret passe inaperçu derrière une gate
  déjà rouge.
- **Correctif** : empreinte du commit historique dans `.gitleaksignore` (justifiée) et
  commentaire `gitleaks:allow` sur la ligne actuelle ; `scripts/secrets` rejoue la gate en
  local (historique et dossier de travail).
- **Règle préventive** : après chaque push, relire `gh pr checks` ; ne jamais pousser un
  nouveau commit par-dessus une gate rouge sans l'avoir d'abord comprise. Toute exception
  gitleaks est une empreinte précise et commentée, jamais un chemin entier.
- **Suivi** : `scripts/secrets`, à enchaîner par `&&` avant `git commit` (D-LL-008) ; voir
  `CONTRIBUTING.md`.

### LL-001 — Masquage de --token déclenché au milieu d'un mot

- **Date** : 2026-10-16
- **Catégorie** : bug
- **Contexte** : ajout de la cible de fuzzing FuzzMatch (T086), avant toute publication.
- **Cause racine** : les motifs de `internal/redact` pour `--token`, `--password`,
  `--docker-password` et `--from-literal` n'étaient pas ancrés sur un début de mot : « 0--token
  00 » était masqué en texte libre mais pas comme argument, ce qui rendait le masquage
  incohérent entre la ligne de commande et la commande corrigée.
- **Impact** : aucun en production (détecté par le fuzzing) ; risque de corrections affichées
  différemment masquées.
- **Correctif** : motifs ancrés sur le début du texte, une espace ou un guillemet.
- **Règle préventive** : tout motif de masquage d'une option commence par `(?:^|[\s'"])` ; toute
  nouvelle règle de masquage reçoit une cible ou un cas de fuzzing.
- **Suivi** : cas `internal/diagnose/testdata/fuzz/FuzzMatch/71c034aa449f1db2` conservé comme test
  de non-régression ; fuzzing hebdomadaire (`.github/workflows/fuzz.yml`).
