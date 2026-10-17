package kuberead

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// forbiddenFields sont les champs JSON qui porteraient une valeur sensible : aucun type lu du
// cluster ne doit les déclarer (FR-018, research.md S2).
var forbiddenFields = map[string]bool{"data": true, "binaryData": true, "stringData": true, "value": true}

// TestNoSensitiveField parcourt par réflexion tous les types lus du cluster.
func TestNoSensitiveField(t *testing.T) {
	types := []any{
		Pod{},
		PodList{},
		Deployment{},
		DeploymentList{},
		ReplicaSetList{},
		Service{},
		ServiceList{},
		EndpointSliceList{},
		Ingress{},
		IngressList{},
		IngressClassList{},
		PVC{},
		PVCList{},
		StorageClassList{},
		ConfigMapDates{},
		EventList{},
	}
	seen := map[reflect.Type]bool{}
	var walk func(rt reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if forbiddenFields[tag] {
				t.Errorf("%s.%s : champ %q interdit (valeur sensible)", path, f.Name, tag)
			}
			if tag == "annotations" && f.Type != reflect.TypeOf(KnownAnnotations{}) {
				t.Errorf("%s.%s : les annotations ne se décodent que par KnownAnnotations", path, f.Name)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	for _, v := range types {
		walk(reflect.TypeOf(v), reflect.TypeOf(v).Name())
	}
}

func TestDecodeObjects(t *testing.T) {
	var svc Service
	mustDecode(t, `{"metadata":{"name":"web"},"spec":{"type":"ClusterIP","selector":{"app":"web"},
		"ports":[{"port":80,"targetPort":"http"},{"port":443,"targetPort":8443,"nodePort":30443}]}}`, &svc)
	if !svc.Spec.Ports[0].TargetPort.IsNamed() || svc.Spec.Ports[0].TargetPort.String() != "http" ||
		svc.Spec.Ports[1].TargetPort.Int != 8443 || svc.Spec.Ports[1].NodePort != 30443 {
		t.Errorf("ports mal lus : %+v", svc.Spec.Ports)
	}
	var eps EndpointSliceList
	mustDecode(t, `{"items":[{"metadata":{"labels":{"kubernetes.io/service-name":"web"}},
		"endpoints":[{"conditions":{"ready":true}},{"conditions":{"ready":false}},{"conditions":{}}]}]}`, &eps)
	if eps.Items[0].ServiceName() != "web" || eps.Items[0].ReadyEndpoints() != 2 {
		t.Errorf("endpoints mal comptés : %+v", eps)
	}
	var pvc PVC
	mustDecode(t, `{"metadata":{"name":"a"},"spec":{"storageClassName":""},"status":{"phase":"Pending"}}`, &pvc)
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "" {
		t.Error("storageClassName vide doit se distinguer d'un champ absent")
	}
	var pvc2 PVC
	mustDecode(t, `{"metadata":{"name":"b"},"spec":{}}`, &pvc2)
	if pvc2.Spec.StorageClassName != nil {
		t.Error("storageClassName absent : nil attendu")
	}
	var rs ReplicaSet
	mustDecode(t, `{"metadata":{"name":"web-1","ownerReferences":[{"kind":"Deployment","name":"web"}],
		"annotations":{"deployment.kubernetes.io/revision":"2"}}}`, &rs)
	if !rs.OwnedBy("web") || rs.OwnedBy("autre") || rs.Metadata.Annotations.Revision != 2 {
		t.Errorf("ReplicaSet mal lu : %+v", rs)
	}
}

func TestParseConfigMapDates(t *testing.T) {
	d := ParseConfigMapDates("conf", "2026-10-17T10:00:00Z\n2026-10-17T12:30:00Z\nMODE=prod\n\n")
	if len(d.Modified) != 2 || !d.Latest().Equal(time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("dates mal lues : %+v", d)
	}
	if strings.Contains(goString(d), "prod") {
		t.Error("une ligne qui n'est pas une date ne doit pas être gardée")
	}
}

// TestCorpusObjectsDecode décode chaque objet réel relevé sur kind (testdata/objects/) et
// vérifie qu'aucune valeur sensible du relevé n'atteint les types décodés.
func TestCorpusObjectsDecode(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/objects/*/*.json")
	if len(files) == 0 {
		t.Fatal("corpus testdata/objects/ vide (D-LL-009)")
	}
	targets := map[string]func() any{
		"service":        func() any { return &Service{} },
		"endpointslices": func() any { return &EndpointSliceList{} },
		"pods":           func() any { return &PodList{} },
		"selected-pods":  func() any { return &PodList{} },
		"deployment":     func() any { return &Deployment{} },
		"replicasets":    func() any { return &ReplicaSetList{} },
		"ingress":        func() any { return &Ingress{} },
		"ingressclasses": func() any { return &IngressClassList{} },
		"pvc":            func() any { return &PVC{} },
		"storageclasses": func() any { return &StorageClassList{} },
		"service-hello":  func() any { return &Service{} },
		"service-docs":   func() any { return &Service{} },
	}
	decoded := 0
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".json")
		mk, ok := targets[base]
		if !ok {
			t.Errorf("%s : fichier de corpus inattendu", f)
			continue
		}
		data, err := os.ReadFile(f) //nolint:gosec // corpus du dépôt
		if err != nil {
			t.Fatal(err)
		}
		v := mk()
		if err := json.Unmarshal(data, v); err != nil {
			t.Errorf("%s : %v", f, err)
			continue
		}
		decoded++
		if dump := goString(v); strings.Contains(dump, "fake-token") || strings.Contains(dump, "last-applied") {
			t.Errorf("%s : valeur sensible décodée", f)
		}
	}
	if decoded == 0 {
		t.Fatal("aucun objet décodé (D-LL-009)")
	}
}

func mustDecode(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatal(err)
	}
}
