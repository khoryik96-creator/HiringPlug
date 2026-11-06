package app

// What the scheduling module needs from candidates, pipeline, identity,
// interviews and emails.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type schedulingCandidates struct{ candidates *candidates.Module }

func (a schedulingCandidates) Person(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (scheduling.Person, error) {
	v, err := a.candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return scheduling.Person{}, err
	}
	return scheduling.Person{Name: strings.TrimSpace(v.Profile.FirstName + " " + v.Profile.LastName), Email: v.Profile.Email}, nil
}

type schedulingApplications struct{ pipeline *pipeline.Module }

func (a schedulingApplications) CandidateOf(ctx context.Context, tx pgx.Tx, applicationID string) (string, error) {
	b, err := a.pipeline.Briefs(ctx, tx, []string{applicationID})
	return b[applicationID].CandidateID, err
}

type schedulingUsers struct{ identity *identity.Module }

func (a schedulingUsers) Person(ctx context.Context, tx pgx.Tx, userID string) (*scheduling.Person, error) {
	c, err := a.identity.ActiveContacts(ctx, tx, []string{userID})
	if err != nil {
		return nil, err
	}
	u, ok := c[userID]
	if !ok {
		return nil, nil //nolint:nilnil // nil: not an active user
	}
	return &scheduling.Person{Name: u.Name, Email: &u.Email}, nil
}

func (a schedulingUsers) TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error) {
	return a.identity.TenantName(ctx, tx, tenantID)
}

type schedulingMailer struct{ emails *emails.Module }

func (a schedulingMailer) Send(ctx context.Context, tx pgx.Tx, tenantID string, m scheduling.Mail) error {
	return a.emails.Notify(ctx, tx, tenantID, emails.Notice{
		To: []string{m.To}, Subject: m.Subject, Body: m.Body, Calendar: m.Calendar, CandidateID: m.CandidateID, ApplicationID: m.ApplicationID,
	})
}
