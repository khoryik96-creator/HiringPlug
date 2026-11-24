package aiusage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
)

// Alert is a level this month's use reached: the company's warning percent,
// or LimitLevel (the whole limit).
type Alert struct {
	Level     int
	ReachedAt time.Time
	Month     time.Time // when the month began (agency time)
	ResetsAt  time.Time // when the next month begins: the limit starts again
}

// monthDate is the month that began at start, as the alert table stores it.
func monthDate(start time.Time) pgtype.Date {
	y, m, _ := start.Date()
	return pgtype.Date{Time: time.Date(y, m, 1, 0, 0, 0, 0, time.UTC), Valid: true}
}

// noteLevels stores the levels this month's use reached for the first time
// and audits each once (the percent and month only). It runs in tx as the
// system for the company, right after a use was recorded.
func (s *Service) noteLevels(ctx context.Context, tx pgx.Tx, q *store.Queries, tenantID string) error {
	set, err := readSettings(ctx, q)
	if err != nil {
		return err
	}
	now := s.now()
	start := MonthStart(now, s.loc())
	use, err := q.UseSince(ctx, start)
	if err != nil {
		return err
	}
	levels := LevelsReached(set.Limit(s.config.DefaultTokens), Spent{Input: use.InputTokens, Output: use.OutputTokens})
	for _, level := range levels {
		n, err := q.ReachLevel(ctx, store.ReachLevelParams{Month: monthDate(start), Level: int16(level)}) //nolint:gosec // 1..100
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		action := "ai_usage.warning_reached"
		if level == LimitLevel {
			action = "ai_usage.limit_reached"
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: action, EntityType: "tenant", EntityID: tenantID,
			Changes: map[string]any{"percent": level, "month": MonthKey(now, s.loc())},
		}); err != nil {
			return err
		}
	}
	return nil
}

// alerts is the levels now's month reached, read in the caller's transaction.
func (s *Service) alerts(ctx context.Context, tx pgx.Tx, now time.Time) ([]Alert, error) {
	start := MonthStart(now, s.loc())
	rows, err := store.New(tx).LevelsReached(ctx, monthDate(start))
	if err != nil {
		return nil, err
	}
	out := make([]Alert, len(rows))
	for i, r := range rows {
		out[i] = Alert{Level: int(r.Level), ReachedAt: r.ReachedAt, Month: start, ResetsAt: NextMonthStart(now, s.loc())}
	}
	return out, nil
}
