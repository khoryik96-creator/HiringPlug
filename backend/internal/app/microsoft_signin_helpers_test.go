package app_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// msTest is the API with a fake Microsoft: the server's app is
// server-client, and Microsoft's sign-in service is entratest's.
type msTest struct {
	t  *testing.T
	a  *testApp
	ms *entratest.Server
}

func newMicrosoftTest(t *testing.T, overrides ...map[string]string) *msTest {
	t.Helper()
	a := newTestApp(t, append([]map[string]string{{
		"HP_PUBLIC_URL": "http://app.test", "HP_MAIL_MICROSOFT_CLIENT_ID": "server-client", "HP_MAIL_MICROSOFT_CLIENT_SECRET": "server-secret",
	}}, overrides...)...)
	ms := entratest.New(t)
	a.modules.Identity.UseMicrosoftService(ms.URL, ms.Client())
	return &msTest{t: t, a: a, ms: ms}
}

// newGUID is a random organisation or object ID: every test has its own
// organisation, so tests running together never match each other's people.
func newGUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:])
}

func randomBinding() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func bindingHash(binding string) string {
	sum := sha256.Sum256([]byte(binding))
	return hex.EncodeToString(sum[:])
}

// enable lets the tenant's team sign in with Microsoft accounts of org.
func (f *msTest) enable(ownerToken, org string, required bool) api.MicrosoftSignInSettings {
	f.t.Helper()
	res := f.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", ownerToken, map[string]any{"enabled": true, "organisationId": org, "required": required})
	require.Equal(f.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.MicrosoftSignInSettings
	res.JSON(&out)
	return out
}

// msFlow is a sign-in started in one browser tab.
type msFlow struct {
	binding                                     string
	authority, clientID, state, nonce, redirect string
	challenge, loginHint                        string
}

// start starts a sign-in (body: email, company or invite).
func (f *msTest) start(body map[string]any) (msFlow, *response) {
	f.t.Helper()
	return f.begin("/v1/auth/microsoft/start", "", body)
}

// startConnecting starts connecting the signed-in person's Microsoft account
// (Your account); proof is their password and two-step code, if they have any.
func (f *msTest) startConnecting(token string, proof ...map[string]any) (msFlow, *response) {
	f.t.Helper()
	body := map[string]any{}
	for _, p := range proof {
		maps.Copy(body, p)
	}
	return f.begin("/v1/me/microsoft/start", token, body)
}

// connect connects the Microsoft account claims are for to the person
// signed in with token (proof: as for startConnecting).
func (f *msTest) connect(token string, claims func(msFlow) jwt.MapClaims, proof ...map[string]any) *response {
	f.t.Helper()
	flow, res := f.startConnecting(token, proof...)
	require.Equal(f.t, http.StatusOK, res.Status, "%s", res.Body)
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: claims(flow)})
	return f.finishConnecting(token, f.ticket(flow, code), flow.binding)
}

func (f *msTest) finishConnecting(token, ticket, binding string) *response {
	f.t.Helper()
	return f.a.request(http.MethodPost, "/v1/me/microsoft/complete", token, map[string]any{"ticket": ticket, "binding": binding})
}

// connectRefused checks connecting was refused, and why.
func connectRefused(t *testing.T, res *response, reason string) {
	t.Helper()
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "%s", res.Body)
	e := res.Error()
	require.Equal(t, api.ErrorCodeMICROSOFTCONNECTFAILED, e.Error.Code)
	assert.Equal(t, reason, (*e.Error.Details)["reason"])
}

func (f *msTest) begin(path, token string, body map[string]any) (msFlow, *response) {
	f.t.Helper()
	flow := msFlow{binding: randomBinding()}
	if body == nil {
		body = map[string]any{}
	}
	body["bindingHash"] = bindingHash(flow.binding)
	res := f.a.request(http.MethodPost, path, token, body)
	if res.Status != http.StatusOK {
		return flow, res
	}
	var out api.MicrosoftSignInRedirect
	res.JSON(&out)
	u, err := url.Parse(out.Url)
	require.NoError(f.t, err)
	require.True(f.t, strings.HasPrefix(out.Url, f.ms.URL+"/"), "to Microsoft's sign-in service: %s", out.Url)
	q := u.Query()
	assert.Equal(f.t, "S256", q.Get("code_challenge_method"))
	assert.Equal(f.t, "code", q.Get("response_type"))
	flow.authority = strings.Split(strings.TrimPrefix(u.Path, "/"), "/")[0]
	flow.clientID, flow.state, flow.nonce, flow.redirect, flow.challenge = q.Get("client_id"), q.Get("state"), q.Get("nonce"), q.Get("redirect_uri"), q.Get("code_challenge")
	flow.loginHint = q.Get("login_hint")
	require.NotEmpty(f.t, flow.state)
	require.NotEmpty(f.t, flow.nonce)
	return flow, res
}

// back is Microsoft sending the browser back (query: code, state, error...);
// it returns the app address the browser is sent on to.
func (f *msTest) back(query url.Values) *url.URL {
	f.t.Helper()
	res := f.a.request(http.MethodGet, "/v1/auth/microsoft/callback?"+query.Encode(), "", nil)
	require.Equal(f.t, http.StatusSeeOther, res.Status, "%s", res.Body)
	assert.Equal(f.t, "no-store", res.Header.Get("Cache-Control"))
	assert.Equal(f.t, "no-referrer", res.Header.Get("Referrer-Policy"))
	loc := res.Header.Get("Location")
	require.True(f.t, strings.HasPrefix(loc, "http://app.test/#/sign-in/microsoft?"), "back to the app only: %s", loc)
	u, err := url.Parse(strings.Replace(loc, "/#/sign-in/microsoft?", "/sign-in/microsoft?", 1))
	require.NoError(f.t, err)
	return u
}

// ticket is the one-time ticket Microsoft's redirect gives for code.
func (f *msTest) ticket(flow msFlow, code string) string {
	f.t.Helper()
	u := f.back(url.Values{"code": {code}, "state": {flow.state}, "session_state": {"x"}})
	q := u.Query()
	require.Empty(f.t, q.Get("error"))
	require.Len(f.t, q, 1, "only the ticket: %s", u)
	return q.Get("ticket")
}

// complete finishes a sign-in with a ticket and the tab's binding.
func (f *msTest) complete(ticket, binding, company string) *response {
	f.t.Helper()
	body := map[string]any{"ticket": ticket, "binding": binding}
	if company != "" {
		body["company"] = company
	}
	return f.a.request(http.MethodPost, "/v1/auth/microsoft/complete", "", body)
}

// account is a Microsoft account of an organisation.
type account struct{ org, oid, email string }

func newAccount(org, email string) account { return account{org: org, oid: newGUID(), email: email} }

// claims is what Microsoft says about acc for flow.
func (acc account) claims(flow msFlow) jwt.MapClaims {
	return entratest.Claims(acc.org, acc.oid, acc.email, flow.clientID, flow.nonce)
}

// signInAs runs a whole sign-in: Microsoft vouches for claims; the same tab finishes.
func (f *msTest) signInAs(flow msFlow, claims jwt.MapClaims) *response {
	f.t.Helper()
	code := "code-" + randomBinding()
	f.ms.Grant(code, entratest.Grant{Claims: claims})
	return f.complete(f.ticket(flow, code), flow.binding, "")
}

// session is the session of a successful sign-in.
func session(t *testing.T, res *response) string {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var s api.Session
	res.JSON(&s)
	return s.Token
}

// msRefused checks a sign-in was refused, and why.
func msRefused(t *testing.T, res *response, reason string) api.Error {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, res.Status, "%s", res.Body)
	e := res.Error()
	require.Equal(t, api.ErrorCodeMICROSOFTSIGNINFAILED, e.Error.Code)
	require.NotNil(t, e.Error.Details)
	assert.Equal(t, reason, (*e.Error.Details)["reason"])
	return e
}

// me is who a token signs in.
func (f *msTest) me(token string) api.Me {
	f.t.Helper()
	res := f.a.request(http.MethodGet, "/v1/me", token, nil)
	require.Equal(f.t, http.StatusOK, res.Status, "%s", res.Body)
	var me api.Me
	res.JSON(&me)
	return me
}

// audited is the changes of a tenant's audit entries with action, oldest first.
func audited(t *testing.T, tenantID, action string) []string {
	t.Helper()
	rows, err := testutil.Current.Admin.Query(context.Background(),
		`SELECT coalesce(changes::text, '') FROM audit_log WHERE tenant_id = $1 AND action = $2 ORDER BY created_at, id`, tenantID, action)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		out = append(out, c)
	}
	return out
}
