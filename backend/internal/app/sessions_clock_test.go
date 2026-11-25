package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// whileHeld runs request while the person's row is held by someone else for
// over a second; it returns the database's clock (to the second) just
// before the row is let go, and the request's answer.
func whileHeld(t *testing.T, tenantID, userID string, request func() *response) (time.Time, *response) {
	t.Helper()
	var released time.Time
	res := meanwhile(t, `SELECT 1 FROM app_user WHERE tenant_id = $1 AND id = $2 FOR NO KEY UPDATE`, []any{tenantID, userID}, request,
		func(tx pgx.Tx) {
			time.Sleep(1200 * time.Millisecond)
			require.NoError(t, tx.QueryRow(context.Background(), `SELECT date_trunc('second', clock_timestamp())`).Scan(&released))
		})
	return released, res
}

func userTime(t *testing.T, userID, column string) time.Time {
	t.Helper()
	var at time.Time
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT `+column+` FROM app_user WHERE id = $1`, userID).Scan(&at))
	return at
}

// An end of sessions is cut off by the database's clock when it is made,
// not when its transaction started: one that waited for the person's row
// (while a sign-in of theirs finished, say) still ends every session issued
// before it.
func TestSessionsEndWhenTheEndIsMadeNotWhenItStarted(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	owner := tenant.Users[api.RoleOwner]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug

	// Deactivated.
	admin := tenant.Users[api.RoleAdmin]
	released, res := whileHeld(t, tenant.ID, admin.ID, func() *response {
		return f.a.request(http.MethodPatch, "/v1/team/"+admin.ID, owner.Token, map[string]any{"active": false})
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.True(t, userTime(t, admin.ID, "tokens_valid_after").After(released), "deactivated")

	// Their Microsoft account unlinked.
	recruiter := tenant.Users[api.RoleRecruiter]
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow)))
	released, res = whileHeld(t, tenant.ID, recruiter.ID, func() *response { return f.unlink(owner.Token, recruiter.ID) })
	require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	assert.True(t, userTime(t, recruiter.ID, "microsoft_sessions_after").After(released), "unlinked")

	// Their password changed.
	sourcer := tenant.Users[api.RoleSourcer]
	setPassword(t, sourcer.ID, testPassword)
	released, res = whileHeld(t, tenant.ID, sourcer.ID, func() *response {
		return f.a.request(http.MethodPut, "/v1/me/password", sourcer.Token, map[string]any{"currentPassword": testPassword, "newPassword": "a-long-new-password-1"})
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.True(t, userTime(t, sourcer.ID, "tokens_valid_after").After(released), "password changed")

	// Their two-step sign-in reset.
	viewer := tenant.Users[api.RoleViewer]
	released, res = whileHeld(t, tenant.ID, viewer.ID, func() *response {
		return f.a.request(http.MethodDelete, "/v1/team/"+viewer.ID+"/two-step", owner.Token, nil)
	})
	require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	assert.True(t, userTime(t, viewer.ID, "tokens_valid_after").After(released), "two-step reset")
}

func issuedAt(t *testing.T, token string) time.Time {
	t.Helper()
	var c jwt.RegisteredClaims
	_, _, err := jwt.NewParser().ParseUnverified(token, &c)
	require.NoError(t, err)
	require.NotNil(t, c.IssuedAt)
	return c.IssuedAt.Time
}

// Sessions are issued by the database's clock (the one every end of
// sessions is cut off by), as of the sign-in's transaction, never by the app
// server's: a sign-in that waited over a second for the person's row is
// dated when its transaction started, like its last sign-in time.
func TestSessionsAreIssuedByTheDatabasesClock(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	setPassword(t, recruiter.ID, testPassword)
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow))) // linked

	_, res := whileHeld(t, tenant.ID, recruiter.ID, func() *response {
		res, _ := f.a.signIn(map[string]any{"email": sam.email, "password": testPassword})
		return res
	})
	assert.Equal(t, userTime(t, recruiter.ID, "date_trunc('second', last_sign_in_at)"), issuedAt(t, session(t, res)), "a password session")

	flow, _ = f.start(map[string]any{"company": slug})
	ticket := f.ticketFor(flow, sam)
	_, res = whileHeld(t, tenant.ID, recruiter.ID, func() *response { return f.complete(ticket, flow.binding, "") })
	assert.Equal(t, userTime(t, recruiter.ID, "date_trunc('second', last_sign_in_at)"), issuedAt(t, session(t, res)), "a Microsoft session")
}
