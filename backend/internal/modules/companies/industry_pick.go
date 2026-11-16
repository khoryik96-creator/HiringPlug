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
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
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

// askIndustry asks the AI to pick the company's industry, when it has none,
// the AI is on and the agency has industries; otherwise it does nothing.
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
	chosen, err := s.industries.ReadIn(ctx, tx, "company", companyID)
	if err != nil || len(chosen.Ids) > 0 {
		return err
	}
	access, on, err := s.ai.Allowed(ctx, tx)
	if err != nil || !on {
		return err
	}
	_, choices, err := s.industries.ChoicesIn(ctx, tx)
	if err != nil || len(choices) == 0 {
		return err
	}
	pick, err := q.InsertIndustryPick(ctx, companyID)
	if err != nil {
		return err
	}
	job := api.CompanyIndustryJob{
		PickId: pick, CompanyName: row.Name, Website: orNullStr(row.Website), About: orNullStr(row.About), Ai: access,
		Hint: nullable.NewNullNullable[string](),
	}
	// An industry named elsewhere (LinkedIn's), kept for this only.
	if h := row.IndustryHint; h != nil && *h != "" {
		job.Hint = nullable.NewNullableWithValue(*h)
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
	companyID, err := store.New(tx).SaveIndustryPick(ctx, params)
	if database.IsNoRows(err) {
		return nil // answered already
	}
	if err != nil || choice == "" || params.Status != "done" || s.industries == nil {
		return err
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

func orNullStr(v *string) nullable.Nullable[string] {
	if v == nil || *v == "" {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
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
