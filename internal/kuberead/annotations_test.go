package kuberead

import (
	"encoding/json"
	"strings"
	"testing"
)

// research.md S2 : seules quatre annotations publiques posées par Kubernetes sont décodées ;
// toute autre annotation, dont last-applied-configuration, est ignorée dès la lecture.
func TestKnownAnnotationsOnly(t *testing.T) {
	raw := `{"metadata":{"name":"x","annotations":{
		"ingressclass.kubernetes.io/is-default-class":"true",
		"storageclass.kubernetes.io/is-default-class":"true",
		"kubernetes.io/ingress.class":"nginx",
		"deployment.kubernetes.io/revision":"3",
		"kubectl.kubernetes.io/last-applied-configuration":"{\"password\":\"s3cret-fake\"}",
		"example.com/token":"s3cret-fake"}}}`
	var obj struct {
		Metadata ObjectMetaAnnotated `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		t.Fatal(err)
	}
	a := obj.Metadata.Annotations
	if !a.DefaultIngressClass || !a.DefaultStorageClass || a.IngressClass != "nginx" || a.Revision != 3 {
		t.Errorf("annotations connues mal lues : %+v", a)
	}
	if dump := goString(obj); strings.Contains(dump, "s3cret") || strings.Contains(dump, "last-applied") {
		t.Errorf("annotation inconnue gardée en mémoire : %s", dump)
	}
	// Valeurs inattendues : ignorées sans erreur.
	var bad struct {
		Metadata ObjectMetaAnnotated `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(`{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"abc","storageclass.kubernetes.io/is-default-class":"oui"}}}`), &bad); err != nil {
		t.Fatal(err)
	}
	if bad.Metadata.Annotations.Revision != 0 || bad.Metadata.Annotations.DefaultStorageClass {
		t.Errorf("valeur inattendue interprétée : %+v", bad.Metadata.Annotations)
	}
}
