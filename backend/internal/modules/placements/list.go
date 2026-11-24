package placements

// The placements list (the dashboard, and an application's placement).

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// Filter narrows the list of placements.
type Filter struct {
	ApplicationID, Status, InvoiceStatus *string
	StartFrom, StartTo                   *time.Time
	// IncludeSalary: the agreed salaries are shown (and counted as shown);
	// without it they are left out of the money.
	IncludeSalary bool
}

// List returns placements, newest start first (at most 500), and (with
// placements.fees:read) the fee totals of every matching one that stands.
// Listed is a page of the list: the placements (at most 500), how many match
// in all, and with placements.fees:read their fee totals.
type Listed struct {
	Items  []View
	Total  int
	Totals []FeeTotal
}

func (s *Service) List(ctx context.Context, p authz.Principal, f Filter) (Listed, error) {
	var out []View
	var totals []FeeTotal
	var total int64
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListPlacements(ctx, store.ListPlacementsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		if err != nil {
			return err
		}
		if total, err = q.CountPlacements(ctx, store.CountPlacementsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		}); err != nil {
			return err
		}
		if out, err = s.views(ctx, tx, p, rows); err != nil {
			return err
		}
		if !f.IncludeSalary {
			withoutSalaries(out)
		} else if err := salaryview.In(ctx, tx, s.salaryViews, salaryview.Placement, salaryOf(out)...); err != nil {
			return err
		}
		if !p.Can(api.PermissionPlacementsFeesRead) {
			return nil
		}
		sums, err := q.PlacementFeeTotals(ctx, store.PlacementFeeTotalsParams{
			ApplicationID: f.ApplicationID, Status: f.Status, InvoiceStatus: f.InvoiceStatus, StartFrom: f.StartFrom, StartTo: f.StartTo,
		})
		if err != nil {
			return err
		}
		moved, err := s.retainerLines(ctx, tx, func(r store.Placement) bool {
			return (f.ApplicationID == nil || r.ApplicationID == *f.ApplicationID) && (f.Status == nil || r.Status == *f.Status) &&
				(f.InvoiceStatus == nil || r.InvoiceStatus == *f.InvoiceStatus) && inPeriod(r.StartDate, f.StartFrom, f.StartTo)
		})
		totals = feeTotals(len(sums), func(i int) (string, string, string) { return sums[i].Currency, sums[i].InvoiceStatus, sums[i].Total }, moved...)
		return err
	})
	return Listed{Items: out, Total: int(total), Totals: totals}, err
}
