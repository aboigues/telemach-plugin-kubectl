package testcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// ObjectFixture rejoue un cas de testdata/objects/<cas>/ : chaque lecture lit le fichier relevé sur
// kind ; un fichier absent vaut « introuvable », un fichier .err rejoue l'échec relevé ;
// denied simule un refus RBAC pour les lectures nommées.
type ObjectFixture struct {
	Dir    string
	Denied map[string]bool
	Calls  []string
	mu     sync.Mutex // les diagnostics lisent en parallèle (research.md S8)
}

func (f *ObjectFixture) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, call)
}

func (f *ObjectFixture) load(file string, v any) error {
	f.record(file)
	if f.Denied[file] {
		return &kuberead.ErrDenied{What: file, Message: "Error from server (Forbidden): " + file}
	}
	if data, err := os.ReadFile(filepath.Join(f.Dir, file+".err")); err == nil { //nolint:gosec // corpus du dépôt
		return &kuberead.ErrKubectl{ExitCode: 1, Stderr: string(data)}
	}
	data, err := os.ReadFile(filepath.Join(f.Dir, file)) //nolint:gosec // corpus du dépôt
	if err != nil {
		return &kuberead.ErrKubectl{ExitCode: 1, Stderr: "Error from server (NotFound): " + file + " not found"}
	}
	return json.Unmarshal(data, v)
}

// named choisit le fichier d'un objet : <type>.json pour l'objet principal du cas, sinon
// <type>-<nom>.json.
func (f *ObjectFixture) named(kind, name, mainName string) string {
	if name == mainName {
		return kind + ".json"
	}
	return kind + "-" + name + ".json"
}

func (f *ObjectFixture) mainName(kind string) string {
	var obj struct {
		Metadata struct{ Name string } `json:"metadata"`
	}
	data, _ := os.ReadFile(filepath.Join(f.Dir, kind+".json")) //nolint:gosec // corpus du dépôt
	_ = json.Unmarshal(data, &obj)
	return obj.Metadata.Name
}

func (f *ObjectFixture) GetPod(name string) (kuberead.Pod, error) {
	var v kuberead.Pod
	return v, f.load("pod-"+name+".json", &v)
}

func (f *ObjectFixture) ListPods() (kuberead.PodList, error) {
	var v kuberead.PodList
	return v, f.load("pods.json", &v)
}

func (f *ObjectFixture) PodsBySelector(string) (kuberead.PodList, error) {
	var v kuberead.PodList
	return v, f.load("selected-pods.json", &v)
}

func (f *ObjectFixture) PodEvents(pod string) (kuberead.EventList, error) {
	var v kuberead.EventList
	err := f.load("events-"+pod+".json", &v)
	if kuberead.IsNotFound(err) {
		return v, nil
	}
	return v, err
}

func (f *ObjectFixture) Logs(string, string, bool) ([]string, error) { return nil, nil }

func (f *ObjectFixture) GetService(name string) (kuberead.Service, error) {
	var v kuberead.Service
	return v, f.load(f.named("service", name, f.mainName("service")), &v)
}

func (f *ObjectFixture) ListServices() (kuberead.ServiceList, error) {
	var v kuberead.ServiceList
	return v, f.load("services.json", &v)
}

func (f *ObjectFixture) ServiceEndpointSlices(string) (kuberead.EndpointSliceList, error) {
	var v kuberead.EndpointSliceList
	return v, f.load("endpointslices.json", &v)
}

func (f *ObjectFixture) ListEndpointSlices() (kuberead.EndpointSliceList, error) {
	var v kuberead.EndpointSliceList
	return v, f.load("all-endpointslices.json", &v)
}

func (f *ObjectFixture) GetDeployment(string) (kuberead.Deployment, error) {
	var v kuberead.Deployment
	return v, f.load("deployment.json", &v)
}

func (f *ObjectFixture) ListDeployments() (kuberead.DeploymentList, error) {
	var v kuberead.DeploymentList
	return v, f.load("deployments.json", &v)
}

func (f *ObjectFixture) ReplicaSetsBySelector(string) (kuberead.ReplicaSetList, error) {
	var v kuberead.ReplicaSetList
	return v, f.load("replicasets.json", &v)
}

func (f *ObjectFixture) GetIngress(string) (kuberead.Ingress, error) {
	var v kuberead.Ingress
	return v, f.load("ingress.json", &v)
}

func (f *ObjectFixture) ListIngresses() (kuberead.IngressList, error) {
	var v kuberead.IngressList
	return v, f.load("ingresses.json", &v)
}

func (f *ObjectFixture) ListIngressClasses() (kuberead.IngressClassList, error) {
	var v kuberead.IngressClassList
	return v, f.load("ingressclasses.json", &v)
}

func (f *ObjectFixture) GetPVC(string) (kuberead.PVC, error) {
	var v kuberead.PVC
	return v, f.load("pvc.json", &v)
}

func (f *ObjectFixture) ListPVCs() (kuberead.PVCList, error) {
	var v kuberead.PVCList
	return v, f.load("pvcs.json", &v)
}

func (f *ObjectFixture) ListStorageClasses() (kuberead.StorageClassList, error) {
	var v kuberead.StorageClassList
	return v, f.load("storageclasses.json", &v)
}

func (f *ObjectFixture) ConfigMapDates(name string) (kuberead.ConfigMapDates, error) {
	f.record("configmap-dates.txt")
	data, err := os.ReadFile(filepath.Join(f.Dir, "configmap-dates.txt")) //nolint:gosec // corpus du dépôt
	if err != nil {
		return kuberead.ConfigMapDates{Name: name}, &kuberead.ErrKubectl{ExitCode: 1, Stderr: "Error from server (NotFound): configmaps " + name}
	}
	return kuberead.ParseConfigMapDates(name, string(data)), nil
}
