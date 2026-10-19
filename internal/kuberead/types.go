package kuberead

import "time"

// Types JSON réduits aux champs utiles au diagnostic. Ils ne déclarent volontairement NI la
// valeur des variables d'environnement (env[].value) NI les annotations d'un objet (dont
// kubectl.kubernetes.io/last-applied-configuration, qui recopie tout le manifeste) : le
// décodeur JSON ignore ces champs, qui ne sont donc jamais gardés en mémoire (FR-024,
// research.md R8).

// PodList est la réponse de « get pods -o json ».
type PodList struct {
	Items []Pod `json:"items"`
}

// Pod est un pod réduit.
type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     PodSpec    `json:"spec"`
	Status   PodStatus  `json:"status"`
}

// ObjectMeta est réduit au nom, au namespace, aux labels et au propriétaire.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	Labels            map[string]string `json:"labels"`
	OwnerReferences   []OwnerReference  `json:"ownerReferences"`
	Generation        int               `json:"generation"`
}

// OwnerReference désigne l'objet propriétaire (ReplicaSet d'un deployment, par exemple).
type OwnerReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PodSpec est réduit aux conteneurs, au placement et aux volumes (références seulement).
type PodSpec struct {
	Containers     []Container       `json:"containers"`
	InitContainers []Container       `json:"initContainers"`
	NodeSelector   map[string]string `json:"nodeSelector"`
	RestartPolicy  string            `json:"restartPolicy"`
	Volumes        []Volume          `json:"volumes"`
}

// Volume ne garde que les noms des objets référencés (ConfigMap, Secret, PVC), jamais leur
// contenu (feature 002, data-model.md).
type Volume struct {
	Name      string `json:"name"`
	ConfigMap *struct {
		Name string `json:"name"`
	} `json:"configMap"`
	Secret *struct {
		SecretName string `json:"secretName"`
	} `json:"secret"`
	PersistentVolumeClaim *struct {
		ClaimName string `json:"claimName"`
	} `json:"persistentVolumeClaim"`
}

// ContainerPort est un port déclaré par un conteneur.
type ContainerPort struct {
	Name          string `json:"name"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// VolumeMount est un montage de volume ; SubPath compte pour la configuration figée (S3).
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	SubPath   string `json:"subPath"`
}

// Container est réduit : noms des variables et références aux Secret et ConfigMap, jamais
// les valeurs.
type Container struct {
	Name         string          `json:"name"`
	Image        string          `json:"image"`
	Env          []EnvVar        `json:"env"`
	EnvFrom      []EnvFromSource `json:"envFrom"`
	Ports        []ContainerPort `json:"ports"`
	VolumeMounts []VolumeMount   `json:"volumeMounts"`
	Resources    struct {
		Requests map[string]string `json:"requests"`
		Limits   map[string]string `json:"limits"`
	} `json:"resources"`
}

// EnvVar ne garde que le nom et la référence (pas de champ Value).
type EnvVar struct {
	Name      string        `json:"name"`
	ValueFrom *EnvVarSource `json:"valueFrom"`
}

// EnvVarSource référence une clé d'un Secret ou d'une ConfigMap.
type EnvVarSource struct {
	SecretKeyRef    *KeyRef `json:"secretKeyRef"`
	ConfigMapKeyRef *KeyRef `json:"configMapKeyRef"`
}

// KeyRef désigne une clé d'un objet.
type KeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// EnvFromSource importe toutes les clés d'un Secret ou d'une ConfigMap.
type EnvFromSource struct {
	SecretRef    *NameRef `json:"secretRef"`
	ConfigMapRef *NameRef `json:"configMapRef"`
}

// NameRef désigne un objet par son nom.
type NameRef struct {
	Name string `json:"name"`
}

// PodStatus est le statut observé du pod.
type PodStatus struct {
	StartTime             *time.Time        `json:"startTime"`
	Phase                 string            `json:"phase"`
	Reason                string            `json:"reason"`
	Message               string            `json:"message"`
	Conditions            []PodCondition    `json:"conditions"`
	ContainerStatuses     []ContainerStatus `json:"containerStatuses"`
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses"`
}

// PodCondition est une condition (PodScheduled, Ready…).
type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// ContainerStatus est l'état d'un conteneur.
type ContainerStatus struct {
	Name         string         `json:"name"`
	Image        string         `json:"image"`
	Ready        bool           `json:"ready"`
	RestartCount int            `json:"restartCount"`
	State        ContainerState `json:"state"`
	LastState    ContainerState `json:"lastState"`
}

// ContainerState : un seul des trois champs est renseigné.
type ContainerState struct {
	Waiting    *StateDetail `json:"waiting"`
	Running    *Running     `json:"running"`
	Terminated *StateDetail `json:"terminated"`
}

// Running est l'état d'un conteneur en cours d'exécution.
type Running struct {
	StartedAt time.Time `json:"startedAt"`
}

// StateDetail détaille un état d'attente ou de fin.
type StateDetail struct {
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	ExitCode   int       `json:"exitCode"`
	FinishedAt time.Time `json:"finishedAt"`
}

// Deployment est réduit au selector, au modèle de pod (conteneurs sans valeur d'env) et à
// l'état du rollout (feature 002, research.md S5).
type Deployment struct {
	Metadata ObjectMeta       `json:"metadata"`
	Spec     DeploymentSpec   `json:"spec"`
	Status   DeploymentStatus `json:"status"`
}

// DeploymentList est la réponse de « get deployments -o json ».
type DeploymentList struct {
	Items []Deployment `json:"items"`
}

// DeploymentSpec est la partie utile de spec.
type DeploymentSpec struct {
	Replicas *int `json:"replicas"`
	Selector struct {
		MatchLabels map[string]string `json:"matchLabels"`
	} `json:"selector"`
	Paused                  bool        `json:"paused"`
	ProgressDeadlineSeconds *int        `json:"progressDeadlineSeconds"`
	Template                PodTemplate `json:"template"`
}

// PodTemplate est le modèle de pod d'un Deployment ou d'un ReplicaSet.
type PodTemplate struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec PodSpec `json:"spec"`
}

// DeploymentStatus est l'état observé d'un rollout.
type DeploymentStatus struct {
	ObservedGeneration int                   `json:"observedGeneration"`
	Replicas           int                   `json:"replicas"`
	UpdatedReplicas    int                   `json:"updatedReplicas"`
	ReadyReplicas      int                   `json:"readyReplicas"`
	AvailableReplicas  int                   `json:"availableReplicas"`
	Conditions         []DeploymentCondition `json:"conditions"`
}

// DeploymentCondition est une condition d'un Deployment (Available, Progressing…).
type DeploymentCondition struct {
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`
	LastUpdateTime time.Time `json:"lastUpdateTime"`
}

// EventList est la réponse de « get events -o json ».
type EventList struct {
	Items []Event `json:"items"`
}

// Event est un événement réduit.
type Event struct {
	Type           string    `json:"type"`
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`
	Count          int       `json:"count"`
	LastTimestamp  time.Time `json:"lastTimestamp"`
	EventTime      time.Time `json:"eventTime"`
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
}
