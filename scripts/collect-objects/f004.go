//go:build integration

package main

import (
	"strings"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
	"github.com/aboigues/telemach-plugin-kubectl/tests/integration/kindcluster"
)

// Cas de la feature 004 (research.md F12, data-model.md § Pitfall et § PlacementCause), chacun
// dans son namespace, sur le cluster à trois nœuds. {W1} et {W2} désignent les workers ; toute
// modification d'un nœud (taint, cordon) est annulée dans « after ». Les cas qui exigent
// metrics-server viennent après son installation (étape de « f004-metrics-server »).

func cluster(a ...string) step { return step{args: a, cluster: true} }

func waitFor(cond, obj string, timeout time.Duration) step {
	return step{args: []string{"wait", "--for=" + cond, obj, "--timeout=" + timeout.String()}, soft: true}
}

func jobReads(name string) []read {
	return []read{
		r("job.json", kuberead.JobArgs(name)),
		r("selected-pods.json", kuberead.PodsBySelectorArgs("batch.kubernetes.io/job-name="+name)),
		r("jobs.json", kuberead.JobsArgs()),
	}
}

func cronJobReads(name string) []read {
	return []read{
		r("cronjob.json", kuberead.CronJobArgs(name)),
		r("jobs.json", kuberead.JobsArgs()),
		r("pods.json", kuberead.PodsArgs()),
	}
}

func statefulSetReads(name string) []read {
	return []read{
		r("statefulset.json", kuberead.StatefulSetArgs(name)),
		r("service.json", kuberead.ServiceArgs(name)),
		r("selected-pods.json", kuberead.PodsBySelectorArgs("app="+name)),
		r("pvcs.json", kuberead.PVCsArgs()),
		r("storageclasses.json", kuberead.StorageClassesArgs()),
	}
}

func hpaReads(name, target string) []read {
	return []read{
		r("hpa.json", kuberead.HPAArgs(name)),
		r("apiservice.json", kuberead.MetricsAPIServiceArgs()),
		r("deployment.json", kuberead.DeploymentArgs(target)),
		r("selected-pods.json", kuberead.PodsBySelectorArgs("app="+target)),
	}
}

// pendingPodEvents lit les événements du pod en attente du Deployment anti.
func pendingPodEvents(c *kindcluster.Cluster, ns string) []string {
	res := c.Kubectl("get", "pods", "-l", "app=anti", "--field-selector", "status.phase=Pending",
		"-o", "jsonpath={.items[0].metadata.name}", "-n", ns)
	return kuberead.PodEventsArgs(strings.TrimSpace(res.Stdout))
}

// replicaSetEvents lit les événements du ReplicaSet courant du Deployment api.
func replicaSetEvents(c *kindcluster.Cluster, ns string) []string {
	res := c.Kubectl("get", "replicasets", "-l", "app=api", "-o", "jsonpath={.items[-1:].metadata.name}", "-n", ns)
	return kuberead.ReplicaSetEventsArgs(strings.TrimSpace(res.Stdout))
}

var f004Cases = []objectCase{
	// Pods Pending (US2).
	{
		id: "pending-taint-cordon", ns: "f004-placement",
		steps: []step{
			cluster("taint", "node", "{W1}", "dedie=gpu:NoSchedule"),
			cluster("cordon", "{W2}"),
			apply("pending-placement.yaml"), pause(10 * time.Second),
		},
		reads: []read{
			r("pods.json", kuberead.PodsArgs()),
			r("events-place.json", kuberead.PodEventsArgs("place")),
			r("nodes.json", kuberead.NodesArgs()),
		},
		after: []step{cluster("uncordon", "{W2}"), cluster("taint", "node", "{W1}", "dedie-")},
	},
	{
		id: "pending-anti-affinity", ns: "f004-anti",
		steps: []step{apply("anti-affinity.yaml"), pause(20 * time.Second)},
		reads: []read{
			r("pods.json", kuberead.PodsArgs()),
			r("nodes.json", kuberead.NodesArgs()),
			{file: "events-pending.json", dyn: pendingPodEvents},
		},
	},
	// Refus d'admission vu depuis le Deployment (US1).
	{
		id: "replica-failure", ns: "f004-quota",
		steps: []step{
			apply("quota.yaml"), apply("quota-deployment.yaml"), waitAvailable("api"),
			run("scale", "deployment", "api", "--replicas=3"), pause(5 * time.Second),
		},
		reads: append(deploymentReads("api", "app=api"), read{file: "events-replicaset.json", dyn: replicaSetEvents}),
	},
	// Jobs (US3).
	{
		id: "job-backoff-limit", ns: "f004-job-echec",
		steps: []step{apply("job-fails.yaml"), waitFor("condition=Failed", "job/calc", 120*time.Second)},
		reads: jobReads("calc"),
	},
	{
		id: "job-deadline", ns: "f004-job-delai",
		steps: []step{apply("job-deadline.yaml"), waitFor("condition=Failed", "job/lent", 60*time.Second)},
		reads: jobReads("lent"),
	},
	{
		id: "job-complete", ns: "f004-job-ok",
		steps: []step{
			run("create", "job", "ok", "--image=busybox:1.38", "--", "sh", "-c", "echo ok"),
			waitFor("condition=Complete", "job/ok", 120*time.Second),
		},
		reads: jobReads("ok"),
	},
	{
		// Même Job, pods supprimés à la main : un Job terminé sans pods (journal perdu).
		id: "job-pods-gone", ns: "f004-job-ok",
		steps: []step{run("delete", "pods", "-l", "batch.kubernetes.io/job-name=ok"), pause(2 * time.Second)},
		reads: jobReads("ok"),
	},
	// CronJobs (US3).
	{
		id: "cronjob-suspended", ns: "f004-cron-suspendu",
		steps: []step{apply("cronjob-suspended.yaml"), pause(2 * time.Second)},
		reads: cronJobReads("sauvegarde"),
	},
	{
		id: "cronjob-never-run", ns: "f004-cron-annuel",
		steps: []step{
			run("create", "cronjob", "annuel", "--image=busybox:1.38", "--schedule=0 0 1 1 *", "--", "sh", "-c", "echo bonne année"),
			pause(2 * time.Second),
		},
		reads: cronJobReads("annuel"),
	},
	{
		id: "cronjob-forbid-skipped", ns: "f004-cron-forbid",
		steps: []step{apply("cronjob-forbid.yaml"), pause(150 * time.Second)},
		reads: cronJobReads("export"),
	},
	{
		id: "cronjob-last-failed", ns: "f004-cron-echec",
		steps: []step{apply("cronjob-fails.yaml"), pause(90 * time.Second)},
		reads: cronJobReads("echec"),
	},
	// DaemonSets (US3).
	{
		id: "daemonset-control-plane", ns: "f004-ds",
		steps: []step{apply("daemonset.yaml"), {args: []string{"rollout", "status", "daemonset/agent", "--timeout=120s"}, soft: true}},
		reads: []read{
			r("daemonset.json", kuberead.DaemonSetArgs("agent")),
			r("nodes.json", kuberead.NodesArgs()),
			r("selected-pods.json", kuberead.PodsBySelectorArgs("app=agent")),
		},
	},
	{
		id: "daemonset-tainted-worker", ns: "f004-ds-taint",
		steps: []step{
			cluster("taint", "node", "{W1}", "dedie=gpu:NoSchedule"),
			apply("daemonset.yaml"), pause(20 * time.Second),
		},
		reads: []read{
			r("daemonset.json", kuberead.DaemonSetArgs("agent")),
			r("nodes.json", kuberead.NodesArgs()),
			r("selected-pods.json", kuberead.PodsBySelectorArgs("app=agent")),
		},
		after: []step{cluster("taint", "node", "{W1}", "dedie-")},
	},
	// StatefulSets (US3).
	{
		id: "statefulset-no-service", ns: "f004-sts-service",
		steps: []step{apply("statefulset-no-service.yaml"), pause(30 * time.Second)},
		reads: statefulSetReads("base"),
	},
	{
		id: "statefulset-ordered-wait", ns: "f004-sts-ordre",
		steps: []step{apply("statefulset-ordered.yaml"), pause(30 * time.Second)},
		reads: statefulSetReads("ordre"),
	},
	{
		id: "statefulset-pvc-pending", ns: "f004-sts-pvc",
		steps: []step{apply("statefulset-pvc.yaml"), pause(20 * time.Second)},
		reads: statefulSetReads("donnees"),
	},
	{
		id: "statefulset-healthy", ns: "f004-sts-sain",
		steps: []step{
			apply("statefulset-no-service.yaml"),
			run("create", "service", "clusterip", "base", "--clusterip=None", "--tcp=8080:8080"),
			{args: []string{"rollout", "status", "statefulset/base", "--timeout=180s"}, soft: true},
		},
		reads: statefulSetReads("base"),
	},
	// HPA sans metrics-server (US4).
	{
		id: "hpa-metrics-unavailable", ns: "f004-hpa-sans-metriques",
		steps: []step{apply("hpa-no-request.yaml"), waitAvailable("web"), pause(30 * time.Second)},
		reads: hpaReads("web", "web"),
	},
	{
		id: "hpa-target-missing", ns: "f004-hpa-orphelin",
		steps: []step{apply("hpa-target-missing.yaml"), pause(30 * time.Second)},
		reads: hpaReads("orphelin", "absent"),
	},
	// Installation de metrics-server dans ce cluster jetable, puis HPA avec métriques.
	{
		id: "metrics-server-installed", ns: "f004-metriques",
		steps: []step{
			cluster("apply", "-f", "testdata/kind/metrics-server.yaml"),
			cluster("wait", "--for=condition=Available", "deployment/metrics-server", "-n", "kube-system", "--timeout=240s"),
			cluster("wait", "--for=condition=Available", "apiservice/"+kuberead.MetricsAPIServiceName, "--timeout=240s"),
		},
		reads: []read{r("apiservice.json", kuberead.MetricsAPIServiceArgs())},
	},
	{
		id: "hpa-missing-request", ns: "f004-hpa-sans-request",
		steps: []step{apply("hpa-no-request.yaml"), waitAvailable("web"), pause(90 * time.Second)},
		reads: hpaReads("web", "web"),
	},
	{
		id: "hpa-limited", ns: "f004-hpa-plafond",
		steps: []step{apply("hpa-limited.yaml"), waitAvailable("web"), pause(120 * time.Second)},
		reads: hpaReads("web", "web"),
	},
	{
		id: "hpa-idle", ns: "f004-hpa-repos",
		steps: []step{apply("hpa-idle.yaml"), waitAvailable("web"), pause(90 * time.Second)},
		reads: hpaReads("web", "web"),
	},
}
