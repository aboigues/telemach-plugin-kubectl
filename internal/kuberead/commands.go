package kuberead

// Arguments des lectures de la feature 002 (research.md S1), sans les options de portée. Ils
// sont définis ici une seule fois : le Reader les exécute, et l'outil de relevé du corpus
// (scripts/collect-objects) les rejoue à l'identique sur kind, pour que le corpus contienne
// exactement ce que le plugin lit. allowlist_test.go vérifie cette liste.

// ConfigMapDatesPath est le modèle jsonpath qui n'écrit que les dates de modification d'une
// ConfigMap, une par ligne : aucune valeur de la ConfigMap ne sort de kubectl (S3).
const ConfigMapDatesPath = `{range .metadata.managedFields[*]}{.time}{"\n"}{end}`

// ServiceArgs lit un Service.
func ServiceArgs(name string) []string { return []string{"get", "service", name, "-o", "json"} }

// ServicesArgs lit les Services du namespace.
func ServicesArgs() []string { return []string{"get", "services", "-o", "json"} }

// ServiceEndpointSlicesArgs lit les EndpointSlices d'un Service.
func ServiceEndpointSlicesArgs(service string) []string {
	return []string{"get", "endpointslices", "-l", "kubernetes.io/service-name=" + service, "-o", "json"}
}

// EndpointSlicesArgs lit les EndpointSlices du namespace.
func EndpointSlicesArgs() []string { return []string{"get", "endpointslices", "-o", "json"} }

// DeploymentArgs lit un Deployment.
func DeploymentArgs(name string) []string {
	return []string{"get", "deployment", name, "-o", "json"}
}

// DeploymentsArgs lit les Deployments du namespace.
func DeploymentsArgs() []string { return []string{"get", "deployments", "-o", "json"} }

// ReplicaSetsArgs lit les ReplicaSets qui portent les labels donnés (selector d'un Deployment).
func ReplicaSetsArgs(selector string) []string {
	return []string{"get", "replicasets", "-l", selector, "-o", "json"}
}

// IngressArgs lit un Ingress.
func IngressArgs(name string) []string { return []string{"get", "ingress", name, "-o", "json"} }

// IngressesArgs lit les Ingress du namespace.
func IngressesArgs() []string { return []string{"get", "ingresses", "-o", "json"} }

// IngressClassesArgs lit les classes d'Ingress (objets du cluster).
func IngressClassesArgs() []string { return []string{"get", "ingressclasses", "-o", "json"} }

// PVCArgs lit un PersistentVolumeClaim.
func PVCArgs(name string) []string { return []string{"get", "pvc", name, "-o", "json"} }

// PVCsArgs lit les PersistentVolumeClaims du namespace.
func PVCsArgs() []string { return []string{"get", "pvc", "-o", "json"} }

// StorageClassesArgs lit les classes de stockage (objets du cluster).
func StorageClassesArgs() []string { return []string{"get", "storageclasses", "-o", "json"} }

// ConfigMapDatesArgs lit les seules dates de modification d'une ConfigMap.
func ConfigMapDatesArgs(name string) []string {
	return []string{"get", "configmap", name, "--show-managed-fields", "-o", "jsonpath=" + ConfigMapDatesPath}
}

// PodsArgs, PodsBySelectorArgs et PodEventsArgs sont les lectures de pods de la feature 001,
// reprises par le relevé du corpus.
func PodsArgs() []string { return []string{"get", "pods", "-o", "json"} }

// PodsBySelectorArgs lit les pods qui portent les labels donnés.
func PodsBySelectorArgs(selector string) []string {
	return []string{"get", "pods", "-l", selector, "-o", "json"}
}

// PodEventsArgs lit les événements d'un pod.
func PodEventsArgs(pod string) []string {
	return []string{"get", "events", "--field-selector", "involvedObject.kind=Pod,involvedObject.name=" + pod, "-o", "json"}
}

// Lectures de la feature 004 (research.md F1), sans les options de portée.

// JobArgs lit un Job.
func JobArgs(name string) []string { return []string{"get", "job", name, "-o", "json"} }

// JobsArgs lit les Jobs du namespace (Jobs d'un CronJob, Jobs en échec).
func JobsArgs() []string { return []string{"get", "jobs", "-o", "json"} }

// CronJobArgs lit un CronJob.
func CronJobArgs(name string) []string { return []string{"get", "cronjob", name, "-o", "json"} }

// DaemonSetArgs lit un DaemonSet.
func DaemonSetArgs(name string) []string { return []string{"get", "daemonset", name, "-o", "json"} }

// StatefulSetArgs lit un StatefulSet.
func StatefulSetArgs(name string) []string {
	return []string{"get", "statefulset", name, "-o", "json"}
}

// HPAArgs lit un HorizontalPodAutoscaler.
func HPAArgs(name string) []string { return []string{"get", "hpa", name, "-o", "json"} }

// NodesArgs lit les nœuds (objets du cluster) : taints et état de planification.
func NodesArgs() []string { return []string{"get", "nodes", "-o", "json"} }

// MetricsAPIServiceName est l'APIService de l'API de métriques (metrics-server).
const MetricsAPIServiceName = "v1beta1.metrics.k8s.io"

// MetricsAPIServiceArgs lit l'APIService de l'API de métriques (objet du cluster).
func MetricsAPIServiceArgs() []string {
	return []string{"get", "apiservice", MetricsAPIServiceName, "-o", "json"}
}

// ReplicaSetEventsArgs lit les événements d'un ReplicaSet (refus FailedCreate).
func ReplicaSetEventsArgs(rs string) []string {
	return []string{"get", "events", "--field-selector", "involvedObject.kind=ReplicaSet,involvedObject.name=" + rs, "-o", "json"}
}
