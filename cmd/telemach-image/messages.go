package main

import (
	"fmt"
	"strings"

	"github.com/aboigues/telemach-plugin-kubectl/internal/render"
)

// imageRef renvoie la référence complète de l'image, avec un tag explicite (principe III).
func imageRef(tag string) string { return "telemachlearning/telemach-kubectl:" + tag }

// Commandes canoniques (contracts/image-cli.md), à copier telles quelles.
func tryCommand(tag string) string {
	return `docker run --rm -it -v "$PWD":/manifestes:ro ` + imageRef(tag) + " check"
}

func installCommand(tag, hostOS string) string {
	env := ""
	if hostOS == "darwin" {
		env = "-e HOST_OS=darwin "
	}
	return `docker run --rm --user "$(id -u):$(id -g)" ` + env + "-v ~/.local/bin:/sortie " + imageRef(tag) + " install"
}

const mkdirCommand = "mkdir -p ~/.local/bin"

// indent décale chaque ligne de n espaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func helpText(t render.Theme, tag string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", t.Title("TELEMACH — explique en français les erreurs kubectl et ce qui ne marche pas"))
	fmt.Fprintf(&b, "%s\n\n", t.Dim("Image "+imageRef(tag)+" : essayer TELEMACH ou l'installer sur ta machine."))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Essayer sans rien installer (vérifier les manifestes du dossier courant, sans cluster)"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(tryCommand(tag)), 3))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Installer le plugin — Linux et WSL2"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(mkdirCommand+"\n"+installCommand(tag, "linux")), 3))

	fmt.Fprintf(&b, "%s\n", t.Label("▶ Installer le plugin — macOS (Docker Desktop)"))
	fmt.Fprintf(&b, "%s\n\n", indent(t.Fix(mkdirCommand+"\n"+installCommand(tag, "darwin")), 3))

	fmt.Fprintf(&b, "%s\n", t.Concept(render.LabelConcept+" — montage (bind mount)"))
	fmt.Fprintf(&b, "%s\n", indent("L'option -v DOSSIER_MACHINE:DOSSIER_CONTENEUR donne au conteneur accès à un dossier\nprécis de ta machine ; « :ro » le rend accessible en lecture seule.", 3))
	return b.String()
}

// archLabel présente le système et l'architecture installés de façon lisible.
func archLabel(hostOS, arch string) string {
	names := map[string]string{
		"darwin/arm64": "macOS, Apple Silicon",
		"darwin/amd64": "macOS, Intel",
		"linux/amd64":  "Linux, x86-64",
		"linux/arm64":  "Linux, ARM64",
	}
	key := hostOS + "/" + arch
	if name, ok := names[key]; ok {
		return name + " — " + key
	}
	return key
}

func successText(t render.Theme, version, previous string, upToDate bool, hostOS, arch string) string {
	var b strings.Builder
	switch {
	case upToDate:
		fmt.Fprintf(&b, "%s\n", t.Success(render.SymbolSuccess+" TELEMACH "+version+" est déjà à jour dans ~/.local/bin ("+archLabel(hostOS, arch)+")."))
		fmt.Fprintf(&b, "%s\n\n", indent("Le binaire a été réécrit à l'identique.", 2))
	default:
		fmt.Fprintf(&b, "%s\n", t.Success(render.SymbolSuccess+" TELEMACH "+version+" installé dans ~/.local/bin ("+archLabel(hostOS, arch)+")."))
		if previous != "" {
			fmt.Fprintf(&b, "%s\n", indent("Version remplacée : "+previous+".", 2))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s\n", t.Concept("📘 Ce qui vient de se passer"))
	fmt.Fprintf(&b, "%s\n\n", indent("Le conteneur a écrit dans un dossier de ta machine grâce au montage\n« -v ~/.local/bin:/sortie » (bind mount), avec ton identité (--user).\nkubectl trouve un plugin par son nom (kubectl-telemach) dans les dossiers du PATH.", 3))
	if hostOS == "linux" && arch != "" {
		fmt.Fprintf(&b, "%s\n", indent(t.Dim("Sur un Mac, relance avec -e HOST_OS=darwin pour installer le binaire macOS."), 3))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s\n", t.Label("➜ Étapes suivantes"))
	fmt.Fprintf(&b, "%s\n", indent("1. Vérifie que kubectl trouve le plugin : "+t.Fix("kubectl plugin list"), 3))
	fmt.Fprintf(&b, "%s\n", indent(t.Dim("   s'il n'y figure pas, ajoute ~/.local/bin à ton PATH :"), 3))
	fmt.Fprintf(&b, "%s\n", indent(t.Fix(`   echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc`), 3))
	fmt.Fprintf(&b, "%s\n", indent(t.Dim("   s'il signale un plugin masqué (installé aussi avec Krew), n'en garde qu'un :\n   kubectl krew uninstall telemach"), 3))
	fmt.Fprintf(&b, "%s\n", indent("2. Active l'explication automatique des erreurs :", 3))
	fmt.Fprintf(&b, "%s\n", indent(t.Fix(`   echo 'eval "$(kubectl-telemach shell-init zsh)"' >> ~/.zshrc`), 3))
	fmt.Fprintf(&b, "%s\n", indent(t.Dim(`   (bash : mêmes lignes dans ~/.bashrc, avec shell-init bash)`), 3))
	fmt.Fprintf(&b, "%s\n", indent("puis ouvre un nouveau terminal et vérifie : "+t.Fix("kubectl telemach version"), 3))
	return b.String()
}

func refusalText(t render.Theme, kind refusalKind, tag, hostOS, detail string) string {
	var title, concept, explain string
	var fix []string
	install := installCommand(tag, hostOS)
	switch kind {
	case refuseNotMounted:
		title = "Aucun dossier de destination : /sortie n'est pas monté."
		concept = "montage (bind mount)"
		explain = "Un conteneur ne voit pas les fichiers de ta machine. L'option -v DOSSIER_MACHINE:/sortie\nlui donne accès à un dossier précis, ici un dossier de ton PATH (~/.local/bin)."
		fix = []string{mkdirCommand, install}
	case refuseRootOwned:
		title = "Le dossier de destination appartient à root : TELEMACH ne peut pas y écrire."
		concept = "dossier créé par Docker"
		explain = "Quand le dossier monté n'existe pas, Docker le crée lui-même, avec le compte root.\nC'est pour cela que la commande commence par mkdir -p : le dossier t'appartient."
		fix = []string{`sudo chown "$(id -u):$(id -g)" ~/.local/bin`, install}
	case refuseNotWritable:
		title = "Le dossier de destination n'est pas accessible en écriture pour le conteneur."
		concept = "identité du processus (--user)"
		explain = "Le conteneur s'exécute sans privilèges, avec un utilisateur qui n'est pas toi.\nL'option --user \"$(id -u):$(id -g)\" lui donne ton identité : il peut alors écrire\ndans ton dossier, et le fichier installé t'appartient."
		fix = []string{install}
	case refuseReadOnly:
		title = "Le dossier de destination est monté en lecture seule : TELEMACH ne peut pas y écrire."
		concept = "montage en lecture seule (:ro)"
		explain = "Le suffixe :ro de l'option -v interdit au conteneur toute écriture dans le dossier\nmonté. Il protège tes manifestes pendant un essai (check), mais install doit copier\nle plugin : monte ~/.local/bin sans :ro."
		fix = []string{install}
	case refuseSymlink:
		title = "~/.local/bin/kubectl-telemach existe déjà sous forme de lien symbolique."
		concept = "une seule installation à la fois"
		explain = "Ce lien pointe vers un autre fichier (une compilation, un autre gestionnaire de\nplugins…). Par sécurité, TELEMACH ne remplace pas un lien. Deux installations du même\nplugin se masquent l'une l'autre dans le PATH : garde-en une seule."
		fix = []string{"ls -l ~/.local/bin/kubectl-telemach", "kubectl plugin list", "# s'il n'est plus utile : rm ~/.local/bin/kubectl-telemach", install}
	case refuseNotRegular:
		title = "~/.local/bin/kubectl-telemach existe déjà mais n'est pas un fichier ordinaire."
		concept = "fichier existant"
		explain = "Un dossier ou un fichier spécial porte déjà ce nom. Par sécurité, TELEMACH ne\nremplace pas ce qu'il ne reconnaît pas."
		fix = []string{"ls -l ~/.local/bin/kubectl-telemach", "# s'il n'est plus utile : rm -r ~/.local/bin/kubectl-telemach", install}
	case refuseForeign:
		title = "~/.local/bin/kubectl-telemach existe déjà et n'est pas un binaire TELEMACH."
		concept = "fichier existant"
		explain = "Par sécurité, TELEMACH ne remplace pas un fichier qu'il ne reconnaît pas."
		fix = []string{"ls -l ~/.local/bin/kubectl-telemach", "# s'il n'est plus utile : rm ~/.local/bin/kubectl-telemach", install}
	case refuseCopyFailed:
		title = "La copie du plugin a échoué : " + detail
		concept = "installation interrompue"
		explain = "Rien n'a été laissé dans le dossier de destination. Vérifie l'espace disque et relance."
		fix = []string{install}
	case refuseWindows:
		title = "Windows n'est pas pris en charge (HOST_OS=windows)."
		concept = "WSL2"
		explain = "Le plugin fonctionne sous Linux et macOS. Sous Windows, utilise-le dans WSL2 :\nlance la commande d'installation depuis ton terminal WSL2, sans HOST_OS."
		fix = []string{mkdirCommand, installCommand(tag, "linux")}
	case refuseUnknownOS:
		title = "Système inconnu : HOST_OS=" + hostOS + "."
		concept = "système cible"
		explain = "HOST_OS indique le système de ta machine : linux (par défaut) ou darwin (macOS)."
		fix = []string{installCommand(tag, "linux"), installCommand(tag, "darwin")}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", t.Anti(render.SymbolError+" "+title))
	fmt.Fprintf(&b, "%s\n", t.Concept(render.LabelConcept+" — "+concept))
	fmt.Fprintf(&b, "%s\n\n", indent(explain, 3))
	fmt.Fprintf(&b, "%s\n", t.Fix("💡 Correction"))
	fmt.Fprintf(&b, "%s\n", indent(strings.Join(fix, "\n"), 3))
	return b.String()
}

func installHelpText(t render.Theme, tag string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", t.Title("install — copie le plugin TELEMACH dans un dossier du PATH de ta machine"))
	fmt.Fprintf(&b, "%s\n", t.Label("Linux et WSL2"))
	fmt.Fprintf(&b, "%s\n\n", indent(mkdirCommand+"\n"+installCommand(tag, "linux"), 3))
	fmt.Fprintf(&b, "%s\n", t.Label("macOS (Docker Desktop)"))
	fmt.Fprintf(&b, "%s\n\n", indent(mkdirCommand+"\n"+installCommand(tag, "darwin"), 3))
	fmt.Fprintf(&b, "%s\n", t.Label("Options"))
	fmt.Fprintf(&b, "%s\n", indent("-v ~/.local/bin:/sortie       dossier de ta machine où copier le plugin (obligatoire, dans le PATH)\n--user \"$(id -u):$(id -g)\"    le conteneur écrit avec ton identité\n-e HOST_OS=darwin             installe le binaire macOS (linux par défaut)", 3))
	return b.String()
}

func noManifestText(t render.Theme, tag, dir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", t.Anti(render.SymbolError+" Aucun manifeste (.yaml, .yml ou .json) trouvé dans "+dir+"."))
	fmt.Fprintf(&b, "%s\n", t.Concept(render.LabelConcept+" — montage (bind mount)"))
	fmt.Fprintf(&b, "%s\n\n", indent("Le conteneur ne voit que les dossiers montés avec -v. Lance la commande depuis le\ndossier de tes manifestes : -v \"$PWD\":/manifestes:ro le rend visible dans le\nconteneur, en lecture seule. Aucun cluster n'est contacté.", 3))
	fmt.Fprintf(&b, "%s\n", t.Fix("💡 Correction"))
	fmt.Fprintf(&b, "%s\n", indent("cd chemin/vers/tes/manifestes\n"+tryCommand(tag), 3))
	return b.String()
}
