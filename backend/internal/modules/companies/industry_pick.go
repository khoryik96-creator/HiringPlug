package companies

// The AI's pick of a new company's industry, from the agency's own list
// (Settings > Industries): what the company says of itself (its website or
// LinkedIn overview), its name and website, and an industry named elsewhere
// (LinkedIn's) as a hint. A company without an industry gets the one picked,
// which anyone may change; one that has an industry keeps it.

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/classification"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// AI is the company's AI provider, if it has one and allowance left this month.
type AI interface {
	Allowed(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error)
}

// IndustryChoice is an industry or subindustry the AI may pick: its id and
// its words ("Financial Services > Banking"), with its industry's words.
type IndustryChoice struct{ ID, Label, Root string }

// UseAI wires the AI allowance (the industry pick).
func (m *Module) UseAI(ai AI) { m.service.ai = ai }

// queueIndustryAsk queues the ask for a company just added without an
// industry (a small job: adding a company does no AI bookkeeping itself), and
// marks the pick as under way so its page reads it again.
func (s *Service) queueIndustryAsk(ctx context.Context, tx pgx.Tx, tenantID, companyID string, d Details) error {
	if s.ai == nil || s.industries == nil || (d.IndustrySelections != nil && len(d.IndustrySelections.Ids) > 0) {
		return nil
	}
	now := time.Now()
	if err := store.New(tx).SetIndustryPickSince(ctx, store.SetIndustryPickSinceParams{ID: companyID, Since: &now}); err != nil {
		return err
	}
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindCompanyIndustryAsk, api.CompanyIndustryAskJob{CompanyId: companyID})
	return err
}

// industryAsk handles company.industry_ask.
func (s *Service) industryAsk(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.CompanyIndustryAskJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	return s.askIndustry(ctx, tx, job.TenantID, r.CompanyId)
}

// askIndustry asks the AI to pick the company's industry, when it has none,
// the AI is on and the agency has industries; otherwise the pick is no longer
// under way.
func (s *Service) askIndustry(ctx context.Context, tx pgx.Tx, tenantID, companyID string) error {
	if s.ai == nil || s.industries == nil {
		return nil
	}
	q := store.New(tx)
	row, err := q.GetCompany(ctx, companyID)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	done := func(err error) error {
		if err != nil {
			return err
		}
		return q.SetIndustryPickSince(ctx, store.SetIndustryPickSinceParams{ID: companyID})
	}
	chosen, err := s.industries.ReadIn(ctx, tx, "company", companyID)
	if err != nil || len(chosen.Ids) > 0 {
		return done(err)
	}
	access, on, err := s.ai.Allowed(ctx, tx)
	if err != nil || !on {
		return done(err)
	}
	_, choices, err := s.industries.ChoicesIn(ctx, tx)
	if err != nil || len(choices) == 0 {
		return done(err)
	}
	pick, err := q.InsertIndustryPick(ctx, companyID)
	if err != nil {
		return err
	}
	now := time.Now()
	if err := q.SetIndustryPickSince(ctx, store.SetIndustryPickSinceParams{ID: companyID, Since: &now}); err != nil {
		return err
	}
	job := api.CompanyIndustryJob{
		PickId: pick, CompanyName: row.Name, Website: apiconv.OrNull(blankNil(row.Website)), About: apiconv.OrNull(blankNil(row.About)),
		Hint: apiconv.OrNull(blankNil(row.IndustryHint)), Ai: access, // the hint: an industry named elsewhere (LinkedIn's)
	}
	for _, c := range choices {
		job.Choices = append(job.Choices, api.CompanyIndustryChoice{Id: c.ID, Label: c.Label})
	}
	_, err = jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindCompanyIndustry, job)
	return err
}

// industried handles company.industried: the AI's pick, given to the company
// if it still has no industry; the tokens are counted either way.
func (s *Service) industried(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.CompanyIndustriedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	params := store.SaveIndustryPickParams{
		ID: r.PickId, Status: "done", InputTokens: int32(min(r.InputTokens, 1<<30)), OutputTokens: int32(min(r.OutputTokens, 1<<30)), //nolint:gosec // capped
	}
	choice, _ := r.ChoiceId.Get()
	if choice != "" {
		params.ChoiceID = &choice
	}
	if v, err := r.Model.Get(); err == nil && v != "" {
		params.Model = clipPtr(v, 200)
	}
	if v, err := r.Error.Get(); err == nil && v != "" {
		params.Status, params.Error = "failed", clipPtr(v, 1000)
	}
	q := store.New(tx)
	companyID, err := q.SaveIndustryPick(ctx, params)
	if database.IsNoRows(err) {
		return nil // answered already
	}
	if err != nil {
		return err
	}
	if err := q.SetIndustryPickSince(ctx, store.SetIndustryPickSinceParams{ID: companyID}); err != nil { // answered: no longer awaited
		return err
	}
	if choice == "" || params.Status != "done" || s.industries == nil {
		return nil
	}
	if err := s.industries.LockIn(ctx, tx); err != nil {
		return err
	}
	if _, err := store.New(tx).GetCompany(ctx, companyID); database.IsNoRows(err) {
		return nil // deleted meanwhile
	} else if err != nil {
		return err
	}
	chosen, err := s.industries.ReadIn(ctx, tx, "company", companyID)
	if err != nil || len(chosen.Ids) > 0 {
		return err // someone chose one meanwhile: theirs stays
	}
	version, choices, err := s.industries.ChoicesIn(ctx, tx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(choices, func(c IndustryChoice) bool { return c.ID == choice })
	if i < 0 {
		return nil // not one of the agency's (any more)
	}
	root := choices[i].Root
	if _, err := s.industries.SaveIn(ctx, tx, "company", companyID,
		&classification.Selections{CatalogueVersion: version, Ids: []string{choice}, PrimaryId: &choice}, &root); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "company.industry_picked", EntityType: "company", EntityID: companyID,
		Changes: map[string]any{"industry": choices[i].Label, "by": "ai"}})
}

// blankNil is nil for an empty text.
func blankNil(v *string) *string {
	if v == nil || *v == "" {
		return nil
	}
	return v
}

func clipPtr(s string, n int) *string {
	out := clip(s, n)
	return &out
}

// TokensSince returns the AI tokens the company used for industry picks since
// the given time, in the caller's transaction (counted in the shared allowance).
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).IndustryPickTokensSince(ctx, since)
}
