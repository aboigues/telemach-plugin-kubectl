package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

// write crée les fichiers d'une arborescence de test (chemin relatif → contenu).
func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Issue #12 : un dossier de TP qui mêle CRD, chart Helm, fichier de valeurs et configuration
// de kubeadm ne contient aucun problème, seulement des informations ; les gabarits des
// alertes Prometheus ({{ $labels… }}) ne sont pas pris pour un template Helm.
func TestNonManifestFilesAreInformation(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"chart/Chart.yaml":           "apiVersion: v2\nname: telemarche\nversion: 0.1.0\n",
		"chart/values.yaml":          "replicaCount: 2\n",
		"chart/templates/svc.yaml":   "apiVersion: v1\nkind: Service\nmetadata:\n  name: {{ .Release.Name }}-api\n",
		"chart/templates/plain.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: fixe\n",
		"traefik-values.yaml":        "service:\n  type: NodePort\n",
		"kubeadm-config.yaml":        "apiVersion: kubeadm.k8s.io/v1beta4\nkind: ClusterConfiguration\n",
		"alertes.yaml":               "apiVersion: monitoring.coreos.com/v1\nkind: PrometheusRule\nmetadata:\n  name: alertes\nspec:\n  groups:\n  - name: api\n    rules:\n    - alert: ApiDown\n      expr: up == 0\n      annotations:\n        summary: \"{{ $labels.instance }} ne répond plus\"\n",
	})
	r := Analyze([]string{root}, true, "v1.37.0")
	if r.Problems() != 0 {
		t.Errorf("aucun problème attendu, %d trouvés : %+v", r.Problems(), r.Issues)
	}
	want := map[string]string{
		"chart/Chart.yaml":           KindHelmValues,
		"chart/values.yaml":          KindHelmValues,
		"chart/templates/svc.yaml":   KindHelmTemplate,
		"chart/templates/plain.yaml": KindHelmTemplate,
		"traefik-values.yaml":        KindHelmValues,
		"kubeadm-config.yaml":        KindToolConfig,
		"alertes.yaml":               KindUnknownCRD,
	}
	for _, is := range r.Issues {
		rel, _ := filepath.Rel(root, is.File)
		if want[filepath.ToSlash(rel)] != is.Kind {
			t.Errorf("%s : %s, attendu %s", rel, is.Kind, want[rel])
		}
		delete(want, filepath.ToSlash(rel))
		if !is.Info() {
			t.Errorf("%s : %s devrait être une information", rel, is.Kind)
		}
		if is.Kind == KindHelmTemplate && is.Suggestion != filepath.Join(root, "chart") {
			t.Errorf("%s : dossier du chart %q", rel, is.Suggestion)
		}
	}
	for rel, kind := range want {
		t.Errorf("%s : information %s absente", rel, kind)
	}
}

// Un vrai problème reste un problème, même à côté d'informations, et passe en premier.
func TestProblemsBeforeInformation(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"a-values.yaml": "replicaCount: 2\n",
		"b.yaml":        "kind: Pod\nmetadata:\n  name: web\n",
	})
	r := Analyze([]string{root}, false, "v1.37.0")
	if r.Problems() != 1 || len(r.Issues) != 2 || r.Issues[0].Kind != KindMissingAPIVersion || !r.Issues[1].Info() {
		t.Errorf("un problème puis une information attendus : %+v", r.Issues)
	}
}

// Feature 004 (T047, FR-018) : un HPA et sa cible dans les fichiers vérifiés ; un conteneur
// sans request pour la ressource visée est signalé par une information, sur le conteneur.
func TestHPAMissingRequestOffline(t *testing.T) {
	root := t.TempDir()
	deploy := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\nspec:\n  selector:\n    matchLabels: {app: web}\n  template:\n    metadata:\n      labels: {app: web}\n    spec:\n      containers:\n      - name: web\n        image: nginxinc/nginx-unprivileged:1.30-alpine\n        resources:\n          requests: {memory: 32Mi}\n          limits: {memory: 64Mi}\n"
	hpa := "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata:\n  name: web\nspec:\n  scaleTargetRef: {apiVersion: apps/v1, kind: Deployment, name: web}\n  maxReplicas: 3\n  metrics:\n  - type: Resource\n    resource:\n      name: cpu\n      target: {type: Utilization, averageUtilization: 50}\n"
	write(t, root, map[string]string{"deploy.yaml": deploy, "hpa.yaml": hpa, "seul/hpa.yaml": hpa})
	r := Analyze([]string{root}, false, "v1.37.2")
	var found []Issue
	for _, is := range r.Issues {
		if is.Kind == KindHPAMissingRequest {
			found = append(found, is)
		}
	}
	if len(found) != 1 || filepath.Base(found[0].File) != "deploy.yaml" || found[0].Line != 13 || !found[0].Info() || r.Problems() != 0 {
		t.Errorf("une information sur deploy.yaml:13 attendue : %+v", r.Issues)
	}
	// HPA seul : la cible n'est pas dans les fichiers vérifiés, rien n'est dit.
	r = Analyze([]string{filepath.Join(root, "seul")}, false, "v1.37.2")
	for _, is := range r.Issues {
		if is.Kind == KindHPAMissingRequest {
			t.Errorf("rien n'est attendu sans la cible : %+v", is)
		}
	}
}
