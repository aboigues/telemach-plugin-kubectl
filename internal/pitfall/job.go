package pitfall

import (
	"strconv"

	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un Job (feature 004, research.md F8 ; catalogue : internal/catalog/data/batch.yaml).
const (
	JobBackoffLimit = "job.backoff-limit"
	JobDeadline     = "job.deadline"
	JobFailed       = "job.failed"
	JobSuspended    = "job.suspended"
	JobRunning      = "job.running"
	JobPodsGone     = "job.pods-gone"
	JobComplete     = "job.complete"
)

// jobNameLabels désignent les pods d'un Job ; job-name est l'étiquette des versions anciennes.
var jobNameLabels = []string{"batch.kubernetes.io/job-name", "job-name"}

func init() {
	knownIDs = append(knownIDs, JobBackoffLimit, JobDeadline, JobFailed, JobSuspended, JobRunning, JobPodsGone, JobComplete)
	diagnosers[KindJob] = DiagnoseJob
}

// DiagnoseJob lit l'état d'un Job (condition, tentatives), puis diagnostique le plus récent de
// ses pods en difficulté avec les causes de la feature 001.
func DiagnoseJob(ctx Context, name string) (ObjectSnapshot, error) {
	job, err := ctx.Reader.GetJob(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindJob, name)
	s.Captures["job"] = name
	jobLinks(ctx, &s, job)
	return s, nil
}

// jobLinks remplit les maillons status et pods d'un Job (partagé avec le diagnostic d'un
// CronJob, qui enchaîne sur son dernier Job).
func jobLinks(ctx Context, s *ObjectSnapshot, job kuberead.Job) {
	limit := job.Spec.BackoffLimitOrDefault()
	caps := map[string]string{
		"job": job.Metadata.Name, "failed": strconv.Itoa(job.Status.Failed), "succeeded": strconv.Itoa(job.Status.Succeeded),
		"backoff_limit": strconv.Itoa(limit),
	}
	if d := job.Spec.ActiveDeadlineSeconds; d != nil {
		caps["deadline"] = strconv.Itoa(*d)
	}
	if t := job.Spec.TTLSecondsAfterFinished; t != nil {
		caps["ttl"] = strconv.Itoa(*t)
	}
	completions := 1
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}
	caps["completions"] = strconv.Itoa(completions)
	s.Facts = append(s.Facts, Observation{Value: strconv.Itoa(job.Status.Succeeded) + "/" + strconv.Itoa(completions) + " réussite(s)"},
		Observation{Label: "échecs", Value: strconv.Itoa(job.Status.Failed) + " (limite " + strconv.Itoa(limit) + ")"})

	failed := kuberead.FindCondition(job.Status.Conditions, "Failed")
	complete := kuberead.FindCondition(job.Status.Conditions, "Complete")
	finished := (failed != nil && failed.Status == "True") || (complete != nil && complete.Status == "True")
	s.AddLink(StepStatus, StateOK, Observation{Label: "réussis", Value: caps["succeeded"]}, Observation{Label: "échoués", Value: caps["failed"]})

	var pods []kuberead.Pod
	var podsErr error
	for _, l := range jobNameLabels {
		var list kuberead.PodList
		if list, podsErr = ctx.Reader.PodsBySelector(l + "=" + job.Metadata.Name); podsErr == nil && len(list.Items) > 0 {
			pods = list.Items
			break
		}
	}
	s.Deny(podsErr)
	s.AddLink(StepPods, LinkStateFor(podsErr), Observation{Value: plural(len(pods), "pod")})
	if podsErr == nil {
		s.Failing = firstFailing(ctx, pods)
	}
	if f := s.Failing; f != nil {
		caps["pod"], caps["reason"] = f.Pod.Name, f.Pod.Reason
	}

	switch {
	case failed != nil && failed.Status == "True" && failed.Reason == "BackoffLimitExceeded":
		s.Add(Pitfall{ID: JobBackoffLimit, Step: StepStatus, Severity: Warning, Captures: caps})
	case failed != nil && failed.Status == "True" && failed.Reason == "DeadlineExceeded":
		s.Add(Pitfall{ID: JobDeadline, Step: StepStatus, Severity: Warning, Captures: caps})
	case failed != nil && failed.Status == "True":
		caps["condition"] = clean(failed.Reason + " : " + failed.Message)
		s.Add(Pitfall{ID: JobFailed, Step: StepStatus, Severity: Warning, Captures: caps})
	case job.Spec.Suspend != nil && *job.Spec.Suspend:
		s.Add(Pitfall{ID: JobSuspended, Step: StepStatus, Severity: Info, Captures: caps})
	case !finished:
		s.Add(Pitfall{ID: JobRunning, Step: StepStatus, Severity: Info, Transient: true, Captures: caps})
	}
	if finished && podsErr == nil && len(pods) == 0 && failed == nil {
		// Terminé sans pod : journal perdu (pods supprimés à la main).
		s.Add(Pitfall{ID: JobPodsGone, Step: StepPods, Severity: Info, Captures: caps})
	} else if complete != nil && complete.Status == "True" {
		s.Add(Pitfall{ID: JobComplete, Step: StepStatus, Severity: OK, Captures: caps})
	}
}
