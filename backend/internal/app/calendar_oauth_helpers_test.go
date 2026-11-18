package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type calendarTestApps struct{}

func (calendarTestApps) MicrosoftApp(context.Context, pgx.Tx, string) (calendars.Application, error) {
	return calendars.Application{ClientID: "synthetic-client", ClientSecret: "synthetic-secret"}, nil
}

type outlookFixture struct {
	mu                          sync.Mutex
	events                      map[string]map[string]any
	creates, patches, refreshes int
	nextID                      int
	writeStatus                 int
	brokenPages                 bool
}

func (f *outlookFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		expires := 1
		if r.Form.Get("grant_type") == "refresh_token" {
			f.refreshes++
			expires = 3600
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "synthetic-access", "refresh_token": "synthetic-rotated-refresh", "token_type": "Bearer", "expires_in": expires, "scope": "Calendars.ReadWrite User.Read offline_access"})
	case r.URL.Path == "/me":
		_, _ = w.Write([]byte(`{"id":"synthetic-account","mail":"recruiter@example.test"}`))
	case r.URL.Path == "/me/calendar":
		_, _ = w.Write([]byte(`{"id":"default","name":"Calendar","canEdit":true,"allowedOnlineMeetingProviders":["teamsForBusiness"]}`))
	case strings.HasSuffix(r.URL.Path, "/calendarView"):
		items := []map[string]any{}
		for _, e := range f.events {
			items = append(items, e)
		}
		body := map[string]any{"value": items}
		if f.brokenPages {
			body["@odata.nextLink"] = "https://evil.test/steal"
		}
		_ = json.NewEncoder(w).Encode(body)
	case strings.HasSuffix(r.URL.Path, "/events") && r.Method == "POST":
		f.creates++
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			return
		}
		var e map[string]any
		_ = json.NewDecoder(r.Body).Decode(&e)
		f.nextID++
		id := "remote-" + time.Now().Format("150405.000000000")
		e["id"] = id
		e["changeKey"] = "v1"
		e["showAs"] = "busy"
		f.events[id] = e
		_ = json.NewEncoder(w).Encode(e)
	case strings.Contains(r.URL.Path, "/events/"):
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		e, ok := f.events[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case "DELETE":
			delete(f.events, id)
			w.WriteHeader(204)
			return
		case "PATCH":
			f.patches++
			if f.writeStatus != 0 {
				w.WriteHeader(f.writeStatus)
				return
			}
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			for key, v := range patch {
				e[key] = v
			}
			e["changeKey"] = "v2"
		}
		_ = json.NewEncoder(w).Encode(e)
	default:
		w.WriteHeader(404)
	}
}
func setupOutlook(t *testing.T, a *testApp) *outlookFixture {
	t.Helper()
	f := &outlookFixture{events: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	a.modules.Calendars.ConnectOutlookWith(calendars.Deps{Apps: calendarTestApps{}}, calendars.Config{KeysSecret: []byte("synthetic-calendar-key-01234567890123456789"), PublicURL: "http://app.test", HTTP: server.Client(), Provider: calendars.Provider{AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", GraphURL: server.URL}})
	return f
}
func outlookTicket(t *testing.T, a *testApp, token string) string {
	t.Helper()
	r := a.request("POST", "/v1/calendar/authorize", token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var authorization struct{ Url string }
	r.JSON(&authorization)
	target, err := url.Parse(authorization.Url)
	require.NoError(t, err)
	assert.Contains(t, target.Query().Get("scope"), "Calendars.ReadWrite")
	assert.NotContains(t, target.Query().Get("scope"), "Mail.")
	r = a.request("GET", "/v1/calendar/callback?"+url.Values{"state": {target.Query().Get("state")}, "code": {"synthetic-code"}}.Encode(), "", nil)
	require.Equal(t, 303, r.Status)
	assert.Equal(t, "no-store", r.Header.Get("Cache-Control"))
	assert.Equal(t, "no-referrer", r.Header.Get("Referrer-Policy"))
	redirect, err := url.Parse(r.Header.Get("Location"))
	require.NoError(t, err)
	parts := strings.SplitN(redirect.Fragment, "?", 2)
	require.Len(t, parts, 2)
	params, err := url.ParseQuery(parts[1])
	require.NoError(t, err)
	return params.Get("ticket")
}
func connectOutlook(t *testing.T, a *testApp, token string) {
	t.Helper()
	ticket := outlookTicket(t, a, token)
	r := a.request("POST", "/v1/calendar/complete", token, map[string]any{"ticket": ticket})
	require.Equal(t, 200, r.Status, "%s", r.Body)
}
func refreshOutlook(t *testing.T, a *testApp, token, tenant string) {
	t.Helper()
	r := a.request("POST", "/v1/calendar/refresh", token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant)
}
func calendarEventState(t *testing.T, id string) (version int, status string) {
	t.Helper()
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT version,sync_status FROM calendar_event WHERE id=$1", id).Scan(&version, &status))
	return
}
