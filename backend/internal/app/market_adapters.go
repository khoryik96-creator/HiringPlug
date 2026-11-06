package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/market"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// marketJobs gives the market map a job's title, as p may see it.
type marketJobs struct{ jobs *jobs.Module }

func (a marketJobs) Title(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (string, error) {
	if !p.Can(api.PermissionJobsRead) {
		return "", apperr.NotFound("job")
	}
	v, err := a.jobs.ViewIn(ctx, tx, p, jobID)
	return v.Details.Title, err
}

// marketCandidates gives the market map the counted details of candidates.
type marketCandidates struct{ candidates *candidates.Module }

func (a marketCandidates) Members(ctx context.Context, tx pgx.Tx, p authz.Principal, words string) ([]market.Member, []string, error) {
	rows, err := a.candidates.MarketPool(ctx, tx, p, words)
	if err != nil {
		return nil, nil, err
	}
	out, ids := make([]market.Member, len(rows)), make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		out[i] = market.Member{
			Company: deref(r.CurrentCompany), Location: deref(r.Location), Country: deref(r.ResidenceCountry),
			Availability: deref(r.Availability), WorkAuth: deref(r.WorkAuthorization), WorkBasis: deref(r.WorkAuthorizationBasis),
			NoticeUnit: deref(r.NoticeUnit),
		}
		if r.NoticeValue != nil {
			n := int(*r.NoticeValue)
			out[i].NoticeValue = &n
		}
	}
	return out, ids, nil
}

func (a marketCandidates) Pay(ctx context.Context, tx pgx.Tx, p authz.Principal, ids []string) ([]market.Pay, error) {
	rows, err := a.candidates.MarketPay(ctx, tx, p, ids)
	out := make([]market.Pay, len(rows))
	for i, r := range rows {
		out[i] = market.Pay(r)
	}
	return out, err
}

// marketInsights gives the market map the user's newest AI market insights.
type marketInsights struct{ assistant *assistant.Module }

func (a marketInsights) Latest(ctx context.Context, tx pgx.Tx, jobID string) (*market.Answer, error) {
	in, ok, err := a.assistant.LatestMarketMap(ctx, tx, jobID)
	if err != nil || !ok {
		return nil, err //nolint:nilnil // no insights yet: none to show
	}
	return &market.Answer{ID: in.ID, Status: in.Status, Text: in.Text, Model: in.Model, AskedAt: in.AskedAt}, nil
}

// assistantMarket gives the AI's market insights the database pool; the
// market module is built after the assistant.
type assistantMarket struct{ m *Modules }

func (a assistantMarket) PoolText(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (string, error) {
	return a.m.Market.PoolText(ctx, tx, p, jobID)
}
