package assistant

// Emails the AI writes for someone: from a short ask, a better version of
// their draft, or a reply to what they were sent. The browser sends names,
// contact details, amounts, links and fields as placeholders and puts them
// back in what comes out; salary lines are taken out here as well, so no
// pay reaches the AI even if the browser missed some.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// EmailAsk is what someone asks the AI to do with an email.
type EmailAsk struct {
	Mode, Recipient                               string
	Style, Instruction, Subject, Draft, TheirText *string
	ApplicationID                                 *string
}

// checkEmailAsk says what is missing for the mode.
func checkEmailAsk(a EmailAsk) error {
	filled := func(s *string) bool { return s != nil && trimmed(*s) != "" }
	var errs apperr.FieldErrors
	switch a.Mode {
	case "write":
		if !filled(a.Instruction) {
			errs = append(errs, apperr.FieldError{Field: "instruction", Message: "say what the email should say"})
		}
	case "improve":
		if !filled(a.Draft) {
			errs = append(errs, apperr.FieldError{Field: "draft", Message: "write a draft first"})
		}
		if a.Style == nil {
			errs = append(errs, apperr.FieldError{Field: "style", Message: "choose how to improve it"})
		}
	case "reply":
		if !filled(a.TheirText) {
			errs = append(errs, apperr.FieldError{Field: "theirMessage", Message: "paste the email they sent"})
		}
	}
	if len(errs) > 0 {
		return apperr.Validation(errs)
	}
	return nil
}

func trimmed(s string) string { return strings.TrimSpace(s) }

// noPay is a text with salary lines taken out (nil stays nil).
func noPay(s *string) *string {
	if s == nil || trimmed(*s) == "" {
		return nil
	}
	v := redact.WithoutSalary(*s)
	return &v
}

// DraftEmail queues an email for the AI to write.
func (s *Service) DraftEmail(ctx context.Context, p authz.Principal, a EmailAsk) (store.AssistantEmail, error) {
	if err := checkEmailAsk(a); err != nil {
		return store.AssistantEmail{}, err
	}
	var out store.AssistantEmail
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		ai, err := s.ready(ctx, tx)
		if err != nil {
			return err
		}
		job := api.AssistantEmailJob{
			Mode: api.AssistantEmailMode(a.Mode), Recipient: api.AssistantEmailRecipient(a.Recipient), Ai: ai,
			Instruction: apiconv.OrNull(noPay(a.Instruction)), Subject: apiconv.OrNull(noPay(a.Subject)),
			Draft: apiconv.OrNull(noPay(a.Draft)), TheirMessage: apiconv.OrNull(noPay(a.TheirText)),
		}
		if a.Style != nil {
			st := api.AssistantEmailStyle(*a.Style)
			job.Style = &st
		}
		if a.ApplicationID != nil && *a.ApplicationID != "" {
			title, company, err := s.aboutJob(ctx, tx, p, *a.ApplicationID)
			if err != nil {
				return err
			}
			job.JobTitle, job.ClientCompany = apiconv.OrNull(&title), apiconv.OrNull(&company)
		}
		if s.deps.Agency != nil {
			if job.Agency, err = s.deps.Agency.TenantName(ctx, tx, p.TenantID); err != nil {
				return err
			}
		}
		if out, err = store.New(tx).InsertEmail(ctx, a.Mode); err != nil {
			return err
		}
		job.DraftId = out.ID
		if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindAssistantEmail, job); err != nil {
			return err
		}
		// What was asked or written is about people: only the mode is logged.
		return audit.Record(ctx, tx, audit.Entry{Action: "assistant.email_asked", EntityType: "assistant_email", EntityID: out.ID,
			Changes: map[string]any{"mode": a.Mode}})
	})
	return out, err
}

// ready is the company's AI provider, when it is on and the month's allowance
// is not used up.
func (s *Service) ready(ctx context.Context, tx pgx.Tx) (*api.AiAccess, error) {
	ai, on, err := s.deps.AI.Access(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !on {
		return nil, apperr.New(api.ErrorCodeASSISTANTOFF, "The AI assistant is switched off: no AI provider is set up yet.")
	}
	used, err := s.used(ctx, tx)
	if err != nil {
		return nil, err
	}
	if used >= s.settings.MonthlyTokens {
		return nil, apperr.New(api.ErrorCodeASSISTANTLIMITREACHED, "Your company has used this month's AI allowance; it starts again on the 1st.")
	}
	return ai, nil
}

// aboutJob is the title and client of an application's job, as the user may see them.
func (s *Service) aboutJob(ctx context.Context, tx pgx.Tx, p authz.Principal, applicationID string) (string, string, error) {
	if !p.Can(api.PermissionPipelineRead) || !p.Can(api.PermissionJobsRead) {
		return "", "", apperr.NotFound("application")
	}
	app, err := s.deps.Applications.ForAssistant(ctx, tx, applicationID)
	if err != nil {
		return "", "", err
	}
	j, err := s.deps.Jobs.ForAssistant(ctx, tx, p, app.JobID)
	return j.Title, j.CompanyName, err
}

// EmailDraft returns an email the AI is writing for the signed-in person.
func (s *Service) EmailDraft(ctx context.Context, p authz.Principal, id string) (store.AssistantEmail, error) {
	var out store.AssistantEmail
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).GetEmail(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("email draft")
		}
		return err
	})
	return out, err
}

// emailed handles assistant.emailed: what the AI wrote, or why it did not.
func (s *Service) emailed(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.AssistantEmailedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	params := store.SaveEmailParams{
		ID: r.DraftId, Status: "done", InputTokens: int32(min(r.InputTokens, 1<<30)), OutputTokens: int32(min(r.OutputTokens, 1<<30)), //nolint:gosec // capped
	}
	if v, err := r.Subject.Get(); err == nil && v != "" {
		params.Subject = clipPtr(v, 1000)
	}
	if v, err := r.Body.Get(); err == nil && v != "" {
		params.Body = clipPtr(v, 40000)
	}
	if v, err := r.Model.Get(); err == nil && v != "" {
		params.Model = clipPtr(v, 200)
	}
	if v, err := r.Error.Get(); err == nil && v != "" {
		params.Status, params.Error = "failed", clipPtr(v, 1000)
	} else if params.Body == nil {
		params.Status, params.Error = "failed", clipPtr("The AI wrote nothing.", 1000)
	}
	n, err := store.New(tx).SaveEmail(ctx, params)
	if err != nil || n == 0 {
		return err // already saved (a repeated result)
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "assistant.emailed", EntityType: "assistant_email", EntityID: r.DraftId,
		Changes: map[string]any{"status": params.Status, "tokens": params.InputTokens + params.OutputTokens}})
}
