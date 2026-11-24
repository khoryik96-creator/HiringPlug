package aiusage

// The month-by-month report of the company's AI use.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiusage/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Months the report lists, at most and unless asked.
const (
	MaxReportMonths     = 24
	DefaultReportMonths = 12
)

// Report is this month's use (by feature and by person), the months before
// it, the limit and the prices costs are worked out at.
type Report struct {
	Months   []MonthUse // this month first
	People   []PersonUse
	Limit    Limit
	Currency string
	Status   Status
}

// Usage returns the report for this month and the months-1 before it.
func (s *Service) Usage(ctx context.Context, p authz.Principal, months int) (Report, error) {
	if months < 1 || months > MaxReportMonths {
		var errs apperr.FieldErrors
		errs.Add("months", "must be between 1 and 24")
		return Report{}, errs.Err()
	}
	now, loc := s.now(), s.loc()
	start := MonthStart(now, loc)
	var out Report
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		set, spent, err := s.month(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		out.Limit, out.Currency = set.Limit(s.config.DefaultTokens), set.Currency
		out.Status = statusOf(out.Limit, spent, now, loc)
		return database.AsSystem(ctx, tx, p.TenantID, func() error {
			q := store.New(tx)
			rows, err := q.UseByMonthAndFeature(ctx, store.UseByMonthAndFeatureParams{
				TimeZone: loc.String(), Since: start.AddDate(0, -(months - 1), 0),
			})
			if err != nil {
				return err
			}
			byMonth := make([]MonthRow, len(rows))
			for i, r := range rows {
				byMonth[i] = MonthRow{Month: r.Month, Feature: r.Feature, Uses: r.Uses, Spent: Spent{Input: r.InputTokens, Output: r.OutputTokens}}
			}
			out.Months = MonthsOf(MonthKeys(now, loc, months), byMonth)
			out.People, err = s.people(ctx, tx, q, start)
			return err
		})
	})
	return out, err
}

// people is the use per person since a time, named.
func (s *Service) people(ctx context.Context, tx pgx.Tx, q *store.Queries, since time.Time) ([]PersonUse, error) {
	rows, err := q.UseByPerson(ctx, since)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, r := range rows {
		if r.UserID != nil {
			ids = append(ids, *r.UserID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 && s.deps.Users != nil {
		if names, err = s.deps.Users.UserNames(ctx, tx, ids); err != nil {
			return nil, err
		}
	}
	out := make([]PersonUse, len(rows))
	for i, r := range rows {
		name := AutomaticName
		if r.UserID != nil {
			if name = names[*r.UserID]; name == "" {
				name = FormerUserName
			}
		}
		out[i] = PersonUse{UserID: r.UserID, Name: name, Uses: r.Uses, Spent: Spent{Input: r.InputTokens, Output: r.OutputTokens}}
	}
	SortPeople(out)
	return out, nil
}
