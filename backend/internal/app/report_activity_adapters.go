package app

// Adapters giving the reports module what users did (activity against
// targets) and what the open pipeline would bring (the revenue forecast),
// read from other modules in its own types.

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/reports"
)

type reportActivity struct {
	candidates *candidates.Module
	notes      *notes.Module
	pipeline   *pipeline.Module
	emails     *emails.Module
	interviews *interviews.Module
	placements *placements.Module
	identity   *identity.Module
}

func (a reportActivity) ScreeningCalls(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]reports.ScreeningCall, error) {
	calls, err := a.candidates.ScreeningCallsBetween(ctx, tx, fromDay, toDay)
	out := make([]reports.ScreeningCall, len(calls))
	for i, c := range calls {
		out[i] = reports.ScreeningCall{By: c.CalledBy, Day: c.CalledOn, NoteID: c.NoteID}
	}
	return out, err
}

func (a reportActivity) CallNotes(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.CallNote, error) {
	logged, err := a.notes.CallsLogged(ctx, tx, from, to)
	out := make([]reports.CallNote, len(logged))
	for i, n := range logged {
		out[i] = reports.CallNote{ID: n.ID, Kind: n.Kind, By: n.AuthorID, At: n.At}
	}
	return out, err
}

func (a reportActivity) ScreeningCallNotes(ctx context.Context, tx pgx.Tx, noteIDs []string) ([]string, error) {
	return a.candidates.ScreeningCallNotes(ctx, tx, noteIDs)
}

// tooMuch is a source's "more than one read takes" as the reports module's.
func tooMuch(err error) error {
	if errors.Is(err, pipeline.ErrTooManyMoves) || errors.Is(err, emails.ErrTooManyEmails) || errors.Is(err, placements.ErrTooManyPlacements) {
		return reports.ErrTooMuchData
	}
	return err
}

func (a reportActivity) Submitted(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.CVSent, error) {
	moves, err := a.pipeline.MovesTo(ctx, tx, pipeline.Submitted, from, to)
	err = tooMuch(err)
	out := make([]reports.CVSent, len(moves))
	for i, m := range moves {
		out[i] = reports.CVSent{By: m.By, CandidateID: m.CandidateID, At: m.At}
	}
	return out, err
}

func (a reportActivity) CVsEmailed(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.CVSent, error) {
	sent, err := a.emails.CVsEmailed(ctx, tx, from, to)
	err = tooMuch(err)
	out := make([]reports.CVSent, len(sent))
	for i, s := range sent {
		out[i] = reports.CVSent{By: s.SenderID, CandidateID: s.CandidateID, At: s.At}
	}
	return out, err
}

func (a reportActivity) Interviews(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.Done, error) {
	arranged, err := a.interviews.ArrangedBetween(ctx, tx, from, to)
	out := make([]reports.Done, len(arranged))
	for i, d := range arranged {
		out[i] = reports.Done{By: d.By, At: d.At}
	}
	return out, err
}

func (a reportActivity) Placements(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.Done, error) {
	recorded, err := a.placements.RecordedBetween(ctx, tx, from, to)
	out := make([]reports.Done, len(recorded))
	for i, d := range recorded {
		out[i] = reports.Done{By: d.CreditedTo, At: d.At}
	}
	return out, err
}

func (a reportActivity) CategoryMoves(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.CategoryMove, error) {
	moves, err := a.pipeline.MovesToCategory(ctx, tx, from, to)
	err = tooMuch(err)
	out := make([]reports.CategoryMove, len(moves))
	for i, m := range moves {
		out[i] = reports.CategoryMove{By: m.By, ApplicationID: m.ApplicationID, CandidateID: m.CandidateID, Category: m.Category, At: m.At}
	}
	return out, err
}

func (a reportActivity) Categories(ctx context.Context, tx pgx.Tx) ([]string, error) {
	cats, err := a.pipeline.CategoriesIn(ctx, tx)
	out := make([]string, len(cats))
	for i, c := range cats {
		out[i] = c.Key
	}
	return out, err
}

// Members are the active users who recruit: those whose role may move
// candidates through the pipeline (not hiring managers or viewers).
func (a reportActivity) Members(ctx context.Context, tx pgx.Tx) ([]reports.Member, error) {
	members, err := a.identity.ActiveMembers(ctx, tx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	perms, err := a.identity.ActiveUsers(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	var out []reports.Member
	for _, m := range members {
		if slices.Contains(perms[m.ID], api.PermissionPipelineUpdate) {
			out = append(out, reports.Member{ID: m.ID, Name: m.Name})
		}
	}
	return out, nil
}

type reportForecast struct {
	pipeline   *pipeline.Module
	placements *placements.Module
	jobs       *jobs.Module
	companies  *companies.Module
}

func (a reportForecast) AtStages(ctx context.Context, tx pgx.Tx, stages []string) ([]reports.OpenApplication, error) {
	briefs, err := a.pipeline.AtStages(ctx, tx, stages)
	out := make([]reports.OpenApplication, len(briefs))
	for i, b := range briefs {
		out[i] = reports.OpenApplication{ID: b.ID, Stage: b.Stage, JobID: b.JobID, JobTitle: b.JobTitle, CompanyName: b.CompanyName, CandidateName: b.CandidateName}
	}
	return out, err
}

func (a reportForecast) ToInvoice(ctx context.Context, tx pgx.Tx) ([]reports.Uninvoiced, error) {
	rows, err := a.placements.ToInvoice(ctx, tx)
	out := make([]reports.Uninvoiced, len(rows))
	for i, r := range rows {
		out[i] = reports.Uninvoiced{ApplicationID: r.ApplicationID, RecruiterID: r.RecruiterID, StartDate: r.StartDate, FeeAmount: r.FeeAmount, FeeCurrency: r.FeeCurrency}
	}
	return out, err
}

func (a reportForecast) WithPlacement(ctx context.Context, tx pgx.Tx, applicationIDs []string) (map[string]bool, error) {
	return a.placements.OnApplications(ctx, tx, applicationIDs)
}

func (a reportForecast) Jobs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]reports.FeeJob, error) {
	briefs, err := a.jobs.Briefs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	budgets, err := a.jobs.SalaryBudgets(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]reports.FeeJob, len(briefs))
	for id, b := range briefs {
		j := reports.FeeJob{CompanyID: b.CompanyID, OwnerID: b.OwnerID}
		if s, ok := budgets[id]; ok {
			j.Budget = &reports.Budget{Min: s.Min, Max: s.Max, Currency: s.Currency, Period: s.Period}
		}
		out[id] = j
	}
	return out, nil
}

func (a reportForecast) Terms(ctx context.Context, tx pgx.Tx, companyID string) (reports.Terms, error) {
	t, err := a.companies.Terms(ctx, tx, companyID)
	out := reports.Terms{Set: t.Version > 0, Currency: t.SalaryCurrency, Rates: make([]reports.Rate, len(t.Tiers))}
	for i, r := range t.Tiers {
		out.Rates[i] = reports.Rate{From: r.SalaryFrom, Below: r.SalaryBelow, Percent: r.FeePercent}
	}
	return out, err
}
