package emails

// Filling a message's fields to see it before it is sent, and the values
// the composer puts in when a field is inserted.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Preview is a message with its fields filled, as it will be sent.
type Preview struct {
	CandidateVersion, CompensationVersion *int
	Subject, BodyHTML                     string
	Missing                               []string
	Salary                                bool
	ClientEmail                           *string // the job's hiring contact, for an application
	// FieldValues: each field with a value, by token ("[Expected salary]"),
	// for the composer to put in the message, where they can be edited;
	// salary fields only when asked for. WithheldFields: the salary fields
	// with a value that were not asked for.
	FieldValues    map[string]string
	WithheldFields []string
}

// PreviewRequest is a message to fill for a candidate (and an application or
// interview), a contact, or several candidates (to a contact or a candidate).
type PreviewRequest struct {
	Subject, BodyHTML                                                        string
	CandidateID, ApplicationID, InterviewID, ContactID, RecipientCandidateID *string
	Candidates                                                               []Shortlisted
	TimeZone                                                                 string
	// IncludeSalary: put the salary fields' values in FieldValues (they are
	// counted as shown); without it they are left out (WithheldFields).
	IncludeSalary bool
}

// Preview fills a message's fields without sending anything.
func (s *Service) Preview(ctx context.Context, p authz.Principal, r PreviewRequest) (Preview, error) {
	subject, bodyHTML, list, recipientCandidateID := r.Subject, r.BodyHTML, r.Candidates, r.RecipientCandidateID
	candidateID, appID, interviewID, contactID := r.CandidateID, r.ApplicationID, r.InterviewID, r.ContactID
	loc, err := location(r.TimeZone)
	if err != nil {
		return Preview{}, err
	}
	var out Preview
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var values map[string]string
		var candidateVersion, compensationVersion *int
		if recipientCandidateID != nil && len(list) == 0 {
			return invalidRecipient()
		}
		if contactID != nil && recipientCandidateID != nil {
			return invalidRecipient()
		}
		if len(list) > 0 {
			if candidateID != nil || appID != nil || interviewID != nil {
				return notBoth()
			}
			first, err := s.checkShortlist(ctx, tx, p, list)
			if err != nil {
				return err
			}
			candidateID, appID = &first.CandidateID, first.ApplicationID
			values, err = s.shortlistValues(ctx, tx, p, first, loc)
			if err != nil {
				return err
			}
		} else if candidateID != nil && contactID == nil {
			dc, err := s.context(ctx, tx, p, *candidateID, appID, interviewID)
			if err != nil {
				return err
			}
			dc.Location = loc
			values = Values(dc)
			v := dc.Candidate.Version
			if v > 0 {
				candidateVersion = &v
			}
			if p.Can(api.PermissionCandidatesCompensationRead) {
				v := dc.Candidate.CompensationVersion
				compensationVersion = &v
			}
		} else {
			var err error
			if values, err = s.values(ctx, tx, p, candidateID, appID, interviewID, contactID, loc); err != nil {
				return err
			}
		}
		if len(list) > 0 {
			if err := s.recipientValues(ctx, tx, p, values, contactID, recipientCandidateID, loc); err != nil {
				return err
			}
		}
		sub := FillFields(strings.Join(strings.Fields(subject), " "), values, false)
		clean := SanitizeHTML(bodyHTML)
		body := FillFields(clean, values, true)
		fields, withheld := fieldValues(values, r.IncludeSalary)
		out = Preview{
			Subject: sub.Text, BodyHTML: WithoutFieldMarks(body.Text), Missing: union(sub.Missing, body.Missing),
			Salary: sub.Salary || body.Salary || ShowsSalaryField(clean), FieldValues: fields, WithheldFields: withheld,
			CandidateVersion: candidateVersion, CompensationVersion: compensationVersion,
		}
		// Said at preview, not only on send: another candidate's salary never goes to a candidate.
		if err := salaryOnlyTheirOwn(recipientCandidateID, bodyHTML, out.Salary); err != nil {
			return err
		}
		// Counted when a salary comes back: asked for among the values, or
		// filled into the subject or message here. The message's own fields
		// are the recipient's when the email goes to a candidate.
		shown := tokensShowSalary(out.FieldValues) || sub.Salary || body.Salary
		if err := s.salaryFieldsShown(ctx, tx, shown, whoseFields(candidateID, recipientCandidateID)); err != nil {
			return err
		}
		if appID != nil {
			app, err := s.application(ctx, tx, *appID, nil)
			if err != nil {
				return err
			}
			out.ClientEmail = app.ClientEmail
		}
		return nil
	})
	return out, err
}
