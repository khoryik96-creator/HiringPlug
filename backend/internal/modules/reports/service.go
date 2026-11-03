package reports

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service works out reports.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	loc  *time.Location
}

// Report is the reports for a period.
type Report struct {
	From, To       time.Time // days
	Funnel         []FunnelStage
	Sources        []SourceRow
	Recruiters     []RecruiterRow
	RecruiterNames map[string]string
	TimeToFill     FillTimes
	JobTitles      map[string]string
	FeesHidden     bool
}

// Report works out the reports for the days from..to (the last 90 days by
// default), in the company's time zone.
func (s *Service) Report(ctx context.Context, p authz.Principal, from, to *time.Time) (Report, error) {
	now := time.Now().In(s.loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	first, last, errs := Period(from, to, today)
	if err := errs.Err(); err != nil {
		return Report{}, err
	}
	// The period as instants: from the start of the first day to the start of
	// the day after the last, in the company's time zone.
	start := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, s.loc)
	end := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, s.loc)
	out := Report{From: first, To: last, FeesHidden: !p.Can(api.PermissionPlacementsFeesRead)}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.applications(ctx, tx, start, end)
		if err != nil {
			return err
		}
		out.Funnel, out.Sources = Funnel(apps), Sources(apps)
		if out.Recruiters, out.RecruiterNames, err = s.recruiters(ctx, tx, first, last, out.FeesHidden); err != nil {
			return err
		}
		out.TimeToFill, out.JobTitles, err = s.timeToFill(ctx, tx, start, end)
		return err
	})
	return out, err
}

// applications are those made in the period, each with its source: the
// channel it came in through, else the candidate's source.
func (s *Service) applications(ctx context.Context, tx pgx.Tx, start, end time.Time) ([]App, error) {
	facts, err := s.deps.Applications.Facts(ctx, tx, start, end)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(facts))
	for i, f := range facts {
		ids[i] = f.CandidateID
	}
	sources, err := s.deps.Sources.Sources(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	apps := make([]App, len(facts))
	for i, f := range facts {
		source := "unknown"
		switch {
		case f.Channel != nil:
			source = *f.Channel
		case sources[f.CandidateID] != nil:
			source = *sources[f.CandidateID]
		}
		apps[i] = App{Source: source, Stages: f.Stages}
	}
	return apps, nil
}

func (s *Service) recruiters(ctx context.Context, tx pgx.Tx, first, last time.Time, feesHidden bool) ([]RecruiterRow, map[string]string, error) {
	placements, err := s.deps.Placements.Facts(ctx, tx, first, last)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for i := range placements {
		if feesHidden { // the database leaves them out too
			placements[i].FeeAmount, placements[i].FeeCurrency = nil, nil
		}
		if placements[i].RecruiterID != nil {
			ids = append(ids, *placements[i].RecruiterID)
		}
	}
	names, err := s.deps.Users.UserNames(ctx, tx, ids)
	return Recruiters(placements), names, err
}

func (s *Service) timeToFill(ctx context.Context, tx pgx.Tx, start, end time.Time) (FillTimes, map[string]string, error) {
	placed, err := s.deps.Applications.FirstPlaced(ctx, tx, start, end)
	if err != nil {
		return FillTimes{}, nil, err
	}
	ids := make([]string, len(placed))
	for i, p := range placed {
		ids[i] = p.JobID
	}
	jobs, err := s.deps.Jobs.Jobs(ctx, tx, ids)
	if err != nil {
		return FillTimes{}, nil, err
	}
	fills := make([]Fill, 0, len(placed))
	titles := map[string]string{}
	for _, p := range placed {
		j, ok := jobs[p.JobID]
		if !ok {
			continue
		}
		titles[p.JobID] = j.Title
		fills = append(fills, Fill{JobID: p.JobID, JobOpened: j.Opened, AppCreated: p.CreatedAt, PlacedAt: p.PlacedAt})
	}
	return TimeToFill(fills), titles, nil
}
