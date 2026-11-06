package candidates

// What a job's market map reads from candidates: only the counted details,
// never names or contact details; pay only for roles that may see salaries.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// PoolMember is one candidate in a market map.
type PoolMember struct {
	ID                                                      string
	CurrentCompany, Location, ResidenceCountry              *string
	Availability, WorkAuthorization, WorkAuthorizationBasis *string
	NoticeValue                                             *int32
	NoticeUnit                                              *string
}

// PoolPay is a pool member's pay ("" when not recorded).
type PoolPay struct {
	CurrentAmount, CurrentCurrency, CurrentPeriod    string
	ExpectedAmount, ExpectedCurrency, ExpectedPeriod string
}

// MarketPool returns the candidates p may see whose current title has every
// word of words, in the caller's transaction.
func (m *Module) MarketPool(ctx context.Context, tx pgx.Tx, p authz.Principal, words string) ([]PoolMember, error) {
	if !p.Can(api.PermissionCandidatesRead) {
		return nil, nil
	}
	rows, err := store.New(tx).MarketPool(ctx, words)
	if err != nil {
		return nil, err
	}
	out := make([]PoolMember, len(rows))
	for i, r := range rows {
		out[i] = PoolMember{
			ID: r.ID, CurrentCompany: r.CurrentCompany, Location: r.Location, ResidenceCountry: r.ResidenceCountry,
			Availability: r.Availability, WorkAuthorization: r.WorkAuthorization, WorkAuthorizationBasis: r.WorkAuthorizationBasis,
			NoticeValue: r.NoticePeriodValue, NoticeUnit: r.NoticePeriodUnit,
		}
	}
	return out, nil
}

// MarketPay returns the pay recorded for the given candidates; nothing for a
// role that may not see salaries.
func (m *Module) MarketPay(ctx context.Context, tx pgx.Tx, p authz.Principal, ids []string) ([]PoolPay, error) {
	if !p.Can(api.PermissionCandidatesCompensationRead) || len(ids) == 0 {
		return nil, nil
	}
	rows, err := store.New(tx).MarketPay(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]PoolPay, len(rows))
	for i, r := range rows {
		out[i] = PoolPay{
			CurrentAmount: r.CurrentAmount, CurrentCurrency: deref(r.CurrentCurrency), CurrentPeriod: deref(r.CurrentPeriod),
			ExpectedAmount: r.ExpectedMin, ExpectedCurrency: deref(r.ExpectedCurrency), ExpectedPeriod: deref(r.ExpectedPeriod),
		}
	}
	return out, nil
}
