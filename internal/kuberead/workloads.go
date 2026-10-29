package kuberead

import "time"

// Types de la feature 004 (data-model.md § Types lus, research.md F2) : seuls les champs
// listés sont décodés ; jamais de valeur d'env (Container ne la décode pas), ni d'annotation,
// ni d'adresse de nœud.

// Toleration autorise un pod sur un nœud porteur d'un taint.
type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Effect   string `json:"effect"`
}

// Condition est une condition d'objet (Job, HPA, APIService, nœud).
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// FindCondition renvoie la condition de type t, ou nil.
func FindCondition(conds []Condition, t string) *Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

// LabelSelector est réduit à matchLabels.
type LabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels"`
}

// JobList est la réponse de « get jobs -o json ».
type JobList struct {
	Items []Job `json:"items"`
}

// Job est un Job réduit.
type Job struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     JobSpec    `json:"spec"`
	Status   JobStatus  `json:"status"`
}

// JobSpec est la partie utile de spec.
type JobSpec struct {
	Completions             *int        `json:"completions"`
	Parallelism             *int        `json:"parallelism"`
	BackoffLimit            *int        `json:"backoffLimit"`
	ActiveDeadlineSeconds   *int        `json:"activeDeadlineSeconds"`
	TTLSecondsAfterFinished *int        `json:"ttlSecondsAfterFinished"`
	Suspend                 *bool       `json:"suspend"`
	Template                PodTemplate `json:"template"`
}

// BackoffLimitOrDefault renvoie backoffLimit, 6 par défaut (valeur de Kubernetes).
func (s JobSpec) BackoffLimitOrDefault() int {
	if s.BackoffLimit == nil {
		return 6
	}
	return *s.BackoffLimit
}

// JobStatus est l'état observé d'un Job.
type JobStatus struct {
	Active         int         `json:"active"`
	Succeeded      int         `json:"succeeded"`
	Failed         int         `json:"failed"`
	StartTime      *time.Time  `json:"startTime"`
	CompletionTime *time.Time  `json:"completionTime"`
	Conditions     []Condition `json:"conditions"`
}

// CronJob est un CronJob réduit.
type CronJob struct {
	Metadata ObjectMeta    `json:"metadata"`
	Spec     CronJobSpec   `json:"spec"`
	Status   CronJobStatus `json:"status"`
}

// CronJobSpec est la partie utile de spec.
type CronJobSpec struct {
	Schedule                   string  `json:"schedule"`
	TimeZone                   *string `json:"timeZone"`
	Suspend                    *bool   `json:"suspend"`
	ConcurrencyPolicy          string  `json:"concurrencyPolicy"`
	StartingDeadlineSeconds    *int    `json:"startingDeadlineSeconds"`
	SuccessfulJobsHistoryLimit *int    `json:"successfulJobsHistoryLimit"`
	FailedJobsHistoryLimit     *int    `json:"failedJobsHistoryLimit"`
	JobTemplate                struct {
		Spec struct {
			Template PodTemplate `json:"template"`
		} `json:"spec"`
	} `json:"jobTemplate"`
}

// ConcurrencyPolicyOrDefault renvoie concurrencyPolicy, Allow par défaut.
func (s CronJobSpec) ConcurrencyPolicyOrDefault() string {
	if s.ConcurrencyPolicy == "" {
		return "Allow"
	}
	return s.ConcurrencyPolicy
}

// CronJobStatus est l'état observé d'un CronJob.
type CronJobStatus struct {
	Active []struct {
		Name string `json:"name"`
	} `json:"active"`
	LastScheduleTime   *time.Time `json:"lastScheduleTime"`
	LastSuccessfulTime *time.Time `json:"lastSuccessfulTime"`
}

// DaemonSet est un DaemonSet réduit.
type DaemonSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Selector       LabelSelector `json:"selector"`
		Template       PodTemplate   `json:"template"`
		UpdateStrategy struct {
			Type string `json:"type"`
		} `json:"updateStrategy"`
	} `json:"spec"`
	Status struct {
		DesiredNumberScheduled int `json:"desiredNumberScheduled"`
		CurrentNumberScheduled int `json:"currentNumberScheduled"`
		NumberReady            int `json:"numberReady"`
		UpdatedNumberScheduled int `json:"updatedNumberScheduled"`
		NumberMisscheduled     int `json:"numberMisscheduled"`
	} `json:"status"`
}

// StatefulSet est un StatefulSet réduit.
type StatefulSet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Replicas             *int          `json:"replicas"`
		ServiceName          string        `json:"serviceName"`
		PodManagementPolicy  string        `json:"podManagementPolicy"`
		Selector             LabelSelector `json:"selector"`
		Template             PodTemplate   `json:"template"`
		VolumeClaimTemplates []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				StorageClassName *string `json:"storageClassName"`
			} `json:"spec"`
		} `json:"volumeClaimTemplates"`
	} `json:"spec"`
	Status struct {
		Replicas        int `json:"replicas"`
		ReadyReplicas   int `json:"readyReplicas"`
		CurrentReplicas int `json:"currentReplicas"`
	} `json:"status"`
}

// ReplicasOrDefault renvoie spec.replicas, 1 par défaut.
func (s StatefulSet) ReplicasOrDefault() int {
	if s.Spec.Replicas == nil {
		return 1
	}
	return *s.Spec.Replicas
}

// PodManagementPolicyOrDefault renvoie podManagementPolicy, OrderedReady par défaut.
func (s StatefulSet) PodManagementPolicyOrDefault() string {
	if s.Spec.PodManagementPolicy == "" {
		return "OrderedReady"
	}
	return s.Spec.PodManagementPolicy
}

// HPA est un HorizontalPodAutoscaler (autoscaling/v2) réduit.
type HPA struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		ScaleTargetRef struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Name       string `json:"name"`
		} `json:"scaleTargetRef"`
		MinReplicas *int        `json:"minReplicas"`
		MaxReplicas int         `json:"maxReplicas"`
		Metrics     []HPAMetric `json:"metrics"`
	} `json:"spec"`
	Status struct {
		CurrentReplicas int         `json:"currentReplicas"`
		DesiredReplicas int         `json:"desiredReplicas"`
		CurrentMetrics  []HPAMetric `json:"currentMetrics"`
		Conditions      []Condition `json:"conditions"`
	} `json:"status"`
}

// MinReplicasOrDefault renvoie minReplicas, 1 par défaut.
func (h HPA) MinReplicasOrDefault() int {
	if h.Spec.MinReplicas == nil {
		return 1
	}
	return *h.Spec.MinReplicas
}

// HPAMetric est une métrique d'un HPA ; seules les métriques de type Resource sont détaillées.
type HPAMetric struct {
	Type     string `json:"type"`
	Resource *struct {
		Name   string `json:"name"`
		Target struct {
			Type               string `json:"type"`
			AverageUtilization *int   `json:"averageUtilization"`
			AverageValue       string `json:"averageValue"`
		} `json:"target"`
		Current struct {
			AverageUtilization *int   `json:"averageUtilization"`
			AverageValue       string `json:"averageValue"`
		} `json:"current"`
	} `json:"resource"`
}

// NodeList est la réponse de « get nodes -o json ».
type NodeList struct {
	Items []Node `json:"items"`
}

// Node est un nœud réduit : nom, labels (rôle), taints, état de planification et condition
// Ready ; jamais status.addresses (research.md F2).
type Node struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Taints        []Taint `json:"taints"`
		Unschedulable bool    `json:"unschedulable"`
	} `json:"spec"`
	Status struct {
		Conditions []Condition `json:"conditions"`
	} `json:"status"`
}

// Taint repousse les pods qui ne le tolèrent pas.
type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

// String écrit le taint comme kubectl : « clé=valeur:effet » (« clé:effet » sans valeur).
func (t Taint) String() string {
	s := t.Key
	if t.Value != "" {
		s += "=" + t.Value
	}
	return s + ":" + t.Effect
}

// ControlPlaneTaint est le taint posé sur les nœuds du plan de contrôle.
const ControlPlaneTaint = "node-role.kubernetes.io/control-plane"

// APIService est réduit à ses conditions (Available).
type APIService struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Conditions []Condition `json:"conditions"`
	} `json:"status"`
}
