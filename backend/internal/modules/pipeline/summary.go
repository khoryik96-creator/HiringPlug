package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// dashboardJobs is how many open jobs the dashboard shows (and counts over).
const dashboardJobs = 50

// StageCounts is how many applications are in each open stage.
type StageCounts map[string]int

// DashboardJob is an open job with its pipeline.
type DashboardJob struct {
	ID, Title, CompanyID, CompanyName string
	Counts                            StageCounts
}

// Summary is the dashboard.
type Summary struct {
	OpenJobs         int
	Counts           StageCounts
	Jobs             []DashboardJob
	Stuck            []View
	PlacedLast30Days int
	MovedLast7Days   int
	MovedPrev7Days   int
	AddedLast7Days   int
	OpenedLast7Days  int
}

// Summary returns the dashboard: open jobs with their pipelines, applications
// stuck longer than stuckDays in one stage, and recent results. With owners,
// all of it is about the jobs those people own.
func (s *Service) Summary(ctx context.Context, p authz.Principal, stuckDays int, owners []string) (Summary, error) {
	var out Summary
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		open, err := s.jobs.OpenJobs(ctx, tx, dashboardJobs, owners)
		if err != nil {
			return err
		}
		ids, companyIDs := make([]string, len(open)), make([]string, len(open))
		for i, j := range open {
			ids[i], companyIDs[i] = j.ID, j.CompanyID
		}
		q := store.New(tx)
		counts, err := q.StageCounts(ctx, ids)
		if err != nil {
			return err
		}
		perJob := map[string]StageCounts{}
		out.Counts = StageCounts{}
		for _, c := range counts {
			if IsClosed(c.Stage) {
				continue
			}
			if perJob[c.JobOrderID] == nil {
				perJob[c.JobOrderID] = StageCounts{}
			}
			perJob[c.JobOrderID][c.Stage] += int(c.N)
			out.Counts[c.Stage] += int(c.N)
		}
		names, err := s.companies.Names(ctx, tx, companyIDs)
		if err != nil {
			return err
		}
		out.OpenJobs = len(open)
		out.Jobs = make([]DashboardJob, len(open))
		for i, j := range open {
			out.Jobs[i] = DashboardJob{ID: j.ID, Title: j.Title, CompanyID: j.CompanyID, CompanyName: names[j.CompanyID], Counts: perJob[j.ID]}
		}
		stuck, err := q.StuckApplications(ctx, store.StuckApplicationsParams{JobIds: ids, StuckDays: int32(stuckDays)}) //nolint:gosec // 1..365 per the contract
		if err != nil {
			return err
		}
		if out.Stuck, err = s.views(ctx, tx, stuck); err != nil {
			return err
		}
		scoped, owned := len(owners) > 0, []string{}
		if scoped {
			if owned, err = s.jobs.OwnedJobs(ctx, tx, owners); err != nil {
				return err
			}
		}
		recent, err := q.RecentResults(ctx, store.RecentResultsParams{Scoped: scoped, JobIds: owned})
		if err != nil {
			return err
		}
		week, err := q.WeekTrends(ctx, store.WeekTrendsParams{Scoped: scoped, JobIds: owned})
		if err != nil {
			return err
		}
		out.PlacedLast30Days, out.MovedLast7Days = int(recent.PlacedLast30Days), int(week.MovedThisWeek)
		out.MovedPrev7Days, out.AddedLast7Days = int(week.MovedLastWeek), int(week.AddedThisWeek)
		for _, j := range open {
			if time.Since(j.CreatedAt) < 7*24*time.Hour {
				out.OpenedLast7Days++
			}
		}
		return nil
	})
	return out, err
}

// JobProgress is a job's candidates per open stage, when anything last
// moved on it, and how many wait over 7 days in one open stage.
type JobProgress struct {
	JobID     string
	Counts    StageCounts
	LastMoved *time.Time
	Waiting   int
	WaitingIn StageCounts // of them, per stage
}

// Progress returns the progress of the given jobs (the jobs list).
func (s *Service) Progress(ctx context.Context, p authz.Principal, jobIDs []string) ([]JobProgress, error) {
	var out []JobProgress
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		counts, err := q.StageCounts(ctx, jobIDs)
		if err != nil {
			return err
		}
		activity, err := q.JobActivity(ctx, jobIDs)
		if err != nil {
			return err
		}
		byJob := make(map[string]*JobProgress, len(jobIDs))
		out = make([]JobProgress, len(jobIDs))
		for i, id := range jobIDs {
			out[i] = JobProgress{JobID: id, Counts: StageCounts{}, WaitingIn: StageCounts{}}
			byJob[id] = &out[i]
		}
		for _, c := range counts {
			if j := byJob[c.JobOrderID]; j != nil && !IsClosed(c.Stage) {
				j.Counts[c.Stage] += int(c.N)
			}
		}
		for _, a := range activity {
			if j := byJob[a.JobOrderID]; j != nil {
				if last := a.LastMoved; j.LastMoved == nil || last.After(*j.LastMoved) {
					j.LastMoved = &last
				}
				j.Waiting += int(a.Waiting)
				if a.Waiting > 0 {
					j.WaitingIn[a.Stage] += int(a.Waiting)
				}
			}
		}
		return nil
	})
	return out, err
}
