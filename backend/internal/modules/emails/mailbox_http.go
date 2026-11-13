package emails

import (
	"context"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func (h *HTTP) GetMailboxConnection(ctx context.Context, _ api.GetMailboxConnectionRequestObject) (api.GetMailboxConnectionResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.mailboxStatus(ctx, p)
	return api.GetMailboxConnection200JSONResponse(out), err
}
func (h *HTTP) SaveSmtpMailbox(ctx context.Context, r api.SaveSmtpMailboxRequestObject) (api.SaveSmtpMailboxResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.saveSMTP(ctx, p, *r.Body); err != nil {
		return nil, err
	}
	out, err := h.svc.mailboxStatus(ctx, p)
	return api.SaveSmtpMailbox200JSONResponse(out), err
}
func (h *HTTP) DisconnectMailbox(ctx context.Context, _ api.DisconnectMailboxRequestObject) (api.DisconnectMailboxResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.disconnectMailbox(ctx, p); err != nil {
		return nil, err
	}
	return api.DisconnectMailbox204Response{}, nil
}
func (h *HTTP) AuthorizeMailbox(ctx context.Context, r api.AuthorizeMailboxRequestObject) (api.AuthorizeMailboxResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	target, err := h.svc.authorizeMailbox(ctx, p, string(r.Body.Provider))
	if err != nil {
		return nil, err
	}
	return api.AuthorizeMailbox200JSONResponse{Url: target}, nil
}
func (h *HTTP) CompleteMailboxAuthorization(ctx context.Context, r api.CompleteMailboxAuthorizationRequestObject) (api.CompleteMailboxAuthorizationResponseObject, error) {
	// Never reflect provider errors, codes, state or tokens into the browser or logs.
	// Nothing is connected here: the sign-in is parked, and this browser is
	// sent back to the app with a one-time ticket to finish it, signed in.
	outcome := "failed"
	if ticket, err := h.svc.parkMailbox(ctx, deref(r.Params.State), deref(r.Params.Code), deref(r.Params.Error)); err == nil {
		outcome = "confirm&ticket=" + ticket
	}
	cache, policy := "no-store", "no-referrer"
	target := h.svc.connections.PublicURL + "/#/email-connection?mailbox=" + outcome
	return api.CompleteMailboxAuthorization303Response{Headers: api.CompleteMailboxAuthorization303ResponseHeaders{Location: &target, CacheControl: &cache, ReferrerPolicy: &policy}}, nil
}

// FinishMailboxAuthorization handles POST /v1/mailbox/complete.
func (h *HTTP) FinishMailboxAuthorization(ctx context.Context, r api.FinishMailboxAuthorizationRequestObject) (api.FinishMailboxAuthorizationResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.finishMailbox(ctx, p, r.Body.Ticket); err != nil {
		return nil, err
	}
	out, err := h.svc.mailboxStatus(ctx, p)
	return api.FinishMailboxAuthorization200JSONResponse(out), err
}

func mailboxUser(ctx context.Context) (authz.Principal, error) {
	p := authz.MustFromContext(ctx)
	if !strings.HasPrefix(p.UserID, "usr_") {
		return p, apperr.New(api.ErrorCodeFORBIDDEN, "API keys cannot manage a personal mailbox; sign in as a user.")
	}
	return p, nil
}
