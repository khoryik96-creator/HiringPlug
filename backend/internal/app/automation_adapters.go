package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// automationApplications gives automations an application with its job's
// client and owner (pipeline and jobs modules).
type automationApplications struct {
	pipeline *pipeline.Module
	jobs     *jobs.Module
}

func (a automationApplications) Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]automations.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	if err != nil || len(briefs) == 0 {
		return map[string]automations.Application{}, err
	}
	jobIDs := make([]string, 0, len(briefs))
	for _, b := range briefs {
		jobIDs = append(jobIDs, b.JobID)
	}
	js, err := a.jobs.Briefs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]automations.Application, len(briefs))
	for id, b := range briefs {
		j := js[b.JobID]
		out[id] = automations.Application{
			Stage: b.Stage, CandidateID: b.CandidateID, CandidateName: b.CandidateName, JobID: b.JobID, JobTitle: b.JobTitle,
			CompanyID: j.CompanyID, CompanyName: b.CompanyName, RecruiterID: j.OwnerID,
		}
	}
	return out, nil
}

type automationInterviews struct{ m *interviews.Module }

func (a automationInterviews) Interview(ctx context.Context, tx pgx.Tx, id string) (automations.Interview, bool, error) {
	b, err := a.m.Brief(ctx, tx, id)
	if e, ok := apperr.As(err); ok && e.Code == api.ErrorCodeNOTFOUND {
		return automations.Interview{}, false, nil
	}
	if err != nil {
		return automations.Interview{}, false, err
	}
	return automations.Interview{ApplicationID: b.ApplicationID, StartsAt: b.Details.StartsAt, Status: b.Details.Status}, true, nil
}

type automationPlacements struct{ m *placements.Module }

func (a automationPlacements) Placement(ctx context.Context, tx pgx.Tx, id string) (automations.Placement, bool, error) {
	b, ok, err := a.m.Brief(ctx, tx, id)
	return automations.Placement{ApplicationID: b.ApplicationID, Status: b.Status, StartDate: b.StartDate, GuaranteeEnds: b.GuaranteeEnds,
		DueOn: b.DueOn, Paid: b.Paid, RecruiterID: b.RecruiterID}, ok, err
}

type automationMailer struct{ m *emails.Module }

func (a automationMailer) SendAutomated(ctx context.Context, tx pgx.Tx, sender authz.Principal, e automations.Email) error {
	_, err := a.m.SendAutomated(ctx, tx, sender, emails.AutomatedEmail{
		CandidateID: e.CandidateID, ApplicationID: e.ApplicationID, InterviewID: e.InterviewID, To: e.To,
		Subject: e.Subject, Body: e.Body, TimeZone: e.TimeZone,
	})
	return err
}
