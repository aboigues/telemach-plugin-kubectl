package main

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/schema"
)

// resolveVersion renvoie la version injectée à la compilation ; à défaut, celle que Go
// inscrit dans le binaire (tag git ou pseudo-version lors d'un « go build » depuis un clone).
func resolveVersion(injected string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if injected != "dev" {
		return injected
	}
	if info, ok := readBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return injected
}

// versionTemplate est le texte affiché par « kubectl telemach --version ».
const versionTemplate = "TELEMACH PLUGIN KUBECTL {{.Version}}\n"

// versionCommand affiche la version du plugin et les versions de règles Kubernetes
// embarquées (contracts/cli.md § version).
func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Affiche la version du plugin et des règles Kubernetes embarquées",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			fmt.Fprintf(a.stdout, "TELEMACH PLUGIN KUBECTL %s\n", a.version)
			fmt.Fprintf(a.stdout, "Règles Kubernetes embarquées : %s\n", strings.Join(schema.Supported, ", "))
			return nil
		},
	}
}
