package placements

// What reports read: placements recorded (each user's activity), and those
// not yet invoiced with their fee (the revenue forecast).

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Recorded is a placement recorded, credited to its recruiter (else to who
// recorded it).
type Recorded struct {
	CreditedTo string
	At         time.Time
}

// RecordedBetween returns the placements recorded in [from, to) that were
// not cancelled or backed out, in the caller's transaction.
func (m *Module) RecordedBetween(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Recorded, error) {
	rows, err := store.New(tx).RecordedBetween(ctx, store.RecordedBetweenParams{FromTime: from, ToTime: to})
	out := make([]Recorded, len(rows))
	for i, r := range rows {
		out[i] = Recorded{CreditedTo: r.CreditedTo, At: r.CreatedAt}
	}
	return out, err
}

// Uninvoiced is a placement that stands and is not yet invoiced. FeeAmount
// is nil when none is recorded or the user may not see fees (the database
// leaves them out).
type Uninvoiced struct {
	ApplicationID          string
	RecruiterID            *string
	StartDate              time.Time
	FeeAmount, FeeCurrency *string
}

// ToInvoice returns the placements that stand and are not yet invoiced, by
// start date, in the caller's transaction.
func (m *Module) ToInvoice(ctx context.Context, tx pgx.Tx) ([]Uninvoiced, error) {
	rows, err := store.New(tx).PlacementsToInvoice(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Uninvoiced, len(rows))
	for i, r := range rows {
		fee, err := database.DecimalString(r.FeeAmount)
		if err != nil {
			return nil, err
		}
		out[i] = Uninvoiced{ApplicationID: r.ApplicationID, RecruiterID: r.RecruiterID, StartDate: r.StartDate, FeeAmount: fee, FeeCurrency: r.FeeCurrency}
	}
	return out, nil
}
