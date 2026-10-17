package kuberead

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Types des objets lus par la feature 002 (data-model.md § Types lus). Seuls les champs
// listés sont décodés : aucun ne porte de valeur de Secret, de ConfigMap ni de variable
// d'environnement, et les annotations se limitent à KnownAnnotations.

// IntOrString est un port donné par son numéro ou par son nom (targetPort, port d'un backend).
type IntOrString struct {
	Int int
	Str string
}

// UnmarshalJSON accepte un nombre ou une chaîne.
func (v *IntOrString) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &v.Int); err == nil {
		v.Str = ""
		return nil
	}
	v.Int = 0
	return json.Unmarshal(data, &v.Str)
}

// IsNamed indique un port désigné par son nom.
func (v IntOrString) IsNamed() bool { return v.Str != "" }

// String écrit le port tel que l'apprenant l'a écrit.
func (v IntOrString) String() string {
	if v.IsNamed() {
		return v.Str
	}
	return strconv.Itoa(v.Int)
}

// ServiceList est la réponse de « get services -o json ».
type ServiceList struct {
	Items []Service `json:"items"`
}

// Service est réduit au type, au selector et aux ports.
type Service struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Type         string            `json:"type"`
		Selector     map[string]string `json:"selector"`
		Ports        []ServicePort     `json:"ports"`
		ExternalName string            `json:"externalName"`
	} `json:"spec"`
}

// ServicePort est un port d'un Service.
type ServicePort struct {
	Name       string      `json:"name"`
	Protocol   string      `json:"protocol"`
	Port       int         `json:"port"`
	TargetPort IntOrString `json:"targetPort"`
	NodePort   int         `json:"nodePort"`
}

// EndpointSliceList est la réponse de « get endpointslices -o json ».
type EndpointSliceList struct {
	Items []EndpointSlice `json:"items"`
}

// EndpointSlice regroupe des endpoints d'un Service (remplace Endpoints, déprécié en 1.33).
type EndpointSlice struct {
	Metadata  ObjectMeta `json:"metadata"`
	Endpoints []struct {
		Conditions struct {
			Ready       *bool `json:"ready"`
			Serving     *bool `json:"serving"`
			Terminating *bool `json:"terminating"`
		} `json:"conditions"`
		TargetRef *struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"targetRef"`
	} `json:"endpoints"`
	Ports []struct {
		Name *string `json:"name"`
		Port *int    `json:"port"`
	} `json:"ports"`
}

// ServiceName renvoie le Service propriétaire de la tranche (label kubernetes.io/service-name).
func (e EndpointSlice) ServiceName() string {
	return e.Metadata.Labels["kubernetes.io/service-name"]
}

// ReadyEndpoints compte les endpoints prêts (une condition ready absente vaut prêt, selon
// l'API EndpointSlice).
func (e EndpointSlice) ReadyEndpoints() int {
	n := 0
	for _, ep := range e.Endpoints {
		if ep.Conditions.Ready == nil || *ep.Conditions.Ready {
			n++
		}
	}
	return n
}

// ReplicaSetList est la réponse de « get replicasets -o json ».
type ReplicaSetList struct {
	Items []ReplicaSet `json:"items"`
}

// ReplicaSet est une version d'un Deployment.
type ReplicaSet struct {
	Metadata ObjectMetaAnnotated `json:"metadata"`
	Spec     struct {
		Replicas *int        `json:"replicas"`
		Template PodTemplate `json:"template"`
	} `json:"spec"`
	Status struct {
		Replicas          int `json:"replicas"`
		ReadyReplicas     int `json:"readyReplicas"`
		AvailableReplicas int `json:"availableReplicas"`
	} `json:"status"`
}

// OwnedBy indique si le ReplicaSet appartient au Deployment nommé.
func (rs ReplicaSet) OwnedBy(deployment string) bool {
	for _, o := range rs.Metadata.OwnerReferences {
		if o.Kind == "Deployment" && o.Name == deployment {
			return true
		}
	}
	return false
}

// IngressList est la réponse de « get ingresses -o json ».
type IngressList struct {
	Items []Ingress `json:"items"`
}

// Ingress est réduit à sa classe, à ses règles et à son adresse.
type Ingress struct {
	Metadata ObjectMetaAnnotated `json:"metadata"`
	Spec     struct {
		IngressClassName *string         `json:"ingressClassName"`
		DefaultBackend   *IngressBackend `json:"defaultBackend"`
		Rules            []struct {
			Host string `json:"host"`
			HTTP *struct {
				Paths []struct {
					Path     string         `json:"path"`
					PathType string         `json:"pathType"`
					Backend  IngressBackend `json:"backend"`
				} `json:"paths"`
			} `json:"http"`
		} `json:"rules"`
	} `json:"spec"`
	Status struct {
		LoadBalancer struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
	} `json:"status"`
}

// IngressBackend est la cible d'une règle (Service seulement).
type IngressBackend struct {
	Service *struct {
		Name string `json:"name"`
		Port struct {
			Number int    `json:"number"`
			Name   string `json:"name"`
		} `json:"port"`
	} `json:"service"`
}

// IngressClassList est la réponse de « get ingressclasses -o json ».
type IngressClassList struct {
	Items []IngressClass `json:"items"`
}

// IngressClass désigne le contrôleur qui applique les Ingress de cette classe.
type IngressClass struct {
	Metadata ObjectMetaAnnotated `json:"metadata"`
	Spec     struct {
		Controller string `json:"controller"`
	} `json:"spec"`
}

// PVCList est la réponse de « get pvc -o json ».
type PVCList struct {
	Items []PVC `json:"items"`
}

// PVC est un PersistentVolumeClaim ; StorageClassName distingue absent (nil, classe par
// défaut) et vide (provisionnement dynamique désactivé).
type PVC struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		StorageClassName *string  `json:"storageClassName"`
		AccessModes      []string `json:"accessModes"`
		Resources        struct {
			Requests map[string]string `json:"requests"`
		} `json:"resources"`
		VolumeName string `json:"volumeName"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

// StorageClassList est la réponse de « get storageclasses -o json ».
type StorageClassList struct {
	Items []StorageClass `json:"items"`
}

// StorageClass est une classe de stockage.
type StorageClass struct {
	Metadata          ObjectMetaAnnotated `json:"metadata"`
	Provisioner       string              `json:"provisioner"`
	VolumeBindingMode string              `json:"volumeBindingMode"`
}

// ConfigMapDates ne contient que les dates de modification d'une ConfigMap (research.md S3).
type ConfigMapDates struct {
	Name     string
	Modified []time.Time
}

// Latest renvoie la date de modification la plus récente (zéro si aucune).
func (c ConfigMapDates) Latest() time.Time {
	var max time.Time
	for _, t := range c.Modified {
		if t.After(max) {
			max = t
		}
	}
	return max
}

// ParseConfigMapDates lit la sortie du jsonpath de ConfigMapDatesArgs : une date RFC 3339 par
// ligne. Toute ligne qui n'est pas une date est ignorée (aucun autre contenu n'est gardé).
func ParseConfigMapDates(name, out string) ConfigMapDates {
	d := ConfigMapDates{Name: name}
	for _, line := range strings.Fields(out) {
		if t, err := time.Parse(time.RFC3339, line); err == nil {
			d.Modified = append(d.Modified, t)
		}
	}
	sort.Slice(d.Modified, func(i, j int) bool { return d.Modified[i].Before(d.Modified[j]) })
	return d
}
