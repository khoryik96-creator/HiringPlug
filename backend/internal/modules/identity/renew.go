package identity

import (
	"context"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// RenewSession gives the signed-in person's session a new expiry,
// SessionLength from now, while they keep working: the same session, so
// signing out (or a password change, a deactivation) still ends it. The
// request itself passed every sign-in check (not signed out, not ended, an
// active person). API keys have no session to renew.
func (m *Module) RenewSession(_ context.Context, p authz.Principal) (Session, error) {
	sess := p.Session()
	if sess.ID == "" {
		return Session{}, apperr.New(api.ErrorCodeFORBIDDEN, "Only a sign-in can be renewed.")
	}
	now := time.Now().Truncate(time.Second)
	token, err := m.verifier.RenewSession(p.TenantID, p.UserID, sess.Method, sess.ID, now, SessionLength)
	return Session{Token: token, ExpiresAt: now.Add(SessionLength)}, err
}

// RenewSession handles POST /v1/auth/renew.
func (h *HTTP) RenewSession(ctx context.Context, _ api.RenewSessionRequestObject) (api.RenewSessionResponseObject, error) {
	s, err := h.m.RenewSession(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.RenewSession200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}
