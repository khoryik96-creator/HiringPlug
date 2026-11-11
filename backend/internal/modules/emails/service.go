package emails

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Service holds the email use cases.
type Service struct {
	connections ConnectionSettings
	pool        *pgxpool.Pool
	deps        Deps
	sender      mail.Sender
	files       *blob.Store // banners and attachments
}

const templateNameIndex = "email_template_name_idx"

// Templates returns the company's templates, adding the starters the first time.
func (s *Service) Templates(ctx context.Context, p authz.Principal) ([]store.EmailTemplate, error) {
	var out []store.EmailTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListTemplates(ctx)
		if err != nil || len(rows) > 0 {
			out = rows
			return err
		}
		for _, t := range Starters {
			if err := q.SeedTemplate(ctx, store.SeedTemplateParams{Name: t.Name, Subject: t.Subject, Body: t.Body}); err != nil {
				return err
			}
		}
		out, err = q.ListTemplates(ctx)
		return err
	})
	return out, err
}

// CreateTemplate adds a template.
func (s *Service) CreateTemplate(ctx context.Context, p authz.Principal, t Template) (store.EmailTemplate, error) {
	t, errs := NormalizeTemplate(t)
	if err := errs.Err(); err != nil {
		return store.EmailTemplate{}, err
	}
	var out store.EmailTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).InsertTemplate(ctx, store.InsertTemplateParams{Name: t.Name, Subject: t.Subject, Body: t.Body, BodyHtml: orNil(t.BodyHTML)})
		if database.IsUniqueViolation(err, templateNameIndex) {
			return nameTaken()
		}
		if err != nil {
			return err
		}
		out = row
		return audit.Record(ctx, tx, audit.Entry{
			Action: "email_template.created", EntityType: "email_template", EntityID: row.ID,
			Changes: map[string]any{"name": t.Name},
		})
	})
	return out, err
}

// UpdateTemplate changes a template, if nobody changed it since version.
func (s *Service) UpdateTemplate(ctx context.Context, p authz.Principal, id string, version int, t Template) (store.EmailTemplate, error) {
	t, errs := NormalizeTemplate(t)
	if err := errs.Err(); err != nil {
		return store.EmailTemplate{}, err
	}
	var out store.EmailTemplate
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		old, err := q.GetTemplate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("email template")
		}
		if err != nil {
			return err
		}
		row, err := q.UpdateTemplate(ctx, store.UpdateTemplateParams{
			ID: id, ExpectedVersion: int32(version), Name: t.Name, Subject: t.Subject, Body: t.Body, BodyHtml: orNil(t.BodyHTML), //nolint:gosec // versions are small
		})
		switch {
		case database.IsNoRows(err):
			return apperr.StaleVersion("email template")
		case database.IsUniqueViolation(err, templateNameIndex):
			return nameTaken()
		case err != nil:
			return err
		}
		out = row
		changes := map[string]any{}
		if old.Name != row.Name {
			changes["name"] = map[string]string{"from": old.Name, "to": row.Name}
		}
		if old.Subject != row.Subject || old.Body != row.Body {
			changes["text"] = map[string]any{"changed": true}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "email_template.updated", EntityType: "email_template", EntityID: id, Changes: changes})
	})
	return out, err
}

// DeleteTemplate removes a template.
func (s *Service) DeleteTemplate(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		old, err := q.GetTemplate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("email template")
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteTemplate(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "email_template.deleted", EntityType: "email_template", EntityID: id, Changes: map[string]any{"name": old.Name}})
	})
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nameTaken() error {
	var errs apperr.FieldErrors
	errs.Add("name", "another template already has this name")
	return errs.Err()
}

// DraftRequest is what to fill a template for.
type DraftRequest struct {
	TemplateID                 string
	CandidateID, ContactID     *string // one of them: a candidate, or a client's contact
	ApplicationID, InterviewID *string
	TimeZone                   string
}

// Draft is a filled template, to review before sending.
type Draft struct {
	To            []string
	Subject, Body string
	BodyHTML      *string // the formatted message, when the template has one
	Missing       []string
}

// Draft fills a template for a candidate (and an application or interview).
func (s *Service) Draft(ctx context.Context, p authz.Principal, r DraftRequest) (Draft, error) {
	loc, err := time.LoadLocation(r.TimeZone)
	if err != nil || r.TimeZone == "Local" {
		var errs apperr.FieldErrors
		errs.Add("timeZone", "is not a time zone, e.g. Asia/Kuala_Lumpur")
		return Draft{}, errs.Err()
	}
	var out Draft
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		tpl, err := store.New(tx).GetTemplate(ctx, r.TemplateID)
		if database.IsNoRows(err) {
			return apperr.NotFound("email template")
		}
		if err != nil {
			return err
		}
		var values map[string]string
		var dc Context
		var contactEmail *string
		switch {
		case r.CandidateID != nil:
			if dc, err = s.context(ctx, tx, p, *r.CandidateID, r.ApplicationID, r.InterviewID); err != nil {
				return err
			}
			dc.Location = loc
			values = Values(dc)
		case r.ContactID != nil:
			ct, err := s.deps.Contacts.Contact(ctx, tx, *r.ContactID)
			if err != nil {
				return err
			}
			tenant, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
			if err != nil {
				return err
			}
			values, contactEmail = ContactValues(ct, p.Name, tenant), ct.Email
		default:
			var errs apperr.FieldErrors
			errs.Add("candidateId", "give candidateId or contactId")
			return errs.Err()
		}
		subject, missingSubject := Fill(tpl.Subject, values)
		body, missingBody := Fill(tpl.Body, values)
		out = Draft{To: []string{}, Subject: subject, Body: body, Missing: union(missingSubject, missingBody)}
		if tpl.BodyHtml != nil && *tpl.BodyHtml != "" {
			filled, missingHTML := FillHTML(*tpl.BodyHtml, values)
			out.BodyHTML, out.Missing = &filled, union(out.Missing, missingHTML)
		}
		to := dc.Candidate.Email
		if r.ContactID != nil {
			to = contactEmail
		} else if ForClient(tpl.Name) {
			to = nil
			if dc.Application != nil {
				to = dc.Application.ClientEmail
			}
		}
		if to != nil && *to != "" {
			out.To = append(out.To, *to)
		}
		return nil
	})
	return out, err
}

// context loads what a draft is about, checking the application and the
// interview belong to the candidate.
func (s *Service) context(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID string, appID, interviewID *string) (Context, error) {
	tenant, err := s.deps.Tenants.TenantName(ctx, tx, p.TenantID)
	if err != nil {
		return Context{}, err
	}
	c := Context{SenderName: p.Name, AgencyName: tenant}
	cand, err := s.deps.Candidates.Detailed(ctx, tx, p, candidateID)
	if err != nil {
		return Context{}, err
	}
	c.Candidate = cand
	var errs apperr.FieldErrors
	if interviewID != nil {
		iv, err := s.deps.Interviews.Brief(ctx, tx, *interviewID)
		if err != nil {
			return Context{}, err
		}
		if appID == nil {
			appID = &iv.ApplicationID
		} else if *appID != iv.ApplicationID {
			errs.Add("interviewId", "is for another application")
		}
		c.Interview = &iv
	}
	if appID != nil {
		app, err := s.application(ctx, tx, *appID, &candidateID)
		if err != nil {
			return Context{}, err
		}
		c.Application = &app
	}
	return c, errs.Err()
}

// application loads an application and checks it is the candidate's (when
// a candidate is given).
func (s *Service) application(ctx context.Context, tx pgx.Tx, id string, candidateID *string) (Application, error) {
	apps, err := s.deps.Applications.Briefs(ctx, tx, []string{id})
	if err != nil {
		return Application{}, err
	}
	app, ok := apps[id]
	if !ok {
		return Application{}, apperr.NotFound("application")
	}
	if candidateID != nil && app.CandidateID != *candidateID {
		var errs apperr.FieldErrors
		errs.Add("applicationId", "is not this candidate's application")
		return Application{}, errs.Err()
	}
	return app, nil
}

func union(a, b []string) []string {
	out := append([]string{}, a...)
	for _, x := range b {
		found := false
		for _, y := range out {
			found = found || x == y
		}
		if !found {
			out = append(out, x)
		}
	}
	return out
}

// EmailView is a sent email with its attachments.
type EmailView struct {
	store.Email
	Attachments []store.EmailAttachment
}

// List returns the emails about a candidate the user may see, newest first.
func (s *Service) List(ctx context.Context, p authz.Principal, candidateID, contactID *string) ([]EmailView, error) {
	if (candidateID == nil) == (contactID == nil) {
		var errs apperr.FieldErrors
		errs.Add("candidateId", "give candidateId or contactId")
		return nil, errs.Err()
	}
	var out []EmailView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var rows []store.Email
		var err error
		if candidateID != nil {
			rows, err = q.ListCandidateEmails(ctx, candidateID)
		} else {
			rows, err = q.ListContactEmails(ctx, contactID)
		}
		if err != nil {
			return err
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		files, err := q.EmailAttachments(ctx, ids)
		if err != nil {
			return err
		}
		out = make([]EmailView, len(rows))
		for i, r := range rows {
			out[i] = EmailView{Email: r}
			for _, f := range files {
				if f.EmailID != nil && *f.EmailID == r.ID {
					out[i].Attachments = append(out[i].Attachments, f)
				}
			}
		}
		return nil
	})
	return out, err
}

// notify records a notice from the app itself and queues it.
func (s *Service) notify(ctx context.Context, tx pgx.Tx, tenantID string, n Notice) error {
	o, errs := NormalizeOutgoing(Outgoing{To: n.To, Subject: n.Subject, Body: n.Body})
	if err := errs.Err(); err != nil {
		return err
	}
	sender := n.SenderName
	if sender == "" {
		sender = "HiringPlug"
	}
	q := store.New(tx)
	row, err := q.InsertEmail(ctx, store.InsertEmailParams{
		ApplicationID: n.ApplicationID, CandidateID: n.CandidateID, ToAddresses: o.To, CcAddresses: []string{}, BccAddresses: []string{},
		Subject: o.Subject, Body: o.Body, SenderName: sender, ReplyTo: "", FeesRestricted: n.ShowsFees,
	})
	if err != nil {
		return err
	}
	if _, err := s.bindDelivery(ctx, tx, row.ID, row.SentBy); err != nil {
		return err
	}
	for _, f := range n.Files {
		key := tenantID + "/eat_" + newKey()
		if f.Key != "" {
			key = tenantID + "/eat_" + storageName(f.Key)
		}
		if err := s.files.Put(key, f.Data); err != nil {
			return err
		}
		if _, err := q.InsertAttachment(ctx, store.InsertAttachmentParams{
			EmailID: &row.ID, FileName: clip(f.Name, 255), ContentType: clip(f.ContentType, 255),
			SizeBytes: int32(len(f.Data)), StorageKey: key, //nolint:gosec // a generated invoice is small
		}); err != nil {
			_ = s.files.Delete(key)
			return err
		}
	}
	if n.Calendar != "" {
		if err := store.New(tx).SetEmailCalendar(ctx, store.SetEmailCalendarParams{ID: row.ID, Calendar: &n.Calendar}); err != nil {
			return err
		}
	}
	if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindEmailSend, api.EmailSendJob{EmailId: row.ID}); err != nil {
		return err
	}
	changes := map[string]any{"to": o.To, "subject": o.Subject, "automatic": true}
	if n.ShowsFees {
		changes["subject"] = audit.Redacted // it may show the amount
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "email.queued", EntityType: "email", EntityID: row.ID, Changes: changes})
}

// storageName is a name as storage keys take it: lower case letters, digits
// and _ ("invoice_plc_01…_HYP-0042" -> "invoice_plc_01…_hyp_0042").
func storageName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '_'
	}, name)
}
