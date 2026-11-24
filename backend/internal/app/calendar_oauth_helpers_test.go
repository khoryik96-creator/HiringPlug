package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"

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
	account                     string         // the signed-in Microsoft account ("synthetic-account" when empty)
	loseCreateAnswer            int            // creates whose event is stored but whose answer is lost (a 503)
	lastPatch                   map[string]any // what the last change sent
	deleteStatus                int            // answer to deletes when not 0 (a broken or busy Outlook)
	trialJoinURL                string
	onWrite                     func() // runs as Outlook takes a create or change, before it answers
	strictMatch                 bool   // a change or delete whose If-Match is not the event's changeKey is refused (412)
	config                      calendars.Config
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
		_ = json.NewEncoder(w).Encode(map[string]string{"id": f.signedIn(), "mail": f.signedIn() + "@example.test"})
	case r.URL.Path == "/me/calendar":
		_, _ = w.Write([]byte(`{"id":"default","name":"Calendar","canEdit":true,"allowedOnlineMeetingProviders":["teamsForBusiness"]}`))
	case strings.HasSuffix(r.URL.Path, "/calendarView"):
		items := []map[string]any{}
		for _, e := range f.events {
			if accountOf(e) == f.signedIn() { // each account sees its own calendar
				items = append(items, answered(r, e))
			}
		}
		body := map[string]any{"value": items}
		if f.brokenPages {
			body["@odata.nextLink"] = "https://evil.test/steal"
		}
		_ = json.NewEncoder(w).Encode(body)
	case strings.HasSuffix(r.URL.Path, "/events") && r.Method == "POST":
		f.creates++
		if f.onWrite != nil {
			f.onWrite()
		}
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			return
		}
		var e map[string]any
		_ = json.NewDecoder(r.Body).Decode(&e)
		for _, old := range f.events { // a mailbox keeps one event per transactionId
			if tx, _ := e["transactionId"].(string); tx != "" && old["transactionId"] == tx && accountOf(old) == f.signedIn() {
				_ = json.NewEncoder(w).Encode(answered(r, old))
				return
			}
		}
		f.nextID++
		id := "remote-" + time.Now().Format("150405.000000000")
		e["id"] = id
		e["changeKey"] = "v1"
		e["showAs"] = "busy"
		if f.trialJoinURL != "" && e["isOnlineMeeting"] == true {
			e["onlineMeeting"] = map[string]string{"joinUrl": f.trialJoinURL}
		}
		e["_account"] = f.signedIn()
		f.events[id] = e
		if f.loseCreateAnswer > 0 {
			f.loseCreateAnswer--
			w.WriteHeader(503) // stored, but the answer never arrives
			return
		}
		_ = json.NewEncoder(w).Encode(answered(r, e))
	case strings.Contains(r.URL.Path, "/events/"):
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		e, ok := f.events[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		if f.strictMatch && r.Method != "GET" && r.Header.Get("If-Match") != `W/"`+fmt.Sprint(e["changeKey"])+`"` {
			w.WriteHeader(http.StatusPreconditionFailed) // changed in Outlook since that version
			return
		}
		switch r.Method {
		case "DELETE":
			if f.deleteStatus != 0 {
				w.WriteHeader(f.deleteStatus)
				return
			}
			delete(f.events, id)
			w.WriteHeader(204)
			return
		case "PATCH":
			f.patches++
			if f.onWrite != nil {
				f.onWrite()
			}
			if f.writeStatus != 0 {
				w.WriteHeader(f.writeStatus)
				return
			}
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			f.lastPatch = patch
			for key, v := range patch {
				e[key] = v
			}
			e["changeKey"] = fmt.Sprintf("v%d", f.patches+1)
		}
		_ = json.NewEncoder(w).Encode(answered(r, e))
	default:
		w.WriteHeader(404)
	}
}

// accountOf is whose calendar an event is in (events a test seeds: the first account's).
func accountOf(e map[string]any) string {
	if a, _ := e["_account"].(string); a != "" {
		return a
	}
	return "synthetic-account"
}

// signedIn is the account the fixture's tokens belong to (callers hold f.mu).
func (f *outlookFixture) signedIn() string {
	if f.account == "" {
		return "synthetic-account"
	}
	return f.account
}
func setupOutlook(t *testing.T, a *testApp) *outlookFixture {
	t.Helper()
	f := &outlookFixture{events: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	f.config = calendars.Config{KeysSecret: []byte("synthetic-calendar-key-01234567890123456789"), PublicURL: "http://app.test", HTTP: server.Client(), Provider: calendars.Provider{AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", GraphURL: server.URL}}
	a.modules.Calendars.ConnectOutlookWith(calendars.Deps{Apps: calendarTestApps{}}, f.config)
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

// runPostponedCalendarJobs makes the tenant's postponed jobs due (a retry an
// Outlook error put off) and runs the queue until it is idle.
func runPostponedCalendarJobs(t *testing.T, a *testApp, tenant string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(t.Context(), `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND status = 'pending'`, tenant)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant)
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// answered is an event as Outlook answers it: with Prefer
// outlook.body-content-type="text", an HTML body comes back as its text (the
// event keeps what was sent, for the tests to read).
func answered(r *http.Request, e map[string]any) map[string]any {
	body, _ := e["body"].(map[string]any)
	if body == nil || body["contentType"] != "html" || !strings.Contains(r.Header.Get("Prefer"), `body-content-type="text"`) {
		return e
	}
	out := make(map[string]any, len(e))
	for k, v := range e {
		out[k] = v
	}
	content, _ := body["content"].(string)
	text := tagRe.ReplaceAllString(strings.NewReplacer("</p>", "\n", "<br>", "\n", "</li>", "\n").Replace(content), "")
	out["body"] = map[string]any{"contentType": "text", "content": strings.TrimSpace(text)}
	return out
}
