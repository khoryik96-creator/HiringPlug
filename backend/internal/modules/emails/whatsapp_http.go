package emails

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListWhatsAppTemplates handles GET /v1/whatsapp-templates.
func (h *HTTP) ListWhatsAppTemplates(ctx context.Context, _ api.ListWhatsAppTemplatesRequestObject) (api.ListWhatsAppTemplatesResponseObject, error) {
	rows, err := h.svc.WhatsAppTemplates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListWhatsAppTemplates200JSONResponse{Items: make([]api.WhatsAppTemplate, len(rows)), Placeholders: WhatsAppPlaceholders}
	for i, r := range rows {
		out.Items[i] = whatsAppTemplateToAPI(r)
	}
	return out, nil
}

// CreateWhatsAppTemplate handles POST /v1/whatsapp-templates.
func (h *HTTP) CreateWhatsAppTemplate(ctx context.Context, req api.CreateWhatsAppTemplateRequestObject) (api.CreateWhatsAppTemplateResponseObject, error) {
	row, err := h.svc.CreateWhatsAppTemplate(ctx, authz.MustFromContext(ctx), WhatsAppTemplate{Name: req.Body.Name, Body: req.Body.Body})
	if err != nil {
		return nil, err
	}
	return api.CreateWhatsAppTemplate201JSONResponse(whatsAppTemplateToAPI(row)), nil
}

// UpdateWhatsAppTemplate handles PUT /v1/whatsapp-templates/{templateId}.
func (h *HTTP) UpdateWhatsAppTemplate(ctx context.Context, req api.UpdateWhatsAppTemplateRequestObject) (api.UpdateWhatsAppTemplateResponseObject, error) {
	b := req.Body
	row, err := h.svc.UpdateWhatsAppTemplate(ctx, authz.MustFromContext(ctx), req.TemplateId, b.Version, WhatsAppTemplate{Name: b.Name, Body: b.Body})
	if err != nil {
		return nil, err
	}
	return api.UpdateWhatsAppTemplate200JSONResponse(whatsAppTemplateToAPI(row)), nil
}

// DeleteWhatsAppTemplate handles DELETE /v1/whatsapp-templates/{templateId}.
func (h *HTTP) DeleteWhatsAppTemplate(ctx context.Context, req api.DeleteWhatsAppTemplateRequestObject) (api.DeleteWhatsAppTemplateResponseObject, error) {
	if err := h.svc.DeleteWhatsAppTemplate(ctx, authz.MustFromContext(ctx), req.TemplateId); err != nil {
		return nil, err
	}
	return api.DeleteWhatsAppTemplate204Response{}, nil
}

// DraftWhatsApp handles POST /v1/whatsapp-drafts.
func (h *HTTP) DraftWhatsApp(ctx context.Context, req api.DraftWhatsAppRequestObject) (api.DraftWhatsAppResponseObject, error) {
	b := req.Body
	d, err := h.svc.WhatsAppDraft(ctx, authz.MustFromContext(ctx), WhatsAppDraftRequest{
		TemplateID: b.TemplateId, CandidateID: b.CandidateId, ContactID: b.ContactId, ApplicationID: b.ApplicationId, TimeZone: b.TimeZone,
	})
	if err != nil {
		return nil, err
	}
	out := api.DraftWhatsApp200JSONResponse{Text: d.Text, Missing: d.Missing, Numbers: make([]api.WhatsAppNumber, len(d.Numbers))}
	for i, n := range d.Numbers {
		out.Numbers[i] = api.WhatsAppNumber{Label: n.Label, Number: n.Number}
		if n.Chat != "" {
			out.Numbers[i].ChatNumber = &n.Chat
		}
	}
	return out, nil
}

func whatsAppTemplateToAPI(r store.WhatsappTemplate) api.WhatsAppTemplate {
	return api.WhatsAppTemplate{Id: r.ID, Version: int(r.Version), Name: r.Name, Body: r.Body, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
