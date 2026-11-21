package app_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const apolloTestOrigin = "https://hp-apollo.tests.workers.dev"
const apolloTestFlow = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type mockApolloTransport func(req *http.Request) (*http.Response, error)

func (f mockApolloTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestFindHRApolloChecksExistingAndRecoversWithoutRepeatingThePaidCall(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_APOLLO_BROKER_URL": apolloTestOrigin, "HP_APOLLO_KEYS_SECRET": strings.Repeat("0123456789abcdef", 4)})
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	colleague := tenant.Users[api.RoleSourcer].Token
	outsider := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	viewer := tenant.Users[api.RoleViewer].Token
	var started, revealed, checked, disconnected atomic.Int32
	var proofHash, op string
	a.modules.Contacts.ConfigureApollo(a.deps.Config.Apollo, mockApolloTransport(func(req *http.Request) (*http.Response, error) {
		defer func() { _ = req.Body.Close() }()
		assert.Equal(t, "https://hp-apollo.tests.workers.dev", req.URL.Scheme+"://"+req.URL.Host)
		assert.Equal(t, "CVStudio-Apollo/1.0", req.UserAgent())
		var data map[string]any
		assert.NoError(t, json.NewDecoder(req.Body).Decode(&data))
		if req.URL.Path != "/start" {
			hash := sha256.Sum256([]byte(data["secret"].(string)))
			assert.Equal(t, proofHash, hex.EncodeToString(hash[:]))
		}
		status := http.StatusOK
		reply := map[string]any{}
		switch req.URL.Path {
		case "/start":
			started.Add(1)
			proofHash, _ = data["proof_hash"].(string)
			query := url.Values{"client_id": {"test-only"}, "redirect_uri": {apolloTestOrigin + "/callback"},
				"response_type": {"code"}, "scope": {"read_user_profile people_match app_scopes"}, "state": {apolloTestFlow}}
			reply = map[string]any{"flow_id": apolloTestFlow, "authorization_url": "https://app.apollo.io/#/oauth/authorize?" + query.Encode()}
		case "/poll":
			assert.Equal(t, apolloTestFlow, data["flow_id"])
			reply = map[string]any{"state": "connected", "name": "Test Recruiter", "connection_id": apolloTestFlow}
		case "/reveal":
			revealed.Add(1)
			op, _ = data["operation_id"].(string)
			assert.Len(t, op, 64)
			assert.Equal(t, apolloTestFlow, data["connection_id"])
			assert.Equal(t, true, data["confirmed"])
			assert.Equal(t, map[string]any{"url": "https://www.linkedin.com/in/farah", "name": "Farah Aziz", "company": "Maju Bank",
				"company_url": "https://www.linkedin.com/company/maju-bank/", "company_id": "123456"}, data["person"])
			status = http.StatusGatewayTimeout // The broker recorded it, but the response was lost.
		case "/lookup-status":
			checked.Add(1)
			assert.Equal(t, op, data["operation_id"])
			reply = map[string]any{"state": "found", "email": "farah@maju.example", "email_status": "catch_all"}
		case "/disconnect":
			disconnected.Add(1)
			reply = map[string]any{"connected": false}
		default:
			t.Errorf("unexpected Apollo action %s", req.URL.Path)
		}
		body, err := json.Marshal(reply)
		assert.NoError(t, err)
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	}))

	bank := a.createCompany(recruiter, map[string]any{"name": "Maju Bank", "linkedinUrl": "https://www.linkedin.com/company/maju-bank/"})
	scan := "/v1/companies/" + bank.Id + "/hr-scan"
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPut, scan, recruiter, map[string]any{
		"linkedinCompanyId": "123456", "linkedinCompanyName": "Maju Bank", "keywords": []string{"recruiter"}, "pages": 1,
		"linkedinCompanyUrl": "https://www.linkedin.com/company/wrong-employer/",
	}).Status)
	a.hrScan(http.MethodPut, scan, recruiter, map[string]any{"linkedinCompanyId": "123456", "linkedinCompanyName": "Maju Bank",
		"linkedinCompanyUrl": "https://www.linkedin.com/company/maju-bank/",
		"keywords":           []string{"HR manager"}, "pages": 1}, http.StatusOK)
	a.hrScan(http.MethodPost, scan+"/pages", recruiter, map[string]any{"query": 0, "page": 1, "people": []any{
		person("farah", "Farah Aziz", "Talent Acquisition Lead"),
		person("mira", "Mira Wong", "Recruiter"),
		person("lee", "Lee Tan", "Head of HR"),
	}}, http.StatusOK)
	a.hrScan(http.MethodPost, scan+"/joined", recruiter, map[string]any{"url": "https://www.linkedin.com/in/lee", "current": false}, http.StatusOK)
	a.createContact(recruiter, map[string]any{"companyId": bank.Id, "firstName": "Mira", "lastName": "Wong",
		"linkedinUrl": "https://www.linkedin.com/in/mira/", "email": "mira@maju.example"})

	base := scan + "/apollo"
	state := func(token, action, name string, confirmation *string) api.ApolloLookup {
		t.Helper()
		res := a.request(http.MethodPost, base, token, api.ApolloLookupRequest{Action: api.ApolloLookupRequestAction(action),
			ProfileUrl: "https://www.linkedin.com/in/" + name, Confirmation: confirmation})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		require.NotContains(t, string(res.Body), `"secret"`) // no raw broker proof fields
		var out api.ApolloLookup
		res.JSON(&out)
		return out
	}

	connection := "/v1/contacts/apollo"
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, connection, viewer, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, base, outsider, map[string]string{
		"action": "preview", "profileUrl": "https://www.linkedin.com/in/farah",
	}).Status)
	assert.Equal(t, api.ApolloLookupStateIneligible, state(recruiter, "preview", "lee", nil).State)
	existing := state(recruiter, "preview", "mira", nil)
	require.NotNil(t, existing.Email)
	assert.Equal(t, "mira@maju.example", *existing.Email)
	assert.Equal(t, api.ApolloLookupStateExisting, existing.State)
	assert.Equal(t, api.ApolloLookupStateDisconnected, state(recruiter, "preview", "farah", nil).State)

	start := a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "start"})
	require.Equal(t, http.StatusOK, start.Status, "%s", start.Body)
	require.NotContains(t, string(start.Body), proofHash)
	assert.Equal(t, http.StatusOK, a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "start"}).Status)
	assert.EqualValues(t, 1, started.Load(), "the second tab reuses the pending login")
	poll := a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "poll"})
	require.Equal(t, http.StatusOK, poll.Status, "%s", poll.Body)
	assert.Equal(t, "connected", poll.Map()["state"])
	assert.Equal(t, "no-store", a.request(http.MethodGet, connection, recruiter, nil).Header.Get("Cache-Control"))
	assert.Equal(t, "disconnected", a.request(http.MethodGet, connection, colleague, nil).Map()["state"])

	preview := state(recruiter, "preview", "Farah", nil)
	require.Equal(t, api.ApolloLookupStateReady, preview.State)
	require.NotNil(t, preview.Confirmation)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, base, recruiter, map[string]string{
		"action": "unlock", "profileUrl": "https://www.linkedin.com/in/farah",
	}).Status, "consent cannot be skipped")
	for range 2 {
		assert.Equal(t, api.ApolloLookupStateQueued, state(recruiter, "unlock", "farah", preview.Confirmation).State)
	}
	assert.Zero(t, revealed.Load(), "the API never sends the paid request")
	worker, err := app.NewWorker(a.deps, a.modules)
	require.NoError(t, err)
	worker.Tenant = tenant.ID
	for range 5 {
		if revealed.Load() != 0 {
			break
		}
		worked, runErr := worker.RunOnce(t.Context())
		require.NoError(t, runErr)
		if !worked {
			break
		}
	}
	require.EqualValues(t, 1, revealed.Load(), "lookup state is %s", state(recruiter, "preview", "farah", nil).State)
	assert.Equal(t, api.ApolloLookupStateUnconfirmed, state(recruiter, "preview", "farah", nil).State)
	payload, err := json.Marshal(api.ContactApolloLookupJob{LookupId: op})
	require.NoError(t, err)
	require.NoError(t, worker.Outside[api.JobKindContactApolloLookup](t.Context(), jobqueue.Job{TenantID: tenant.ID, Payload: payload}))
	assert.EqualValues(t, 1, revealed.Load(), "a worker retry with the same receipt never dispatches")
	recovered := state(recruiter, "check", "farah", nil)
	require.Equal(t, api.ApolloLookupStateFound, recovered.State)
	assert.Equal(t, "farah@maju.example", *recovered.Email)
	assert.Equal(t, api.ApolloLookupEmailStatusCatchAll, *recovered.EmailStatus)
	assert.EqualValues(t, 1, checked.Load())
	assert.EqualValues(t, 1, revealed.Load())
	assert.Equal(t, api.ApolloLookupStateDisconnected, state(colleague, "preview", "farah", nil).State)

	var sealed string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT email_sealed FROM hr_apollo_lookup WHERE tenant_id=$1 AND id=$2",
		tenant.ID, op).Scan(&sealed))
	assert.NotContains(t, sealed, "farah@maju.example")
	assert.Equal(t, http.StatusOK, a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "disconnect"}).Status)
	assert.EqualValues(t, 1, disconnected.Load())
	assert.Equal(t, api.ApolloLookupStatePreviousConnection, state(recruiter, "preview", "farah", nil).State)
}
