package emails

// Emails an automation sends (automations module): written from the
// automation's subject and message, filled for the candidate, the job and the
// interview, and sent from the sender's own mailbox as if they wrote it.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Who an automated email goes to.
const (
	ToCandidate = "candidate" // the candidate
	ToClient    = "client"    // the job's hiring contact
	ToSender    = "sender"    // the sender (a reminder to themselves)
)

// AutomatedEmail is an email an automation sends.
type AutomatedEmail struct {
	CandidateID, ApplicationID string
	InterviewID                *string
	To                         string
	Subject, Body              string
	TimeZone                   *time.Location // for interview times
}

// SendAutomated records an automated email and queues it, from sender's
// mailbox, in the caller's transaction (sender's company). Refused, with the
// reason, when it cannot go: no address, a candidate who asked not to be
// contacted, a field with no value, or a salary field (automations never
// send salaries).
func (m *Module) SendAutomated(ctx context.Context, tx pgx.Tx, sender authz.Principal, e AutomatedEmail) (string, error) {
	return m.service.sendAutomated(ctx, tx, sender, e)
}

func (s *Service) sendAutomated(ctx context.Context, tx pgx.Tx, p authz.Principal, e AutomatedEmail) (string, error) {
	loc := e.TimeZone
	if loc == nil {
		loc = time.UTC
	}
	app, err := s.application(ctx, tx, e.ApplicationID, &e.CandidateID)
	if err != nil {
		return "", err
	}
	var to *string
	switch e.To {
	case ToCandidate:
		if err := s.contactable(ctx, tx, e.CandidateID); err != nil {
			return "", err
		}
		cands, err := s.deps.Candidates.Briefs(ctx, tx, []string{e.CandidateID})
		if err != nil {
			return "", err
		}
		to = cands[e.CandidateID].Email
	case ToClient:
		to = app.ClientEmail
	case ToSender:
		to = &p.Email
	}
	if to == nil || *to == "" {
		return "", apperr.New(api.ErrorCodeVALIDATIONFAILED, "There is no email address to send it to.")
	}
	if s.deps.Undeliverable != nil {
		bad, err := s.deps.Undeliverable.Undeliverable(ctx, tx, []string{*to})
		if err != nil {
			return "", err
		}
		for _, why := range bad {
			return "", apperr.New(api.ErrorCodeEMAILUNDELIVERABLE, why)
		}
	}
	values, err := s.values(ctx, tx, p, &e.CandidateID, &e.ApplicationID, e.InterviewID, nil, loc)
	if err != nil {
		return "", err
	}
	o, errs := NormalizeOutgoing(Outgoing{To: []string{*to}, Subject: e.Subject, Body: e.Body})
	if err := errs.Err(); err != nil {
		return "", err
	}
	filled, err := fill(o, values)
	if err != nil {
		return "", err
	}
	if filled.salary {
		return "", apperr.New(api.ErrorCodeVALIDATIONFAILED, "Automated emails do not show salaries: take the salary field out.")
	}
	q := store.New(tx)
	row, err := q.InsertEmail(ctx, store.InsertEmailParams{
		CandidateID: &e.CandidateID, ApplicationID: &e.ApplicationID, ToAddresses: filled.To, CcAddresses: []string{}, BccAddresses: []string{},
		Subject: filled.Subject, Body: filled.Body, SenderName: p.Name, ReplyTo: p.Email,
	})
	if err != nil {
		return "", err
	}
	if _, err := s.bindDelivery(ctx, tx, row.ID, p.UserID); err != nil {
		return "", err
	}
	if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindEmailSend, api.EmailSendJob{EmailId: row.ID}); err != nil {
		return "", err
	}
	return row.ID, audit.Record(ctx, tx, audit.Entry{
		Action: "email.queued", EntityType: "email", EntityID: row.ID,
		Changes: map[string]any{"candidateId": e.CandidateID, "to": filled.To, "subject": "(see the email)", "automated": true},
	})
}
