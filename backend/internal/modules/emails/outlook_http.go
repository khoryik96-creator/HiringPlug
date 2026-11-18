package emails

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func (h *HTTP) ListOutlookMail(ctx context.Context, r api.ListOutlookMailRequestObject) (api.ListOutlookMailResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.listOutlook(ctx, p, r.Params)
	return api.ListOutlookMail200JSONResponse(out), err
}
func (h *HTTP) SyncOutlookMail(ctx context.Context, _ api.SyncOutlookMailRequestObject) (api.SyncOutlookMailResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.SyncOutlookMail204Response{}, h.svc.requestOutlook(ctx, p, false)
}
func (h *HTTP) DisableOutlookMail(ctx context.Context, _ api.DisableOutlookMailRequestObject) (api.DisableOutlookMailResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.DisableOutlookMail204Response{}, h.svc.requestOutlook(ctx, p, true)
}
func (h *HTTP) GetOutlookMessage(ctx context.Context, r api.GetOutlookMessageRequestObject) (api.GetOutlookMessageResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.outlookBody(ctx, p, r.MessageId)
	return api.GetOutlookMessage200JSONResponse(out), err
}
func (h *HTTP) ChangeOutlookMessage(ctx context.Context, r api.ChangeOutlookMessageRequestObject) (api.ChangeOutlookMessageResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.ChangeOutlookMessage204Response{}, h.svc.changeOutlook(ctx, p, r.MessageId, *r.Body)
}
func (h *HTTP) ReplyOutlookMessage(ctx context.Context, r api.ReplyOutlookMessageRequestObject) (api.ReplyOutlookMessageResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.ReplyOutlookMessage202Response{}, h.svc.replyOutlook(ctx, p, r.MessageId, *r.Body)
}
func (h *HTTP) LinkOutlookConversation(ctx context.Context, r api.LinkOutlookConversationRequestObject) (api.LinkOutlookConversationResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.LinkOutlookConversation204Response{}, h.svc.linkOutlook(ctx, p, r.MessageId, r.Body)
}
func (h *HTTP) UnlinkOutlookConversation(ctx context.Context, r api.UnlinkOutlookConversationRequestObject) (api.UnlinkOutlookConversationResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.UnlinkOutlookConversation204Response{}, h.svc.linkOutlook(ctx, p, r.MessageId, nil)
}
