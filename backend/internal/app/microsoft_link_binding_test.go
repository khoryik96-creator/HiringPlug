package app_test

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// ticketFor runs a Microsoft sign-in up to its last step (Microsoft vouched
// for acc): the ticket the tab finishes with.
func (f *msTest) ticketFor(flow msFlow, acc account) string {
	f.t.Helper()
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: acc.claims(flow)})
	return f.ticket(flow, code)
}

// A sign-in started from a link finishes with that very link only: an
// invitation accepted with a password since does nothing, even though a
// reconnect link sent since works for the same person.
func TestASignInFinishesOnlyWithTheLinkItStartedFrom(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	f.enable(owner, org, false)
	res := f.a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": "pat." + tenant.ID + "@test.local", "name": "Pat", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var inv api.Invitation
	res.JSON(&inv)
	invite := regexp.MustCompile(`[0-9a-f]{64}$`).FindString(inv.Link)

	// Someone starts accepting the invitation with Microsoft (their account)...
	flow, res := f.start(map[string]any{"invite": invite})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	ticket := f.ticketFor(flow, newAccount(org, "mallory."+tenant.ID+"@contoso.test"))
	// ...Pat accepts it with a password, and is sent a reconnect link.
	res = f.a.request(http.MethodPost, "/v1/auth/links/"+invite, "", map[string]any{"password": "a-long-new-password-1"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	link := f.reconnectLink(owner, inv.Member.Id)

	msRefused(t, f.complete(ticket, flow.binding, ""), "failed")
	assert.Equal(t, []api.SignInMethod{api.SignInMethodPassword}, f.methodsOf(owner, inv.Member.Id), "no Microsoft account linked to Pat")
	assert.Empty(t, audited(t, tenant.ID, "user.microsoft_linked"))
	flow, _ = f.start(map[string]any{"invite": link})
	assert.Equal(t, inv.Member.Id, f.me(session(t, f.signInAs(flow, newAccount(org, "pat.upn."+tenant.ID+"@contoso.test").claims(flow)))).User.Id,
		"the reconnect link still works, from a sign-in started with it")
}

// Two sign-ins finishing with one link at once: the link is held, so one
// works and the other finds it used.
func TestOneLinkSignsInOnce(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	org := newGUID()
	f.enable(owner.Token, org, false)
	sam := newAccount(org, "sam.upn."+tenant.ID+"@contoso.test")
	link := f.reconnectLink(owner.Token, recruiter.ID)
	var linkID string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT id FROM user_link WHERE user_id = $1 AND kind = 'reconnect' AND used_at IS NULL`, recruiter.ID).Scan(&linkID))

	// Another finish holds the link, and uses it, while this one finishes.
	flow, _ := f.start(map[string]any{"invite": link})
	ticket := f.ticketFor(flow, sam)
	res := meanwhile(t, `SELECT 1 FROM user_link WHERE id = $1 FOR UPDATE`, []any{linkID},
		func() *response { return f.complete(ticket, flow.binding, "") },
		func(tx pgx.Tx) {
			_, err := tx.Exec(context.Background(), `UPDATE user_link SET used_at = now() WHERE id = $1`, linkID)
			require.NoError(t, err)
		})
	msRefused(t, res, "failed")
	assert.Empty(t, f.methodsOf(owner.Token, recruiter.ID))

	// Two at once, for real: one signs in.
	link = f.reconnectLink(owner.Token, recruiter.ID)
	flows, tickets := make([]msFlow, 2), make([]string, 2)
	for i := range flows {
		flows[i], _ = f.start(map[string]any{"invite": link})
		tickets[i] = f.ticketFor(flows[i], sam)
	}
	answers := make([]*response, 2)
	var wg sync.WaitGroup
	for i := range answers {
		wg.Go(func() { answers[i] = f.complete(tickets[i], flows[i].binding, "") })
	}
	wg.Wait()
	ok := 0
	for _, a := range answers {
		if a.Status == http.StatusOK {
			ok++
		} else {
			msRefused(t, a, "failed")
		}
	}
	assert.Equal(t, 1, ok, "one sign-in with the link")
}
