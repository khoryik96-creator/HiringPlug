package app

// Adapters giving the assistant module what it needs from other modules.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type assistantCandidates struct{ candidates *candidates.Module }

// rightToWork says a work authorization in words (no note: it is free text
// about the person).
var rightToWork = map[string]string{
	"local":          "local (no work pass needed)",
	"expat_eligible": "foreigner who may already work here",
	"requires_visa":  "needs a work pass",
}

func (a assistantCandidates) ForAssistant(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (assistant.Candidate, error) {
	v, err := a.candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return assistant.Candidate{}, err
	}
	pr := v.Profile
	c := assistant.Candidate{
		ID: v.ID, Name: pr.FirstName + " " + pr.LastName, Email: deref(pr.Email), Phone: deref(pr.Phone),
		CurrentTitle: pr.CurrentTitle, CurrentCompany: pr.CurrentCompany, Location: pr.Location,
		Industry: pr.Industry, Specialization: pr.Specialization, Skills: pr.Skills,
	}
	if w := pr.WorkAuthorization; w != nil {
		s := rightToWork[w.Status]
		c.WorkAuthorization = &s
	}
	if n := pr.NoticePeriod; n != nil {
		s := "immediately"
		if n.Value > 0 {
			s = fmt.Sprintf("%d %s(s)", n.Value, n.Unit)
		}
		c.NoticePeriod = &s
	}
	for _, j := range v.History.Work {
		c.Work = append(c.Work, assistant.Work{Title: j.Title, Company: j.Company, StartDate: j.StartDate, EndDate: j.EndDate, Current: j.Current, Description: j.Description})
	}
	for _, q := range v.History.Education {
		c.Education = append(c.Education, assistant.Qualification{Qualification: q.Qualification, Institution: q.Institution, EndYear: q.EndYear})
	}
	return c, nil // v.Compensation is never passed on: salary never goes to the AI
}

type assistantJobs struct{ jobs *jobs.Module }

func (a assistantJobs) ForAssistant(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (assistant.Job, error) {
	v, err := a.jobs.ViewIn(ctx, tx, p, id)
	if err != nil {
		return assistant.Job{}, err
	}
	d := v.Details
	return assistant.Job{
		ID: v.ID, Title: d.Title, CompanyName: v.CompanyName, Status: d.Status, Location: d.Location, WorkType: d.WorkType,
		WorkArrangement: d.WorkArrangement, Description: d.Description, Openings: d.Openings,
	}, nil // v.Salary is never passed on
}

type assistantApplications struct{ pipeline *pipeline.Module }

func (a assistantApplications) ForAssistant(ctx context.Context, tx pgx.Tx, id string) (assistant.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, []string{id})
	if err != nil {
		return assistant.Application{}, err
	}
	b, ok := briefs[id]
	if !ok {
		return assistant.Application{}, apperr.NotFound("application")
	}
	return assistant.Application{ID: b.ID, CandidateID: b.CandidateID, JobID: b.JobID, Stage: b.Stage}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
