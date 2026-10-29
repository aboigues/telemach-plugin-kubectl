package pitfall

import (
	"strconv"
	"time"

	"github.com/aboigues/telemach-plugin-kubectl/internal/cron"
	"github.com/aboigues/telemach-plugin-kubectl/internal/kuberead"
)

// Pièges d'un CronJob (feature 004, research.md F8 ; catalogue : batch.yaml).
const (
	CronJobSuspended     = "cronjob.suspended"
	CronJobNeverRun      = "cronjob.never-run"
	CronJobForbidSkipped = "cronjob.forbid-skipped"
	CronJobLastFailed    = "cronjob.last-failed"
	CronJobOK            = "cronjob.ok"
)

func init() {
	knownIDs = append(knownIDs, CronJobSuspended, CronJobNeverRun, CronJobForbidSkipped, CronJobLastFailed, CronJobOK)
	diagnosers[KindCronJob] = DiagnoseCronJob
}

// dateLayout écrit une échéance pour l'apprenant.
const dateLayout = "02/01/2006 15:04 MST"

// DiagnoseCronJob vérifie la planification (suspension, prochaine échéance), l'historique des
// Jobs (exécutions sautées par concurrencyPolicy: Forbid) et le dernier Job, diagnostiqué comme
// un Job.
func DiagnoseCronJob(ctx Context, name string) (ObjectSnapshot, error) {
	cj, err := ctx.Reader.GetCronJob(name)
	if err != nil {
		return ObjectSnapshot{}, err
	}
	s := newSnapshot(ctx, KindCronJob, name)
	s.Captures["cronjob"] = name
	policy := cj.Spec.ConcurrencyPolicyOrDefault()
	loc, tz := time.UTC, "UTC (fuseau du contrôleur, par défaut)"
	if cj.Spec.TimeZone != nil && *cj.Spec.TimeZone != "" {
		if l, err := time.LoadLocation(*cj.Spec.TimeZone); err == nil {
			loc, tz = l, *cj.Spec.TimeZone
		}
	}
	caps := map[string]string{"schedule": cj.Spec.Schedule, "policy": policy, "timezone": tz}
	s.Facts = append(s.Facts, Observation{Label: "planification", Value: cj.Spec.Schedule}, Observation{Label: "fuseau", Value: tz},
		Observation{Label: "concurrencyPolicy", Value: policy})

	// Planification.
	sched, perr := cron.Parse(cj.Spec.Schedule)
	var next time.Time
	if perr == nil {
		next = sched.Next(ctx.Now, loc)
	}
	if !next.IsZero() {
		caps["next"] = next.In(loc).Format(dateLayout)
		s.AddLink(StepSchedule, StateOK, Observation{Label: "prochaine exécution", Value: caps["next"]})
	} else {
		s.AddLink(StepSchedule, StateOK)
	}
	suspended := cj.Spec.Suspend != nil && *cj.Spec.Suspend
	if suspended {
		s.Add(Pitfall{ID: CronJobSuspended, Step: StepSchedule, Severity: Info, Captures: caps})
	}

	// Historique : Jobs créés par ce CronJob.
	jobs, jerr := ctx.Reader.ListJobs()
	s.Deny(jerr)
	var owned []kuberead.Job
	for _, j := range jobs.Items {
		for _, o := range j.Metadata.OwnerReferences {
			if o.Kind == "CronJob" && o.Name == name {
				owned = append(owned, j)
			}
		}
	}
	s.AddLink(StepHistory, LinkStateFor(jerr), Observation{Value: plural(len(owned), "Job")})
	last := cj.Status.LastScheduleTime
	if last != nil {
		caps["last"] = last.In(loc).Format(dateLayout)
	}
	if jerr == nil && len(owned) == 0 && last == nil && !suspended {
		first := time.Time{}
		if sched != nil {
			first = sched.Next(cj.Metadata.CreationTimestamp, loc)
		}
		sev := Info
		if !first.IsZero() && first.Before(ctx.Now) {
			// Une échéance est passée sans exécution : startingDeadlineSeconds, contrôleur…
			sev, caps["due"] = Warning, first.In(loc).Format(dateLayout)
		}
		s.Add(Pitfall{ID: CronJobNeverRun, Step: StepHistory, Severity: sev, Captures: caps})
	}
	if policy == "Forbid" && len(cj.Status.Active) > 0 && last != nil && sched != nil {
		if n := sched.Next(*last, loc); !n.IsZero() && n.Before(ctx.Now) {
			caps["active"] = cj.Status.Active[0].Name
			s.Add(Pitfall{ID: CronJobForbidSkipped, Step: StepHistory, Severity: Warning, Captures: caps})
		}
	}

	// Dernier Job.
	latest := latestJob(owned)
	if latest == nil {
		s.AddLink(StepLastJob, StateSkipped)
	} else {
		cronLastJob(ctx, &s, *latest, caps)
	}
	if len(s.Pitfalls) == 0 {
		s.Add(Pitfall{ID: CronJobOK, Step: StepSchedule, Severity: OK, Captures: caps})
	}
	return s, nil
}

// cronLastJob décrit le dernier Job et diagnostique son pod en difficulté.
func cronLastJob(ctx Context, s *ObjectSnapshot, job kuberead.Job, caps map[string]string) {
	caps["job"] = job.Metadata.Name
	failed := kuberead.FindCondition(job.Status.Conditions, "Failed")
	state := StateOK
	if failed != nil && failed.Status == "True" {
		state = StateBroken
	}
	s.AddLink(StepLastJob, StateOK, Observation{Value: job.Metadata.Name}, Observation{Label: "échecs", Value: strconv.Itoa(job.Status.Failed)})
	if state != StateBroken {
		return
	}
	pods, err := ctx.Reader.ListPods()
	s.Deny(err)
	if err == nil {
		var mine []kuberead.Pod
		for _, p := range pods.Items {
			for _, l := range jobNameLabels {
				if p.Metadata.Labels[l] == job.Metadata.Name {
					mine = append(mine, p)
					break
				}
			}
		}
		s.Failing = firstFailing(ctx, mine)
	}
	if f := s.Failing; f != nil {
		caps["pod"], caps["reason"] = f.Pod.Name, f.Pod.Reason
	}
	caps["reason_job"] = failed.Reason
	s.Add(Pitfall{ID: CronJobLastFailed, Step: StepLastJob, Severity: Warning, Captures: caps})
}

// latestJob renvoie le Job créé le plus récemment, ou nil.
func latestJob(jobs []kuberead.Job) *kuberead.Job {
	var out *kuberead.Job
	for i := range jobs {
		if out == nil || jobs[i].Metadata.CreationTimestamp.After(out.Metadata.CreationTimestamp) {
			out = &jobs[i]
		}
	}
	return out
}
