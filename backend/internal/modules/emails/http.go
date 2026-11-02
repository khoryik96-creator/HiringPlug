package emails

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the emails operations of the contract.
type HTTP struct{ svc *Service }

// ListEmailTemplates handles GET /v1/email-templates.
func (h *HTTP) ListEmailTemplates(ctx context.Context, _ api.ListEmailTemplatesRequestObject) (api.ListEmailTemplatesResponseObject, error) {
	rows, err := h.svc.Templates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListEmailTemplates200JSONResponse{Items: make([]api.EmailTemplate, len(rows)), Placeholders: Placeholders}
	for i, r := range rows {
		out.Items[i] = templateToAPI(r)
	}
	return out, nil
}

// CreateEmailTemplate handles POST /v1/email-templates.
func (h *HTTP) CreateEmailTemplate(ctx context.Context, req api.CreateEmailTemplateRequestObject) (api.CreateEmailTemplateResponseObject, error) {
	b := req.Body
	row, err := h.svc.CreateTemplate(ctx, authz.MustFromContext(ctx), Template{Name: b.Name, Subject: b.Subject, Body: b.Body})
	if err != nil {
		return nil, err
	}
	return api.CreateEmailTemplate201JSONResponse(templateToAPI(row)), nil
}

// UpdateEmailTemplate handles PUT /v1/email-templates/{templateId}.
func (h *HTTP) UpdateEmailTemplate(ctx context.Context, req api.UpdateEmailTemplateRequestObject) (api.UpdateEmailTemplateResponseObject, error) {
	b := req.Body
	row, err := h.svc.UpdateTemplate(ctx, authz.MustFromContext(ctx), req.TemplateId, b.Version, Template{Name: b.Name, Subject: b.Subject, Body: b.Body})
	if err != nil {
		return nil, err
	}
	return api.UpdateEmailTemplate200JSONResponse(templateToAPI(row)), nil
}

// DraftEmail handles POST /v1/email-drafts.
func (h *HTTP) DraftEmail(ctx context.Context, req api.DraftEmailRequestObject) (api.DraftEmailResponseObject, error) {
	b := req.Body
	d, err := h.svc.Draft(ctx, authz.MustFromContext(ctx), DraftRequest{
		TemplateID: b.TemplateId, CandidateID: b.CandidateId, ApplicationID: b.ApplicationId, InterviewID: b.InterviewId, TimeZone: b.TimeZone,
	})
	if err != nil {
		return nil, err
	}
	return api.DraftEmail200JSONResponse{To: convert[openapi_types.Email](d.To), Subject: d.Subject, Body: d.Body, Missing: d.Missing}, nil
}

// ListEmails handles GET /v1/emails.
func (h *HTTP) ListEmails(ctx context.Context, req api.ListEmailsRequestObject) (api.ListEmailsResponseObject, error) {
	rows, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ListEmails200JSONResponse{Items: make([]api.Email, len(rows))}
	for i, r := range rows {
		out.Items[i] = emailToAPI(r)
	}
	return out, nil
}

// SendEmail handles POST /v1/emails.
func (h *HTTP) SendEmail(ctx context.Context, req api.SendEmailRequestObject) (api.SendEmailResponseObject, error) {
	b := req.Body
	o := Outgoing{To: convert[string](b.To), Subject: b.Subject, Body: b.Body}
	if b.Cc != nil {
		o.Cc = convert[string](*b.Cc)
	}
	row, err := h.svc.Send(ctx, authz.MustFromContext(ctx), o, b.CandidateId, b.ApplicationId)
	if err != nil {
		return nil, err
	}
	return api.SendEmail202JSONResponse(emailToAPI(row)), nil
}

func templateToAPI(r store.EmailTemplate) api.EmailTemplate {
	return api.EmailTemplate{
		Id: r.ID, Version: int(r.Version), Name: r.Name, Subject: r.Subject, Body: r.Body, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func emailToAPI(r store.Email) api.Email {
	return api.Email{
		Id: r.ID, CandidateId: c.OrNull(r.CandidateID), ApplicationId: c.OrNull(r.ApplicationID),
		To: r.ToAddresses, Cc: r.CcAddresses, Subject: r.Subject, Body: r.Body,
		Status: api.EmailStatus(r.Status), Error: c.OrNull(r.Error),
		SentBy: r.SentBy, SentByName: r.SenderName, CreatedAt: r.CreatedAt, SentAt: c.OrNull(r.SentAt),
	}
}

// convert changes a list of strings between string types (addresses).
func convert[To ~string, From ~string](in []From) []To {
	out := make([]To, len(in))
	for i, v := range in {
		out[i] = To(v)
	}
	return out
}
