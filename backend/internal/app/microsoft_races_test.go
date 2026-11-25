package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// meanwhile is a change made by someone else while a request runs: it
// starts a transaction (sql, held open), runs request, and once the request
// waits for that transaction, or has finished, runs then (if any) and
// commits. It returns the request's answer.
func meanwhile(t *testing.T, sql string, args []any, request func() *response, then func(tx pgx.Tx)) *response {
	t.Helper()
	ctx := context.Background()
	tx, err := testutil.Current.Admin.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	var pid int32
	require.NoError(t, tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	_, err = tx.Exec(ctx, sql, args...)
	require.NoError(t, err)

	done := make(chan *response, 1)
	go func() { done <- request() }()
	var answer *response
	deadline := time.Now().Add(10 * time.Second)
	for answer == nil && time.Now().Before(deadline) {
		select {
		case answer = <-done:
			continue
		default:
		}
		var waiting int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE $1 = ANY (pg_blocking_pids(pid))`, pid).Scan(&waiting))
		if waiting > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if then != nil {
		then(tx)
	}
	require.NoError(t, tx.Commit(ctx))
	if answer == nil {
		select {
		case answer = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the request did not finish")
		}
	}
	return answer
}

// What a sign-in found before its transaction is checked again inside it:
// an account unlinked, or a person made an owner, meanwhile, signs nobody in.
func TestMicrosoftSignInChecksAgainWhatChangedMeanwhile(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	org := newGUID()
	f.enable(owner.Token, org, false)
	slug := f.me(owner.Token).Tenant.CareersSlug
	sam := newAccount(org, emailOf(api.RoleRecruiter, tenant.ID))
	flow, _ := f.start(map[string]any{"company": slug})
	session(t, f.signInAs(flow, sam.claims(flow))) // linked

	// Its link removed while it signs in.
	flow, _ = f.start(map[string]any{"company": slug})
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: sam.claims(flow)})
	ticket := f.ticket(flow, code)
	res := meanwhile(t, `DELETE FROM user_microsoft WHERE tenant_id = $1 AND user_id = $2`, []any{tenant.ID, recruiter.ID},
		func() *response { return f.complete(ticket, flow.binding, "") }, nil)
	msRefused(t, res, "failed")

	// Made an owner while their account is being linked by its address.
	ada := newAccount(org, emailOf(api.RoleAdmin, tenant.ID))
	flow, _ = f.start(map[string]any{"company": slug})
	f.ms.Grant(code+"b", entratest.Grant{Claims: ada.claims(flow)})
	ticket = f.ticket(flow, code+"b")
	res = meanwhile(t, `UPDATE app_user SET role = 'owner' WHERE tenant_id = $1 AND id = $2`, []any{tenant.ID, admin.ID},
		func() *response { return f.complete(ticket, flow.binding, "") }, nil)
	msRefused(t, res, "connect_first")
	assert.Empty(t, f.methodsOf(owner.Token, admin.ID), "nothing linked")
}

// Turning Microsoft sign-in on and changing the Microsoft app it uses wait
// for each other: an admin's change never slips in as it is turned on, nor
// is it turned on with an app removed meanwhile.
func TestTheMicrosoftAppAndSignInChangeOneAtATime(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_PUBLIC_URL": "http://app.test"}) // no server app: only the company's
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner, admin := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token
	app := func(client string) map[string]any {
		return map[string]any{"clientId": client, "clientSecret": "agency-secret-value"}
	}
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", admin, app("first-client")).Status)
	org := newGUID()

	// The owner turns it on (their transaction, held) as an admin changes the app.
	res := meanwhile(t, `SELECT 1 FROM tenant WHERE id = $1 FOR UPDATE`, []any{tenant.ID},
		func() *response {
			return a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", admin, app("admins-client"))
		},
		func(tx pgx.Tx) {
			_, err := tx.Exec(context.Background(), `UPDATE tenant SET microsoft_sign_in = true, microsoft_tenant_id = $2 WHERE id = $1`, tenant.ID, org)
			require.NoError(t, err)
		})
	require.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMICROSOFTAPPOWNERSONLY, res.Error().Error.Code)

	// Off again; an admin removes the app (their transaction, held) as the owner turns it on.
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE tenant SET microsoft_sign_in = false WHERE id = $1`, tenant.ID)
	require.NoError(t, err)
	res = meanwhile(t, `SELECT 1 FROM tenant WHERE id = $1 FOR SHARE`, []any{tenant.ID},
		func() *response {
			return a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner, map[string]any{"enabled": true, "organisationId": org, "required": false})
		},
		func(tx pgx.Tx) {
			_, err := tx.Exec(context.Background(), `DELETE FROM email_mail_app WHERE tenant_id = $1 AND provider = 'microsoft'`, tenant.ID)
			require.NoError(t, err)
		})
	require.Equal(t, http.StatusBadRequest, res.Status, "turned on with no app: %s", res.Body)
	var s api.MicrosoftSignInSettings
	a.request(http.MethodGet, "/v1/agency/microsoft-sign-in", owner, nil).JSON(&s)
	assert.False(t, s.Enabled)
}
