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
- **Suivi** : `internal/diagnose/race_on_test.go` et étape « Objectifs de latence » de
  `.github/workflows/ci.yml`.

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
