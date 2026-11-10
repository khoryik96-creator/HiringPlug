package emails

// One email to several people, each their own copy: to them alone, its
// fields filled from them, kept on their emails. Those who cannot be sent to
// are listed with the reason and left out; the others go.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// maxBatch is how many people one email goes to separately.
const maxBatch = 50

// Recipient is a candidate or one of a client's contacts.
type Recipient struct {
	CandidateID, ContactID *string
}

// BatchRequest is one email to several people, each their own copy.
type BatchRequest struct {
	Recipients              []Recipient
	Subject, Body, BodyHTML string
	TimeZone                string
	AttachmentIDs           []string
	CheckOnly               bool // send nothing: say who would get it
}

// BatchResult says, in the order given, who got it (or would) and who did not.
type BatchResult struct {
	Sent       int
	Recipients []BatchRecipient
}

// BatchRecipient is one person: their address and email, or why not.
type BatchRecipient struct {
	Recipient
	Name    string
	To      *string
	EmailID *string
	Problem *string
}

func (r BatchRequest) check() apperr.FieldErrors {
	var errs apperr.FieldErrors
	if len(r.Recipients) == 0 || len(r.Recipients) > maxBatch {
		errs.Add("recipients", "between 1 and 50 people")
	}
	if len(r.AttachmentIDs) > maxFiles {
		errs.Add("attachmentIds", "at most 10 files")
	}
	seen := map[string]bool{}
	for _, x := range r.Recipients {
		if (x.CandidateID == nil) == (x.ContactID == nil) {
			errs.Add("recipients", "each is a candidate or a contact")
			break
		}
		key := deref(x.CandidateID) + deref(x.ContactID)
		if seen[key] {
			errs.Add("recipients", "lists someone twice")
			break
		}
		seen[key] = true
	}
	return errs
}

// SendSeparately sends (or, with CheckOnly, checks) one email to each person.
func (s *Service) SendSeparately(ctx context.Context, p authz.Principal, r BatchRequest) (BatchResult, error) {
	if err := r.check().Err(); err != nil {
		return BatchResult{}, err
	}
	// The message as everyone gets it, before their fields; a stand-in address only to tidy it.
	base, errs := NormalizeOutgoing(Outgoing{To: []string{"someone@example.com"}, Subject: r.Subject, Body: r.Body, BodyHTML: r.BodyHTML})
	if err := errs.Err(); err != nil {
		return BatchResult{}, err
	}
	loc, err := location(r.TimeZone)
	if err != nil {
		return BatchResult{}, err
	}
	var out BatchResult
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := s.checkBanners(ctx, q, base.BodyHTML); err != nil {
			return err
		}
		staged, err := q.StagedAttachments(ctx, r.AttachmentIDs)
		if err != nil {
			return err
		}
		if len(staged) != len(unique(r.AttachmentIDs)) {
			var errs apperr.FieldErrors
			errs.Add("attachmentIds", "a file is no longer there: upload it again")
			return errs.Err()
		}
		total := 0
		for _, a := range staged {
			total += int(a.SizeBytes)
		}
		if err := attachmentsFit(total); err != nil {
			return err
		}
		visibility := ""
		claimed := false
		for _, rec := range r.Recipients {
			one, values, err := s.recipient(ctx, tx, p, rec, loc)
			if err != nil {
				return err
			}
			var filled filledEmail
			if one.Problem == nil {
				filled, one.Problem = fillFor(base, *one.To, values)
			}
			if one.Problem != nil || r.CheckOnly {
				out.Recipients = append(out.Recipients, one)
				continue
			}
			restricted := false
			if filled.salary {
				if visibility == "" {
					if visibility, err = s.deps.Policy.EmailSalaryVisibility(ctx, tx); err != nil {
						return err
					}
				}
				restricted = visibility != "everyone"
			}
			row, err := q.InsertEmail(ctx, store.InsertEmailParams{
				CandidateID: rec.CandidateID, ContactID: rec.ContactID, ToAddresses: filled.To, CcAddresses: []string{}, BccAddresses: []string{},
				Subject: filled.Subject, Body: filled.Body, BodyHtml: orNil(filled.BodyHTML), Salary: filled.salary, SalaryRestricted: restricted,
				SenderName: p.Name, ReplyTo: p.Email,
			})
			if err != nil {
				return err
			}
			// The uploads go with the first email; the others carry the same files.
			if !claimed && len(staged) > 0 {
				n, err := q.AttachToEmail(ctx, store.AttachToEmailParams{EmailID: &row.ID, Ids: r.AttachmentIDs})
				if err != nil {
					return err
				}
				if int(n) != len(staged) {
					var errs apperr.FieldErrors
					errs.Add("attachmentIds", "a file was just sent with another email: upload it again")
					return errs.Err()
				}
				claimed = true
			} else {
				for _, a := range staged {
					if _, err := q.InsertAttachment(ctx, store.InsertAttachmentParams{
						EmailID: &row.ID, FileName: a.FileName, ContentType: a.ContentType, SizeBytes: a.SizeBytes, StorageKey: a.StorageKey,
					}); err != nil {
						return err
					}
				}
			}
			if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindEmailSend, api.EmailSendJob{EmailId: row.ID}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "email.queued", EntityType: "email", EntityID: row.ID,
				Changes: map[string]any{"candidateId": rec.CandidateID, "contactId": rec.ContactID, "to": filled.To,
					"subject": "(see the email)", "attachments": len(staged), "salary": filled.salary, "separately": len(r.Recipients)},
			}); err != nil {
				return err
			}
			id := row.ID
			one.EmailID = &id
			out.Sent++
			out.Recipients = append(out.Recipients, one)
		}
		return nil
	})
	return out, err
}

// recipient reads who someone is, their address, and the values for their
// fields; Problem says why they cannot be sent to.
func (s *Service) recipient(ctx context.Context, tx pgx.Tx, p authz.Principal, r Recipient, loc *time.Location) (BatchRecipient, map[string]string, error) {
	one := BatchRecipient{Recipient: r}
	var email *string
	if r.ContactID != nil {
		ct, err := s.deps.Contacts.Contact(ctx, tx, *r.ContactID)
		if err != nil {
			return one, nil, err
		}
		one.Name, email = ct.FullName, ct.Email
	} else {
		cand, err := s.deps.Candidates.Detailed(ctx, tx, p, *r.CandidateID)
		if err != nil {
			return one, nil, err
		}
		one.Name, email = cand.FullName, cand.Email
		if s.deps.Consent != nil {
			no, err := s.deps.Consent.DoNotContact(ctx, tx, *r.CandidateID)
			if err != nil {
				return one, nil, err
			}
			if no {
				one.Problem = problem("withdrew consent to be contacted")
				return one, nil, nil
			}
		}
	}
	if email == nil || strings.TrimSpace(*email) == "" {
		one.Problem = problem("has no email address")
		return one, nil, nil
	}
	to := strings.ToLower(strings.TrimSpace(*email))
	one.To = &to
	values, err := s.values(ctx, tx, p, r.CandidateID, nil, nil, r.ContactID, loc)
	return one, values, err
}

// fillFor fills the message for one person, or says why it cannot be sent to them.
func fillFor(base Outgoing, to string, values map[string]string) (filledEmail, *string) {
	o := base
	o.To = []string{to}
	if _, errs := NormalizeOutgoing(o); len(errs) > 0 {
		return filledEmail{}, problem("the address " + to + " is not valid")
	}
	sub := FillFields(o.Subject, values, false)
	text := o.Body
	if o.BodyHTML != "" {
		text = o.BodyHTML
	}
	body := FillFields(text, values, o.BodyHTML != "")
	if missing := union(sub.Missing, body.Missing); len(missing) > 0 {
		return filledEmail{}, problem("no value for " + strings.Join(missing, ", "))
	}
	filled, err := fill(o, values)
	if err != nil {
		return filledEmail{}, problem("the message is too long once filled in")
	}
	return filled, nil
}

func problem(s string) *string { return &s }
