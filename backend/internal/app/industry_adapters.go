package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type industryRecords struct{ m *Modules }

func (a industryRecords) LegacyIndustries(ctx context.Context, tx pgx.Tx) ([]industries.LegacyIndustry, error) {
	candidates, e := a.m.Candidates.LegacyIndustries(ctx, tx)
	if e != nil {
		return nil, e
	}
	companies, e := a.m.Companies.LegacyIndustries(ctx, tx)
	out := []industries.LegacyIndustry{}
	for id, label := range candidates {
		out = append(out, industries.LegacyIndustry{Kind: "candidate", ID: id, Label: label})
	}
	for id, label := range companies {
		out = append(out, industries.LegacyIndustry{Kind: "company", ID: id, Label: label})
	}
	return out, e
}
func (a industryRecords) SetIndustrySummary(ctx context.Context, tx pgx.Tx, kind, id string, label *string) error {
	if kind == "candidate" {
		return a.m.Candidates.SetIndustrySummary(ctx, tx, id, label)
	}
	return a.m.Companies.SetIndustrySummary(ctx, tx, id, label)
}
func (a industryRecords) SetIndustrySummaries(ctx context.Context, tx pgx.Tx, kind string, ids, labels []string) error {
	if kind == "candidate" {
		return a.m.Candidates.SetIndustrySummaries(ctx, tx, ids, labels)
	}
	return a.m.Companies.SetIndustrySummaries(ctx, tx, ids, labels)
}
func (a industryRecords) EmployerEvidence(ctx context.Context, tx pgx.Tx, id string) (industries.Evidence, error) {
	c, e := a.m.Candidates.ViewIn(ctx, tx, authz.MustFromContext(ctx), id)
	if e != nil {
		return industries.Evidence{}, e
	}
	out := industries.Evidence{Version: c.Version}
	for _, j := range c.History.Work {
		out.Employers = append(out.Employers, industries.Employment{Name: plainString(j.Company), Title: plainString(j.Title), Start: plainString(j.StartDate), End: plainString(j.EndDate), Current: j.Current})
	}
	if len(out.Employers) == 0 && c.Profile.CurrentCompany != nil {
		out.Employers = append(out.Employers, industries.Employment{Name: *c.Profile.CurrentCompany, Title: plainString(c.Profile.CurrentTitle), Current: true})
	}
	return out, nil
}
func (a industryRecords) VerifiedEmployer(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	id, e := a.m.Companies.IndustryEmployerID(ctx, tx, name)
	if e != nil || id == "" {
		return "", e
	}
	v, e := a.m.Industries.ReadIn(ctx, tx, "company", id)
	if e != nil || v.PrimaryId == nil {
		return "", e
	}
	return *v.PrimaryId, nil
}
func plainString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
