package emails

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func (h *HTTP) CreateOutlookDraft(ctx context.Context, r api.CreateOutlookDraftRequestObject) (api.CreateOutlookDraftResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.writeOutlookDraft(ctx, p, "", "", *r.Body, false)
	return api.CreateOutlookDraft201JSONResponse(out), err
}
func (h *HTTP) GetOutlookDraft(ctx context.Context, r api.GetOutlookDraftRequestObject) (api.GetOutlookDraftResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.writeOutlookDraft(ctx, p, r.MessageId, "", api.OutlookDraftInput{}, false)
	return api.GetOutlookDraft200JSONResponse(out), err
}
func (h *HTTP) UpdateOutlookDraft(ctx context.Context, r api.UpdateOutlookDraftRequestObject) (api.UpdateOutlookDraftResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	in := r.Body
	out, err := h.svc.writeOutlookDraft(ctx, p, r.MessageId, in.Version, api.OutlookDraftInput{Subject: in.Subject, Body: in.Body, BodyHtml: in.BodyHtml, To: in.To, Cc: in.Cc, Bcc: in.Bcc}, false)
	return api.UpdateOutlookDraft200JSONResponse(out), err
}
func (h *HTTP) DeleteOutlookDraft(ctx context.Context, r api.DeleteOutlookDraftRequestObject) (api.DeleteOutlookDraftResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	_, err = h.svc.writeOutlookDraft(ctx, p, r.MessageId, r.Params.Version, api.OutlookDraftInput{}, true)
	return api.DeleteOutlookDraft204Response{}, err
}
func (h *HTTP) ScheduleOutlookDraft(ctx context.Context, r api.ScheduleOutlookDraftRequestObject) (api.ScheduleOutlookDraftResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.scheduleOutlook(ctx, p, r.MessageId, *r.Body)
	return api.ScheduleOutlookDraft202JSONResponse(out), err
}
func (h *HTTP) ListOutlookScheduled(ctx context.Context, r api.ListOutlookScheduledRequestObject) (api.ListOutlookScheduledResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	offset := 0
	if r.Params.Offset != nil {
		offset = *r.Params.Offset
	}
	out, err := h.svc.scheduledOutlook(ctx, p, offset)
	return api.ListOutlookScheduled200JSONResponse(out), err
}
func (h *HTTP) ResolveOutlookScheduled(ctx context.Context, r api.ResolveOutlookScheduledRequestObject) (api.ResolveOutlookScheduledResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.resolveOutlookSend(ctx, p, r.SendId, r.Body.Version)
	return api.ResolveOutlookScheduled200JSONResponse(out), err
}
func (h *HTTP) CancelOutlookScheduled(ctx context.Context, r api.CancelOutlookScheduledRequestObject) (api.CancelOutlookScheduledResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.cancelOutlookSend(ctx, p, r.SendId, r.Body.Version)
	return api.CancelOutlookScheduled200JSONResponse(out), err
}
