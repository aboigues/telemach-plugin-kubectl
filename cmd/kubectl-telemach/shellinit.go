package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aboigues/telemach-plugin-kubectl/internal/shellinit"
)

func (a *app) shellInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-init bash|zsh",
		Short: "Affiche le code à évaluer pour activer l'explication automatique des erreurs",
		Long: `Affiche le code d'intégration à ajouter à ton fichier de configuration du shell :

  eval "$(kubectl-telemach shell-init zsh)"    # dans ~/.zshrc
  eval "$(kubectl-telemach shell-init bash)"   # dans ~/.bashrc

Chaque commande kubectl en échec est alors expliquée, y compris quand « kubectl » est un
alias (microk8s kubectl, minikube kubectl --) ou que tu utilises l'alias « k ». Désactivation
ponctuelle : TELEMACH_DISABLE=1.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			script, err := shellinit.Script(args[0], a.executable(), a.version)
			if err != nil {
				fmt.Fprintf(a.stderr, "kubectl telemach : %v\n", err)
				return exitError{code: 2}
			}
			_, err = fmt.Fprint(a.stdout, script)
			return err
		},
	}
}
