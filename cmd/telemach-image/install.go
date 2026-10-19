package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// modulePath identifie un binaire TELEMACH dans les informations de build Go.
const modulePath = "github.com/aboigues/telemach-plugin-kubectl"

// Codes de sortie de install (contracts/image-cli.md).
const (
	exitOK      = 0
	exitRefused = 1 // environnement inadapté ; rien n'a été écrit
	exitUsage   = 2 // HOST_OS invalide, argument inconnu
)

// refusalKind identifie la cause d'un refus d'installation.
type refusalKind int

const (
	refuseNotMounted refusalKind = iota
	refuseRootOwned
	refuseNotWritable
	refuseSymlink
	refuseNotRegular
	refuseForeign
	refuseCopyFailed
	refuseWindows
	refuseUnknownOS
)

// install copie le plugin du système demandé dans le dossier monté de l'hôte (FR-017), ou
// refuse avec une explication pédagogique sans rien modifier (FR-018).
func (a *app) install(args []string) int {
	if len(args) > 0 {
		if args[0] == "--help" || args[0] == "-h" {
			theme, w := a.output(a.stdout)
			fmt.Fprint(w, installHelpText(theme, a.imageTag()))
			return exitOK
		}
		theme, w := a.output(a.stderr)
		fmt.Fprintf(w, "%s\n\n", theme.Anti("✖ Argument inconnu pour install : "+args[0]))
		fmt.Fprint(w, installHelpText(theme, a.imageTag()))
		return exitUsage
	}

	hostOS := strings.ToLower(strings.TrimSpace(a.getenv("HOST_OS")))
	switch hostOS {
	case "":
		hostOS = "linux"
	case "linux", "darwin":
	case "windows":
		return a.refuse(refuseWindows, hostOS, "")
	default:
		return a.refuse(refuseUnknownOS, hostOS, "")
	}

	if !a.isMounted() {
		return a.refuse(refuseNotMounted, hostOS, "")
	}
	if uid, err := a.owner(a.dest); err == nil && uid == 0 && a.getuid() != 0 {
		return a.refuse(refuseRootOwned, hostOS, "")
	}

	target := filepath.Join(a.dest, pluginName)
	previous, kind, refused := a.existingVersion(target)
	if refused {
		return a.refuse(kind, hostOS, "")
	}

	src := filepath.Join(a.root, hostOS, pluginName)
	upToDate := previous != "" && sameContent(src, target)
	if err := a.copyAtomic(src, target); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return a.refuse(refuseNotWritable, hostOS, "")
		}
		return a.refuse(refuseCopyFailed, hostOS, err.Error())
	}

	theme, w := a.output(a.stdout)
	fmt.Fprint(w, successText(theme, a.version, previous, upToDate, hostOS, a.goarch))
	return exitOK
}

// sameContent indique si deux fichiers ont un contenu identique (même binaire).
func sameContent(a, b string) bool {
	x, err := os.ReadFile(a) // #nosec G304 -- chemins fixes (image et destination montée)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b) // #nosec G304
	return err == nil && bytes.Equal(x, y)
}

// isMounted indique si le dossier de destination est un point de montage (bind mount).
func (a *app) isMounted() bool {
	data, err := a.mountinfo()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 4 && unescapeMount(fields[4]) == a.dest {
			return true
		}
	}
	return false
}

// unescapeMount décode les séquences octales (\040 pour une espace) de /proc/self/mountinfo.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// existingVersion examine un éventuel fichier déjà présent : il n'est remplacé que s'il s'agit
// d'un fichier ordinaire reconnu comme binaire TELEMACH. previous est sa version (vide s'il
// n'existe pas).
func (a *app) existingVersion(target string) (previous string, kind refusalKind, refused bool) {
	fi, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, false
	}
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", refuseNotWritable, true
		}
		return "", refuseNotRegular, true
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		// Krew installe le plugin comme lien symbolique vers son magasin (research R7).
		return "", refuseSymlink, true
	}
	if !fi.Mode().IsRegular() {
		return "", refuseNotRegular, true
	}
	info, err := a.buildInfo(target)
	if err != nil || info.Main.Path != modulePath {
		return "", refuseForeign, true
	}
	return displayVersion(info.Main.Version), 0, false
}

// displayVersion présente la version d'un module : sans le « v », « développement » pour
// une compilation hors dépôt git.
func displayVersion(v string) string {
	if v == "" || v == "(devel)" {
		return "développement"
	}
	return strings.TrimPrefix(v, "v")
}

// copyAtomic copie src vers dst via un fichier temporaire du même dossier, renommé une fois
// complet : dst n'est jamais partiellement écrit, et le temporaire est supprimé en cas d'échec.
func (a *app) copyAtomic(src, dst string) (err error) {
	in, err := os.Open(src) // #nosec G304 -- chemin fixe dans l'image
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+pluginName+"-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()           //nolint:errcheck,gosec
			os.Remove(tmp.Name()) //nolint:errcheck,gosec
		}
	}()
	if _, err = io.Copy(tmp, in); err != nil {
		return err
	}
	if err = tmp.Chmod(0o755); err != nil { // #nosec G302 -- un plugin kubectl doit être exécutable
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// fileOwner renvoie l'UID propriétaire d'un fichier.
func fileOwner(path string) (int, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("propriétaire inconnu")
	}
	return int(st.Uid), nil
}

// refuse affiche l'explication d'un refus sur stderr et renvoie le code correspondant.
func (a *app) refuse(kind refusalKind, hostOS, detail string) int {
	theme, w := a.output(a.stderr)
	fmt.Fprint(w, refusalText(theme, kind, a.imageTag(), hostOS, detail))
	if kind == refuseWindows || kind == refuseUnknownOS {
		return exitUsage
	}
	return exitRefused
}
