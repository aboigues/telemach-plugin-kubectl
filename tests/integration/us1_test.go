//go:build integration

package integration

import (
	"strings"
	"testing"
)

// TestUS1 rejoue les scénarios V1, V2, V4 et V5 du quickstart avec un vrai kubectl, le vrai
// binaire et l'intégration shell (jamais le binaire en direct : D-LL-014).
func TestUS1(t *testing.T) {
	ns := namespace(t, "us1")
	if err := cluster.Must("run", "web", "--image=nginxinc/nginx-unprivileged:1.30-alpine", "-n", ns); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, ns)
	cases := []struct {
		name, cmd string
		code      string
		want      []string
	}{
		{"V1", "kubectl get deploymnet -n " + ns, "1", []string{"✖ Le type de ressource « deploymnet »", "kubectl get deployment -n " + ns}},
		{"V2", "kubectl get pod web", "1", []string{"est introuvable dans le namespace courant", "kubectl get pods -A"}},
		{"V4", "kubectl get pods -n " + ns + " --as=system:serviceaccount:" + ns + ":default", "1", []string{"RBAC", "kubectl auth can-i list pods -n " + ns, "cluster-admin"}},
		{"V5", "kubectl -n " + ns + " telemach diagnose", "1", []string{"placée avant le nom du plugin", "kubectl telemach diagnose -n " + ns}},
	}
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range cases {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				stdout, stderr := shellRun(t, shell, tc.cmd+`; echo "rc=$?"`)
				if !strings.Contains(stdout, "rc="+tc.code) {
					t.Errorf("code de sortie non préservé : %q", stdout)
				}
				for _, w := range tc.want {
					if !strings.Contains(stderr, w) {
						t.Errorf("explication sans %q :\n%s", w, stderr)
					}
				}
				if !strings.Contains(stderr, "Message kubectl d'origine") {
					t.Errorf("le message d'origine doit rester consultable :\n%s", stderr)
				}
			})
		}
	}
	if after := snapshot(t, ns); after != before {
		t.Errorf("le namespace a été modifié (SC-008) :\navant :\n%s\naprès :\n%s", before, after)
	}
}

// TestUS1KubectlAlias : « kubectl » est un alias vers une autre commande (comme
// « microk8s kubectl ») ; l'erreur est expliquée et la commande de l'alias exécutée (R4).
func TestUS1KubectlAlias(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			if _, err := lookPath(shell); err != nil {
				t.Skip(err)
			}
			body := `alias kubectl='command kubectl'` + "\n" + `alias k=kubectl` + "\n" +
				`eval "$(kubectl-telemach shell-init ` + shell + `)"` + "\n" + `k get pdos; echo "rc=$?"`
			stdout, stderr := shellRunInteractive(t, shell, body)
			if !strings.Contains(stdout, "rc=1") || !strings.Contains(stderr, "k get pods") {
				t.Errorf("stdout %q\nstderr :\n%s", stdout, stderr)
			}
		})
	}
}
