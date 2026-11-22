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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
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
	var started, revealed, checked, disconnected, senCalls atomic.Int32
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
			reply = map[string]any{"protocol": 2, "flow_id": apolloTestFlow, "authorization_url": "https://app.apollo.io/#/oauth/authorize?" + query.Encode()}
		case "/poll":
			assert.Equal(t, apolloTestFlow, data["flow_id"])
			reply = map[string]any{"state": "connected", "name": "Test Recruiter", "connection_id": apolloTestFlow}
		case "/reveal-v2":
			revealed.Add(1)
			person := data["person"].(map[string]any)
			if person["url"] == "https://www.linkedin.com/in/sen" {
				if senCalls.Add(1) == 1 {
					reply = map[string]any{"state": "limited", "not_dispatched": true}
				} else {
					reply = map[string]any{"state": "found", "email": "sen@maju.example", "identity_checked": true}
				}
				break
			}
			op, _ = data["operation_id"].(string)
			assert.Len(t, op, 64)
			assert.Equal(t, apolloTestFlow, data["connection_id"])
			assert.Equal(t, true, data["confirmed"])
			assert.Equal(t, map[string]any{"url": "https://www.linkedin.com/in/farah", "name": "Farah Aziz", "company": "Maju Bank",
				"company_url": "https://www.linkedin.com/company/maju-bank/", "company_id": "123456"}, data["person"])
			status = http.StatusGatewayTimeout // The broker recorded it, but the response was lost.
		case "/lookup-status-v2":
			checked.Add(1)
			assert.Equal(t, op, data["operation_id"])
			reply = map[string]any{"state": "found", "email": "farah@maju.example", "email_status": "catch_all", "identity_checked": true}
		case "/disconnect-v2":
			if disconnected.Add(1) == 1 {
				status = http.StatusServiceUnavailable
			} else {
				reply = map[string]any{"protocol": 2, "connected": false}
			}
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
		person("li", "Li Foo", "Recruiter"),
		person("rahim", "Rahim Foo", "Recruiter"),
		person("sen", "Sen Foo", "Recruiter"),
	}}, http.StatusOK)
	a.hrScan(http.MethodPost, scan+"/joined", recruiter, map[string]any{"url": "https://www.linkedin.com/in/lee", "current": false}, http.StatusOK)
	a.createContact(recruiter, map[string]any{"companyId": bank.Id, "firstName": "Mira", "lastName": "Wong",
		"linkedinUrl": "https://www.linkedin.com/in/mira/", "email": "mira@maju.example"})

	a.createContact(recruiter, map[string]any{"companyId": bank.Id, "firstName": "Li", "lastName": "Foo",
		"linkedinUrl": "https://www.linkedin.com/in/li", "email": "li@maju.example", "status": "left"})
	rahim := a.createContact(recruiter, map[string]any{"companyId": bank.Id, "firstName": "Rahim", "lastName": "Foo",
		"linkedinUrl": "https://www.linkedin.com/in/rahim", "email": "rahim@maju.example"})
	_, err := testutil.Current.Admin.Exec(t.Context(), `UPDATE contact_mailbox_check SET status='invalid' WHERE contact_id = $1`, rahim.Id)
	require.NoError(t, err)

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
	for _, profile := range []string{"farah", "li", "rahim"} {
		assert.Equal(t, api.ApolloLookupStateDisconnected, state(recruiter, "preview", profile, nil).State, profile)
	}

	start := a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "start"})
	require.Equal(t, http.StatusOK, start.Status, "%s", start.Body)
	require.NotContains(t, string(start.Body), proofHash)
	assert.Equal(t, "no-store", start.Header.Get("Cache-Control"), "the sign-in link is not cached")
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
	stepUntil := func(wanted int32) {
		t.Helper()
		for range 16 {
			if revealed.Load() == wanted {
				break
			}
			worked, runErr := worker.RunOnce(t.Context())
			require.NoError(t, runErr)
			if !worked {
				break
			}
		}
		require.Equal(t, wanted, revealed.Load())
	}
	stepUntil(1)
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
	sen := state(recruiter, "preview", "sen", nil)
	assert.Equal(t, api.ApolloLookupStateQueued, state(recruiter, "unlock", "sen", sen.Confirmation).State)
	_, err = testutil.Current.Admin.Exec(t.Context(), `UPDATE hr_apollo_lookup
		SET created_at=now()-interval '20 minutes', updated_at=now()-interval '20 minutes'
		WHERE tenant_id=$1 AND profile_url='https://www.linkedin.com/in/sen'`, tenant.ID)
	require.NoError(t, err)
	expired := state(recruiter, "preview", "sen", nil)
	assert.Equal(t, api.ApolloLookupStateCancelled, expired.State)
	require.Equal(t, api.ApolloLookupReasonQueueExpired, *expired.Reason)
	require.NotNil(t, expired.Confirmation)
	assert.NotEqual(t, *sen.Confirmation, *expired.Confirmation)
	assert.Equal(t, api.ApolloLookupStateQueued, state(recruiter, "unlock", "sen", expired.Confirmation).State)
	stepUntil(2)

	limited := state(recruiter, "preview", "sen", nil)
	assert.Equal(t, api.ApolloLookupStateCancelled, limited.State)
	require.Equal(t, api.ApolloLookupReasonRetryableLimit, *limited.Reason)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, base, recruiter, api.ApolloLookupRequest{
		ProfileUrl: "https://www.linkedin.com/in/sen", Action: api.ApolloLookupRequestActionUnlock, Confirmation: expired.Confirmation,
	}).Status, "a replay of the previous confirmation cannot queue a retry")
	assert.Equal(t, api.ApolloLookupStateQueued, state(recruiter, "unlock", "sen", limited.Confirmation).State)
	stepUntil(3)
	assert.Equal(t, api.ApolloLookupStateFound, state(recruiter, "preview", "sen", nil).State)
	assert.EqualValues(t, 2, senCalls.Load(), "the expired job never reached the broker")

	disconnect := map[string]string{"action": "disconnect"}
	assert.Equal(t, "disconnected_locally", a.request(http.MethodPost, connection, recruiter, disconnect).Map()["state"])
	assert.Equal(t, "disconnected_locally", a.request(http.MethodGet, connection, recruiter, nil).Map()["state"])
	assert.Equal(t, "disconnected_locally", a.request(http.MethodPost, connection, recruiter, map[string]string{"action": "start"}).Map()["state"])
	assert.Equal(t, api.ApolloLookupStatePreviousConnection, state(recruiter, "preview", "farah", nil).State)
	assert.Equal(t, "disconnected", a.request(http.MethodPost, connection, recruiter, disconnect).Map()["state"])
	assert.EqualValues(t, 2, disconnected.Load())
	assert.Equal(t, api.ApolloLookupStatePreviousConnection, state(recruiter, "preview", "farah", nil).State)
}

func TestApolloLookupFollowsTheRecruitersCurrentAccess(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_APOLLO_BROKER_URL": apolloTestOrigin, "HP_APOLLO_KEYS_SECRET": strings.Repeat("0123456789abcdef", 4)})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	var revealed atomic.Int32
	a.modules.Contacts.ConfigureApollo(a.deps.Config.Apollo, mockApolloTransport(func(req *http.Request) (*http.Response, error) {
		defer func() { _ = req.Body.Close() }()
		reply := map[string]any{}
		switch req.URL.Path {
		case "/start":
			query := url.Values{"client_id": {"test-only"}, "redirect_uri": {apolloTestOrigin + "/callback"},
				"response_type": {"code"}, "scope": {"read_user_profile people_match app_scopes"}, "state": {apolloTestFlow}}
			reply = map[string]any{"protocol": 2, "flow_id": apolloTestFlow, "authorization_url": "https://app.apollo.io/#/oauth/authorize?" + query.Encode()}
		case "/poll":
			reply = map[string]any{"state": "connected", "name": "Test Recruiter", "connection_id": apolloTestFlow}
		case "/reveal-v2":
			revealed.Add(1)
			reply = map[string]any{"state": "found", "email": "farah@maju.example", "identity_checked": true}
		default:
			t.Errorf("unexpected Apollo action %s", req.URL.Path)
		}
		body, err := json.Marshal(reply)
		assert.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	}))

	bank := a.createCompany(recruiter, map[string]any{"name": "Maju Bank", "linkedinUrl": "https://www.linkedin.com/company/maju-bank/"})
	scan := "/v1/companies/" + bank.Id + "/hr-scan"
	a.hrScan(http.MethodPut, scan, recruiter, map[string]any{"linkedinCompanyId": "123456", "linkedinCompanyName": "Maju Bank",
		"linkedinCompanyUrl": "https://www.linkedin.com/company/maju-bank/", "keywords": []string{"HR manager"}, "pages": 1}, http.StatusOK)
	a.hrScan(http.MethodPost, scan+"/pages", recruiter, map[string]any{"query": 0, "page": 1, "people": []any{
		person("farah", "Farah Aziz", "Talent Acquisition Lead"),
	}}, http.StatusOK)
	for _, action := range []string{"start", "poll"} {
		require.Equal(t, http.StatusOK, a.request(http.MethodPost, "/v1/contacts/apollo", recruiter, map[string]string{"action": action}).Status)
	}
	lookup := func(action string, confirmation *string) (int, api.ApolloLookup) {
		t.Helper()
		res := a.request(http.MethodPost, scan+"/apollo", recruiter, api.ApolloLookupRequest{Action: api.ApolloLookupRequestAction(action),
			ProfileUrl: "https://www.linkedin.com/in/farah", Confirmation: confirmation})
		var out api.ApolloLookup
		if res.Status == http.StatusOK {
			res.JSON(&out)
		}
		return res.Status, out
	}
	_, preview := lookup("preview", nil)
	require.Equal(t, api.ApolloLookupStateReady, preview.State)
	_, queued := lookup("unlock", preview.Confirmation)
	require.Equal(t, api.ApolloLookupStateQueued, queued.State)

	// The owner takes Apollo away before the worker reaches the confirmed lookup.
	defaults := authz.PermissionsForRole(api.RoleRecruiter)
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": without(defaults, api.PermissionContactsApolloUse),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	worker, err := app.NewWorker(a.deps, a.modules)
	require.NoError(t, err)
	worker.Tenant = tenant.ID
	for range 16 {
		worked, runErr := worker.RunOnce(t.Context())
		require.NoError(t, runErr)
		if !worked {
			break
		}
	}
	assert.Zero(t, revealed.Load(), "a recruiter who lost Apollo access spends no credit")
	var state string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT state FROM hr_apollo_lookup WHERE tenant_id=$1 AND profile_url='https://www.linkedin.com/in/farah'", tenant.ID).Scan(&state))
	assert.Equal(t, "cancelled", state)

	// Apollo without contacts:read does not show the scan or saved addresses.
	a.setRole(owner, "recruiter", map[string]any{"version": 1, "permissions": without(defaults, api.PermissionContactsRead),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	status, _ := lookup("preview", nil)
	assert.Equal(t, http.StatusForbidden, status)
}
