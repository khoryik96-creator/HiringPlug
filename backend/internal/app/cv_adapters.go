package app

// Adapters giving the cvformats module candidates and applications, in its
// own types.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type cvCandidates struct{ candidates *candidates.Module }

func (a cvCandidates) ForCV(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (cvformats.Candidate, error) {
	v, err := a.candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return cvformats.Candidate{}, err
	}
	pr := v.Profile
	c := cvformats.Candidate{
		ID: v.ID, FirstName: pr.FirstName, LastName: pr.LastName, Email: pr.Email, Phone: pr.Phone, Nationality: pr.Nationality,
		Location: pr.Location, CurrentTitle: pr.CurrentTitle, CurrentCompany: pr.CurrentCompany, Skills: pr.Skills,
	}
	if n := pr.NoticePeriod; n != nil {
		c.NoticeValue, c.NoticeUnit = &n.Value, n.Unit
	}
	if w := pr.WorkAuthorization; w != nil {
		c.WorkAuthorization, c.WorkAuthorizationBasis = &w.Status, w.Basis
	}
	for _, j := range v.History.Work {
		c.Work = append(c.Work, cvformats.Job{Title: j.Title, Company: j.Company, StartDate: j.StartDate, EndDate: j.EndDate, Current: j.Current, Description: j.Description})
	}
	for _, q := range v.History.Education {
		c.Education = append(c.Education, cvformats.Qualification{Qualification: q.Qualification, Institution: q.Institution, EndYear: q.EndYear})
	}
	if comp := v.Compensation; comp != nil { // only there when the user may see salary
		if s := comp.Current; s != nil {
			c.CurrentSalary = &cvformats.Salary{Min: s.Amount, Currency: s.Currency, Period: s.Period}
		}
		if s := comp.Expected; s != nil {
			c.ExpectedSalary = &cvformats.Salary{Min: s.Min, Max: s.Max, Currency: s.Currency, Period: s.Period}
		}
	}
	return c, nil
}

type cvApplications struct{ pipeline *pipeline.Module }

func (a cvApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]cvformats.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	out := make(map[string]cvformats.Application, len(briefs))
	for id, b := range briefs {
		out[id] = cvformats.Application{CandidateID: b.CandidateID, JobTitle: b.JobTitle, CompanyName: b.CompanyName}
	}
	return out, err
}

// cvSources gives cvformats a CV to format CV Studio's way (documents module).
type cvSources struct{ documents *documents.Module }

func (a cvSources) SourceCV(ctx context.Context, tx pgx.Tx, id string) (cvformats.SourceCV, string, error) {
	d, url, err := a.documents.SourceCV(ctx, tx, id)
	return cvformats.SourceCV{ID: d.ID, CandidateID: d.CandidateID, Source: d.Source, FileName: d.FileName,
		ContentType: d.ContentType, SizeBytes: d.SizeBytes}, url, err
}
