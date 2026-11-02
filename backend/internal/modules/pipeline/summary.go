package pipeline

import (
	"context"

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
}

// Summary returns the dashboard: open jobs with their pipelines, applications
// stuck longer than stuckDays in one stage, and recent results.
func (s *Service) Summary(ctx context.Context, p authz.Principal, stuckDays int) (Summary, error) {
	var out Summary
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		open, err := s.jobs.OpenJobs(ctx, tx, dashboardJobs)
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
		recent, err := q.RecentResults(ctx)
		out.PlacedLast30Days, out.MovedLast7Days = int(recent.PlacedLast30Days), int(recent.MovedLast7Days)
		return err
	})
	return out, err
}
