package kuberead

import (
	"encoding/json"
	"strconv"
	"time"
)

// Clés des seules annotations décodées (research.md S2) : publiques, posées par Kubernetes ou
// par l'administrateur du cluster, sans donnée de l'apprenant.
const (
	annotationDefaultIngressClass = "ingressclass.kubernetes.io/is-default-class"
	annotationDefaultStorageClass = "storageclass.kubernetes.io/is-default-class"
	annotationIngressClass        = "kubernetes.io/ingress.class"
	annotationRevision            = "deployment.kubernetes.io/revision"
)

// KnownAnnotations garde les quatre annotations utiles au diagnostic ; toute autre clé, dont
// kubectl.kubernetes.io/last-applied-configuration qui recopie le manifeste et ses secrets, est
// ignorée au décodage et n'est jamais conservée.
type KnownAnnotations struct {
	DefaultIngressClass bool   // classe d'Ingress par défaut
	DefaultStorageClass bool   // classe de stockage par défaut
	IngressClass        string // ancienne façon de choisir la classe d'un Ingress (dépréciée)
	Revision            int    // révision d'un ReplicaSet (0 : inconnue)
}

// UnmarshalJSON ne lit que les quatre clés connues ; une valeur inattendue est ignorée.
func (a *KnownAnnotations) UnmarshalJSON(data []byte) error {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	text := func(key string) string {
		var v string
		if raw, ok := all[key]; ok {
			_ = json.Unmarshal(raw, &v)
		}
		return v
	}
	*a = KnownAnnotations{
		DefaultIngressClass: text(annotationDefaultIngressClass) == "true",
		DefaultStorageClass: text(annotationDefaultStorageClass) == "true",
		IngressClass:        text(annotationIngressClass),
	}
	if n, err := strconv.Atoi(text(annotationRevision)); err == nil && n > 0 {
		a.Revision = n
	}
	return nil
}

// ObjectMetaAnnotated est ObjectMeta avec les seules annotations connues.
type ObjectMetaAnnotated struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences"`
	Annotations       KnownAnnotations  `json:"annotations"`
}
