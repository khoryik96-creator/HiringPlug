package emails

// Sending an email from the composer: its fields filled from the candidate
// and application, files attached (uploads, or the candidate's CVs), and
// whether it shows salary (then hidden from roles that may not see salaries,
// unless the company chose otherwise).

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// Attachment limits: per email, as most mail servers take them.
const (
	maxFiles     = 10
	maxFileBytes = 10 << 20
	maxAllBytes  = 20 << 20
)

const defaultZone = "Asia/Kuala_Lumpur"

// CVRef is one of the candidate's CVs to attach.
type CVRef struct {
	DocumentID string
	AsPDF      bool
}

// SendRequest is an email from the composer.
type SendRequest struct {
	Outgoing
	CandidateID, ApplicationID *string
	RecipientCandidateID       *string
	ContactID                  *string // to one of a client's contacts (their page)
	TimeZone                   string
	AttachmentIDs              []string
	CVs                        []CVRef
	Candidates                 []Shortlisted // to the client about several candidates, instead of CandidateID
}

// Preview is a message with its fields filled, as it will be sent.
type Preview struct {
	CandidateVersion, CompensationVersion *int
	Subject, BodyHTML                     string
	Missing                               []string
	Salary                                bool
	ClientEmail                           *string // the job's hiring contact, for an application
	// FieldValues: each field with a value, by token ("[Expected salary]"),
	// for the composer to put in the message, where they can be edited.
	FieldValues map[string]string
}

// fieldValues are the values by field token, for the composer.
func fieldValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range Fields {
		if v, ok := values[f.Key]; ok && v != "" {
			out[f.Token()] = v
		}
	}
	return out
}

// Preview fills a message's fields for a candidate (and an application or
// interview) without sending anything.
func (s *Service) Preview(ctx context.Context, p authz.Principal, subject, bodyHTML string, candidateID, appID, interviewID, contactID *string, list []Shortlisted, zone string, recipientCandidateID *string) (Preview, error) {
	loc, err := location(zone)
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
		out = Preview{
			Subject: sub.Text, BodyHTML: WithoutFieldMarks(body.Text), Missing: union(sub.Missing, body.Missing),
			Salary: sub.Salary || body.Salary || ShowsSalaryField(clean), FieldValues: fieldValues(values), CandidateVersion: candidateVersion, CompensationVersion: compensationVersion,
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

// values are the field values for an email: the candidate's (as p may see
// them), the application's and the interview's, the sender's and the agency's;
// or, for an email to a client's contact, the contact's and their company's.
func (s *Service) values(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID, appID, interviewID, contactID *string, loc *time.Location) (map[string]string, error) {
	if candidateID == nil && appID == nil && contactID != nil {
		ct, err := s.deps.Contacts.Contact(ctx, tx, *contactID)
		if err != nil {
			return nil, err
		}
		tenant, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
		return ContactValues(ct, p.Name, tenant), err
	}
	if candidateID == nil && appID != nil {
		app, err := s.application(ctx, tx, *appID, nil)
		if err != nil {
			return nil, err
		}
		candidateID = &app.CandidateID
	}
	if candidateID == nil {
		tenant, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
		return map[string]string{"sender.name": p.Name, "agency.name": tenant}, err
	}
	dc, err := s.context(ctx, tx, p, *candidateID, appID, interviewID)
	if err != nil {
		return nil, err
	}
	dc.Location = loc
	return Values(dc), nil
}

func location(zone string) (*time.Location, error) {
	if zone == "" {
		zone = defaultZone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "Local" {
		var errs apperr.FieldErrors
		errs.Add("timeZone", "is not a time zone, e.g. Asia/Kuala_Lumpur")
		return nil, errs.Err()
	}
	return loc, nil
}

// Send records an email and queues it; the worker sends it.
func (s *Service) Send(ctx context.Context, p authz.Principal, r SendRequest) (EmailView, error) {
	o, errs := NormalizeOutgoing(r.Outgoing)
	if len(r.AttachmentIDs)+len(r.CVs) > maxFiles {
		errs.Add("attachments", "at most 10 files")
	}
	if err := errs.Err(); err != nil {
		return EmailView{}, err
	}
	if len(r.CVs) > 0 && !p.Can(api.PermissionDocumentsRead) {
		return EmailView{}, apperr.New(api.ErrorCodeFORBIDDEN, "Attaching CVs needs permission to see documents.")
	}
	loc, err := location(r.TimeZone)
	if err != nil {
		return EmailView{}, err
	}
	var out EmailView
	var stored []string // files written for this email, removed if it is not saved
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		candidateID, appID := r.CandidateID, r.ApplicationID
		if r.ContactID != nil && r.RecipientCandidateID != nil {
			return invalidRecipient()
		}
		if r.RecipientCandidateID != nil && len(r.Candidates) == 0 {
			return invalidRecipient()
		}
		var first Shortlisted // the first of several candidates
		if len(r.Candidates) > 0 {
			if candidateID != nil || appID != nil {
				return notBoth()
			}
			var err error
			if first, err = s.checkShortlist(ctx, tx, p, r.Candidates); err != nil {
				return err
			}
			candidateID, appID = &first.CandidateID, first.ApplicationID
		}
		if appID != nil && len(r.Candidates) == 0 {
			app, err := s.application(ctx, tx, *appID, candidateID)
			if err != nil {
				return err
			}
			candidateID = &app.CandidateID
		}
		if candidateID != nil && len(r.Candidates) == 0 {
			if err := s.contactable(ctx, tx, *candidateID); err != nil {
				return err
			}
		}
		if r.ContactID != nil && len(r.Candidates) == 0 && (candidateID != nil || appID != nil) {
			var errs apperr.FieldErrors
			errs.Add("contactId", "an email is to a contact or about a candidate, not both")
			return errs.Err()
		}
		var values map[string]string
		var err error
		if len(r.Candidates) > 0 {
			values, err = s.shortlistValues(ctx, tx, p, first, loc)
		} else {
			values, err = s.values(ctx, tx, p, candidateID, appID, nil, r.ContactID, loc)
		}
		if err != nil {
			return err
		}
		if len(r.Candidates) > 0 {
			if err := s.recipientValues(ctx, tx, p, values, r.ContactID, r.RecipientCandidateID, loc); err != nil {
				return err
			}
		}
		if r.RecipientCandidateID != nil {
			if err := s.contactable(ctx, tx, *r.RecipientCandidateID); err != nil {
				return err
			}
			candidateID = r.RecipientCandidateID
			appID = nil
		}
		filled, err := fill(o, values)
		if err != nil {
			return err
		}
		if err := salaryOnlyTheirOwn(r.RecipientCandidateID, r.Candidates, filled.salary); err != nil {
			return err
		}
		q := store.New(tx)
		if err := s.checkBanners(ctx, q, filled.BodyHTML); err != nil {
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
		cvs := make([]CVFile, 0, len(r.CVs))
		for _, ref := range r.CVs {
			cv, err := s.deps.Documents.CVFile(ctx, tx, ref.DocumentID, ref.AsPDF)
			if err != nil {
				return err
			}
			if !cvAllowed(cv.CandidateID, candidateID, r.Candidates) {
				var errs apperr.FieldErrors
				errs.Add("cvs", "is not this candidate's CV")
				return errs.Err()
			}
			total += len(cv.Data)
			cvs = append(cvs, cv)
		}
		if err := attachmentsFit(total); err != nil {
			return err
		}
		restricted := false
		if filled.salary {
			visibility, err := s.deps.Policy.EmailSalaryVisibility(ctx, tx)
			if err != nil {
				return err
			}
			restricted = visibility != "everyone"
		}
		row, err := q.InsertEmail(ctx, store.InsertEmailParams{
			CandidateID: candidateID, ApplicationID: appID, ContactID: r.ContactID, ToAddresses: filled.To, CcAddresses: filled.Cc, BccAddresses: filled.Bcc,
			Subject: filled.Subject, Body: filled.Body, BodyHtml: orNil(filled.BodyHTML), Salary: filled.salary, SalaryRestricted: restricted,
			SenderName: p.Name, ReplyTo: p.Email,
		})
		if err != nil {
			return err
		}
		mode, err := s.bindDelivery(ctx, tx, row.ID, p.UserID)
		if err != nil {
			return err
		}
		row.DeliveryMode = mode
		// Claimed only if still unsent: another email sent at the same moment
		// with the same upload refuses this one rather than going without it.
		claimed, err := q.AttachToEmail(ctx, store.AttachToEmailParams{EmailID: &row.ID, Ids: r.AttachmentIDs})
		if err != nil {
			return err
		}
		if int(claimed) != len(staged) {
			var errs apperr.FieldErrors
			errs.Add("attachmentIds", "a file was just sent with another email: upload it again")
			return errs.Err()
		}
		for _, cv := range cvs {
			key := p.TenantID + "/eat_" + newKey()
			if err := s.files.Put(key, cv.Data); err != nil {
				return err
			}
			stored = append(stored, key)
			if _, err := q.InsertAttachment(ctx, store.InsertAttachmentParams{
				EmailID: &row.ID, FileName: clip(cv.FileName, 255), ContentType: cv.ContentType, SizeBytes: int32(len(cv.Data)), StorageKey: key, //nolint:gosec // at most 20 MB
			}); err != nil {
				return err
			}
		}
		if err := s.keepShortlist(ctx, tx, row.ID, r.Candidates, filled.To); err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindEmailSend, api.EmailSendJob{EmailId: row.ID}); err != nil {
			return err
		}
		files, err := q.EmailAttachments(ctx, []string{row.ID})
		if err != nil {
			return err
		}
		out = EmailView{Email: row, Attachments: files}
		// The text stays in the email record; never the salary in the audit log.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "email.queued", EntityType: "email", EntityID: row.ID,
			Changes: map[string]any{"candidateId": candidateID, "candidates": len(r.Candidates), "to": filled.To, "cc": filled.Cc, "bcc": len(filled.Bcc),
				"subject": "(see the email)", "attachments": len(files), "salary": filled.salary},
		})
	})
	if err != nil {
		for _, key := range stored {
			_ = s.files.Delete(key)
		}
	}
	return out, err
}

// contactable refuses candidates who asked not to be contacted.
func (s *Service) contactable(ctx context.Context, tx pgx.Tx, candidateID string) error {
	if s.deps.Consent == nil {
		return nil
	}
	no, err := s.deps.Consent.DoNotContact(ctx, tx, candidateID)
	if err != nil {
		return err
	}
	if no {
		return apperr.New(api.ErrorCodeEMAILDONOTCONTACT,
			"This candidate withdrew consent to be contacted, so no email is sent to them. See Data protection on their page.")
	}
	return nil
}

type filledEmail struct {
	Outgoing
	salary bool
}

// fill fills the subject and message; a field with no value refuses the email.
func fill(o Outgoing, values map[string]string) (filledEmail, error) {
	sub := FillFields(o.Subject, values, false)
	out := filledEmail{Outgoing: o}
	out.Subject = sub.Text
	missing, salary := sub.Missing, sub.Salary
	if o.BodyHTML != "" {
		// A value the composer filled in (and may have been edited) shows
		// salary like a [salary] field does; its mark is not sent.
		body := FillFields(o.BodyHTML, values, true)
		out.BodyHTML, out.Body = WithoutFieldMarks(body.Text), TextFromHTML(body.Text)
		missing, salary = union(missing, body.Missing), salary || body.Salary || ShowsSalaryField(o.BodyHTML)
	} else {
		body := FillFields(o.Body, values, false)
		out.Body = body.Text
		missing, salary = union(missing, body.Missing), salary || body.Salary
	}
	if len(missing) > 0 {
		var errs apperr.FieldErrors
		errs.Add("body", "no value for this candidate: "+strings.Join(missing, ", ")+" (fill it in, or take it out)")
		return out, errs.Err()
	}
	if out.Body == "" {
		out.Body = "(See the formatted message.)"
	}
	if len([]rune(out.Body)) > maxBody {
		var errs apperr.FieldErrors
		errs.Add("body", "is too long (at most 20,000 characters)")
		return out, errs.Err()
	}
	out.salary = salary
	return out, nil
}

// checkBanners refuses banners that are not the company's.
func (s *Service) checkBanners(ctx context.Context, q *store.Queries, html string) error {
	ids := BannerIDs(html)
	if len(ids) == 0 {
		return nil
	}
	found, err := q.GetBanners(ctx, ids)
	if err != nil {
		return err
	}
	live := 0
	for _, b := range found {
		if b.DeletedAt == nil {
			live++
		}
	}
	if live != len(ids) {
		var errs apperr.FieldErrors
		errs.Add("bodyHtml", "a banner in it was deleted: insert another one")
		return errs.Err()
	}
	return nil
}

// attachmentsFit refuses attachments of more than 20 MB in all.
func attachmentsFit(total int) error {
	if total > maxAllBytes {
		return apperr.New(api.ErrorCodePAYLOADTOOLARGE, "The attachments come to more than 20 MB; leave some out.")
	}
	return nil
}

func unique(xs []string) []string {
	return union(nil, xs)
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func newKey() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
