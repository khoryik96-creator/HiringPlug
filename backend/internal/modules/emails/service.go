package emails

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Service holds the email use cases.
type Service struct {
	pool   *pgxpool.Pool
	deps   Deps
	sender mail.Sender
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
		row, err := store.New(tx).InsertTemplate(ctx, store.InsertTemplateParams{Name: t.Name, Subject: t.Subject, Body: t.Body})
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
			ID: id, ExpectedVersion: int32(version), Name: t.Name, Subject: t.Subject, Body: t.Body, //nolint:gosec // versions are small
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

func nameTaken() error {
	var errs apperr.FieldErrors
	errs.Add("name", "another template already has this name")
	return errs.Err()
}

// DraftRequest is what to fill a template for.
type DraftRequest struct {
	TemplateID, CandidateID    string
	ApplicationID, InterviewID *string
	TimeZone                   string
}

// Draft is a filled template, to review before sending.
type Draft struct {
	To            []string
	Subject, Body string
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
		dc, err := s.context(ctx, tx, p, r.CandidateID, r.ApplicationID, r.InterviewID)
		if err != nil {
			return err
		}
		dc.Location = loc
		values := Values(dc)
		subject, missingSubject := Fill(tpl.Subject, values)
		body, missingBody := Fill(tpl.Body, values)
		out = Draft{To: []string{}, Subject: subject, Body: body, Missing: union(missingSubject, missingBody)}
		to := dc.Candidate.Email
		if ForClient(tpl.Name) {
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
	cands, err := s.deps.Candidates.Briefs(ctx, tx, []string{candidateID})
	if err != nil {
		return Context{}, err
	}
	cand, ok := cands[candidateID]
	if !ok {
		return Context{}, apperr.NotFound("candidate")
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

// Send records an email and queues it; the worker sends it.
func (s *Service) Send(ctx context.Context, p authz.Principal, o Outgoing, candidateID, appID *string) (store.Email, error) {
	o, errs := NormalizeOutgoing(o)
	if err := errs.Err(); err != nil {
		return store.Email{}, err
	}
	var out store.Email
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if appID != nil {
			app, err := s.application(ctx, tx, *appID, candidateID)
			if err != nil {
				return err
			}
			candidateID = &app.CandidateID
		}
		if candidateID != nil {
			found, err := s.deps.Candidates.Briefs(ctx, tx, []string{*candidateID})
			if err != nil {
				return err
			}
			if _, ok := found[*candidateID]; !ok {
				return apperr.NotFound("candidate")
			}
		}
		row, err := store.New(tx).InsertEmail(ctx, store.InsertEmailParams{
			CandidateID: candidateID, ApplicationID: appID, ToAddresses: o.To, CcAddresses: o.Cc,
			Subject: o.Subject, Body: o.Body, SenderName: p.Name, ReplyTo: p.Email,
		})
		if err != nil {
			return err
		}
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindEmailSend, api.EmailSendJob{EmailId: row.ID}); err != nil {
			return err
		}
		out = row
		// The text is in the email record; the audit log keeps who and what about.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "email.queued", EntityType: "email", EntityID: row.ID,
			Changes: map[string]any{"candidateId": candidateID, "to": o.To, "cc": o.Cc, "subject": o.Subject},
		})
	})
	return out, err
}

// List returns the emails about a candidate, newest first.
func (s *Service) List(ctx context.Context, p authz.Principal, candidateID string) ([]store.Email, error) {
	var out []store.Email
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListCandidateEmails(ctx, &candidateID)
		return err
	})
	return out, err
}
