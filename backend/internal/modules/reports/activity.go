package reports

// Each user's activity in a week or month against their targets, and the
// targets and stage weights owners and admins set.

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/reports/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ActivityRow is a user's activity, with their target (nil when none).
type ActivityRow struct {
	UserID, Name string
	Done         Counts
	Target       *Goal
}

// ActivityReport is each user's activity from one day to another.
type ActivityReport struct {
	Period   string // week, month, or "" for any other period
	From, To time.Time
	Team     bool // everyone is listed; else only the user
	Rows     []ActivityRow
}

func (s *Service) today() time.Time {
	now := time.Now().In(s.loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// Activity is each user's activity in the week or month that day is in
// (today when nil), against their targets.
func (s *Service) Activity(ctx context.Context, p authz.Principal, period string, day *time.Time) (ActivityReport, error) {
	if period != Month {
		period = Week
	}
	d := s.today()
	if day != nil {
		d = *day
	}
	first, last := PeriodOf(period, d)
	var out ActivityReport
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.activityIn(ctx, tx, p, first, last)
		if err != nil {
			return err
		}
		out.Period = period
		targets, err := listTargets(ctx, tx)
		for i, r := range out.Rows {
			if g, ok := TargetFor(targets, r.UserID, period); ok {
				out.Rows[i].Target = &g
			}
		}
		return err
	})
	return out, err
}

// activityIn counts each user's activity from first to last (days), in tx:
// every active user (and anyone else who did something) with
// reports.team:read, else only p.
func (s *Service) activityIn(ctx context.Context, tx pgx.Tx, p authz.Principal, first, last time.Time) (ActivityReport, error) {
	out := ActivityReport{From: first, To: last, Team: p.Can(api.PermissionReportsTeamRead)}
	start := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, s.loc)
	end := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, s.loc)
	src := s.deps.Activity
	var f Facts
	var err error
	if f.ScreeningCalls, err = src.ScreeningCalls(ctx, tx, first, last); err != nil {
		return out, err
	}
	if f.CallNotes, err = src.CallNotes(ctx, tx, start, end); err != nil {
		return out, err
	}
	var screening []string
	for _, n := range f.CallNotes {
		if n.Kind == "screening" {
			screening = append(screening, n.ID)
		}
	}
	linked, err := src.ScreeningCallNotes(ctx, tx, screening)
	if err != nil {
		return out, err
	}
	f.ScreeningNotes = map[string]bool{}
	for _, id := range linked {
		f.ScreeningNotes[id] = true
	}
	if f.Submitted, err = src.Submitted(ctx, tx, start, end); err != nil {
		return out, err
	}
	if f.Emailed, err = src.CVsEmailed(ctx, tx, start, end); err != nil {
		return out, err
	}
	if f.Interviews, err = src.Interviews(ctx, tx, start, end); err != nil {
		return out, err
	}
	if f.Placements, err = src.Placements(ctx, tx, start, end); err != nil {
		return out, err
	}
	counts := CountActivity(f, func(t time.Time) time.Time { t = t.In(s.loc); return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) })
	names := map[string]string{}
	if out.Team {
		members, err := src.Members(ctx, tx)
		if err != nil {
			return out, err
		}
		for _, m := range members {
			names[m.ID] = m.Name
		}
		var others []string // left since, or not active: named when they did something
		for id := range counts {
			if _, ok := names[id]; !ok {
				others = append(others, id)
			}
		}
		more, err := s.deps.Users.UserNames(ctx, tx, others)
		if err != nil {
			return out, err
		}
		for id, n := range more {
			names[id] = n
		}
	} else {
		names[p.UserID] = p.Name
	}
	for id, name := range names {
		row := ActivityRow{UserID: id, Name: name}
		if c := counts[id]; c != nil {
			row.Done = *c
		}
		out.Rows = append(out.Rows, row)
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].Name != out.Rows[j].Name {
			return out.Rows[i].Name < out.Rows[j].Name
		}
		return out.Rows[i].UserID < out.Rows[j].UserID
	})
	return out, nil
}

func i32(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func to32(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v) //nolint:gosec // checked: at most 100000
	return &n
}

func listTargets(ctx context.Context, tx pgx.Tx) ([]Target, error) {
	rows, err := store.New(tx).ListTargets(ctx)
	out := make([]Target, len(rows))
	for i, r := range rows {
		out[i] = Target{UserID: r.UserID, Period: r.Period, Goal: Goal{Calls: i32(r.Calls), CVsSent: i32(r.CvsSent), Interviews: i32(r.Interviews), Placements: i32(r.Placements)}}
	}
	return out, err
}

func listWeights(ctx context.Context, tx pgx.Tx) ([]StageWeight, error) {
	rows, err := store.New(tx).ListStageWeights(ctx)
	saved := make([]StageWeight, len(rows))
	for i, r := range rows {
		saved[i] = StageWeight{Stage: r.Stage, Percent: int(r.Percent), MonthsAhead: int(r.MonthsAhead)}
	}
	return Weights(saved), err
}

// Settings are the activity targets and the forecast's stage weights.
type Settings struct {
	Targets []Target
	Weights []StageWeight
}

// Settings returns the targets and weights.
func (s *Service) Settings(ctx context.Context, p authz.Principal) (Settings, error) {
	var out Settings
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		if out.Targets, err = listTargets(ctx, tx); err != nil {
			return err
		}
		out.Weights, err = listWeights(ctx, tx)
		return err
	})
	return out, err
}

// SaveSettings replaces the targets, and sets the weights given (stages left
// out keep theirs).
func (s *Service) SaveSettings(ctx context.Context, p authz.Principal, in Settings) (Settings, error) {
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		members, err := s.deps.Activity.Members(ctx, tx)
		if err != nil {
			return err
		}
		active := map[string]bool{}
		for _, m := range members {
			active[m.ID] = true
		}
		if err := CheckSettings(in.Targets, in.Weights, active).Err(); err != nil {
			return err
		}
		q := store.New(tx)
		if err := q.DeleteTargets(ctx); err != nil {
			return err
		}
		for _, t := range in.Targets {
			if err := q.InsertTarget(ctx, store.InsertTargetParams{
				UserID: t.UserID, Period: t.Period, Calls: to32(t.Goal.Calls), CvsSent: to32(t.Goal.CVsSent),
				Interviews: to32(t.Goal.Interviews), Placements: to32(t.Goal.Placements),
			}); err != nil {
				return err
			}
		}
		weights := map[string]any{}
		for _, w := range in.Weights {
			if err := q.PutStageWeight(ctx, store.PutStageWeightParams{
				Stage: w.Stage, Percent: int32(w.Percent), MonthsAhead: int32(w.MonthsAhead), //nolint:gosec // checked: small
			}); err != nil {
				return err
			}
			weights[w.Stage] = map[string]int{"percent": w.Percent, "monthsAhead": w.MonthsAhead}
		}
		// Counts and percentages only: no money in the log.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "reports.settings_saved", EntityType: "tenant", EntityID: p.TenantID,
			Changes: map[string]any{"targets": len(in.Targets), "stageWeights": weights},
		})
	})
	if err != nil {
		return Settings{}, err
	}
	return s.Settings(ctx, p)
}
