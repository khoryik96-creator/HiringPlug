package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	s := testStore(t)
	return &App{store: s, origin: "http://localhost:8769", session: "synthetic-session", unlocked: true, clientHTTP: secureHTTP()}
}
func call(a *App, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:8769"+path, strings.NewReader(body))
	if auth {
		r.Header.Set("X-Migration-Session", a.session)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func TestLocalUIRequiresSessionAndRejectsOtherOrigins(t *testing.T) {
	a := testApp(t)
	if w := call(a, "GET", "/api/status", "", false); w.Code != 401 {
		t.Fatal("unauthorized local data exposed")
	}
	for _, headers := range []map[string]string{{"Origin": "https://evil.example"}, {"Host": "evil.example:8769"}} {
		r := httptest.NewRequest("GET", a.origin+"/api/status", nil)
		r.Header.Set("X-Migration-Session", a.session)
		for k, v := range headers {
			if k == "Host" {
				r.Host = v
			} else {
				r.Header.Set(k, v)
			}
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("cross-origin/rebinding allowed")
		}
	}
	a.connection = Connection{API: "https://api.jobadder.com/v2", AccessToken: "private-access", ClientSecret: "private-secret", Fingerprint: "source"}
	w := call(a, "GET", "/api/status", "", true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-") {
		t.Fatal("status exposes credentials")
	}
	if call(a, "POST", "/api/connect", `{"api":"http://127.0.0.1/private","token":"secret"}`, true).Code != 400 {
		t.Fatal("arbitrary host accepted")
	}
}
func TestOriginalExportPreservesExactBytes(t *testing.T) {
	a := testApp(t)
	run, _ := a.store.Start("source", false)
	raw := []byte("{\n\"totalCount\":1,\"legacy\":{\"salary\":1234567890123456789.125,\"note\":\"typo alowance\",\"null\":null}}")
	n := int64(1)
	if err := a.store.SaveResult(run, Resource{Key: "candidates", Label: "Candidates", Kind: "count"}, "counted", &n, 200, raw); err != nil {
		t.Fatal(err)
	}
	w := call(a, "GET", "/api/originals.zip", "", true)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range z.File {
		if f.Name == "responses/candidates.json" {
			r, _ := f.Open()
			got, _ := io.ReadAll(r)
			_ = r.Close()
			found = bytes.Equal(got, raw)
		}
	}
	if !found {
		t.Fatal("original fields changed during export")
	}
}
func TestExportRefusesCorruptEvidenceBeforeWritingZip(t *testing.T) {
	a := testApp(t)
	run, _ := a.store.Start("source", false)
	n := int64(1)
	_ = a.store.SaveResult(run, Resource{Key: "a", Label: "A", Kind: "count"}, "counted", &n, 200, []byte(`{"totalCount":1}`))
	_, _ = a.store.db.Exec(`UPDATE results SET sha='wrong' WHERE run_id=?`, run)
	w := call(a, "GET", "/api/originals.zip", "", true)
	if w.Code != 400 || w.Header().Get("Content-Type") == "application/zip" {
		t.Fatal("corrupt evidence exported as a valid archive")
	}
}
func TestConcurrentConnectionAndInventoryAreBlocked(t *testing.T) {
	a := testApp(t)
	a.connection.Fingerprint = "source"
	a.connecting = true
	if call(a, "POST", "/api/start", `{"resume":false}`, true).Code != 400 {
		t.Fatal("inventory started while connection changing")
	}
	if call(a, "POST", "/api/connect", `{"api":"https://api.jobadder.com/v2","token":"secret"}`, true).Code != 400 {
		t.Fatal("concurrent connection allowed")
	}
	a.connecting = false
	if call(a, "POST", "/api/connect", `{"api":"bad","token":""}`, true).Code != 400 || a.connecting {
		t.Fatal("failed connection left application busy")
	}
}

func TestOAuthStateIsBoundExpiringAndOneUse(t *testing.T) {
	a := testApp(t)
	a.oauthState = "expected"
	a.oauthUntil = time.Now().Add(time.Minute)
	a.pending = Connection{ClientSecret: "private"}
	if call(a, "GET", "/oauth/callback?state=wrong&error=denied", "", false).Code != 400 {
		t.Fatal("wrong state accepted")
	}
	if a.oauthState != "expected" {
		t.Fatal("invalid callback cancelled legitimate request")
	}
	if w := call(a, "GET", "/oauth/callback?state=expected&error=denied", "", false); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if call(a, "GET", "/oauth/callback?state=expected&error=denied", "", false).Code != 400 {
		t.Fatal("state replay accepted")
	}
	a.oauthState = "expired"
	a.oauthUntil = time.Now().Add(-time.Second)
	if call(a, "GET", "/oauth/callback?state=expired", "", false).Code != 400 {
		t.Fatal("expired state accepted")
	}
}
func TestInventoryCompletesWithExplicitAccessGap(t *testing.T) {
	a := testApp(t)
	a.demo = true
	a.connection = Connection{API: "https://api.jobadder.com/v2", Account: "Demo", Fingerprint: "synthetic"}
	a.clientHTTP = &http.Client{Transport: demoTransport{}}
	if w := call(a, "POST", "/api/start", `{"resume":false}`, true); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		a.mu.Lock()
		running := a.running
		a.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("inventory did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	report, err := a.store.Report()
	if err != nil || report.Status != "finished" || len(report.Results) != len(Catalog()) {
		t.Fatal("inventory incomplete", err)
	}
	for _, r := range report.Results {
		if r.Key == "payroll" && (r.Status != "deferred" || r.Count != nil || r.HTTP != 0) {
			t.Fatalf("candidate-level payroll was probed: %+v", r)
		}
		if r.Key == "opportunities" && (r.Status != "denied" || r.Count != nil) {
			t.Fatal("denied resource counted as zero")
		}
	}
}
func TestIdentityCannotSwitchWorkspaceAccount(t *testing.T) {
	a := testApp(t)
	c := Connection{API: "https://api.jobadder.com/v2", AccessToken: "synthetic", Fingerprint: "existing-source"}
	if err := a.store.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	a.clientHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return response(200, `{"userId":2,"accountName":"Different agency"}`), nil
	})}
	if err := a.accept(context.Background(), c); err == nil {
		t.Fatal("cross-account resume accepted")
	}
	stored, _ := a.store.Config()
	if stored.Fingerprint != "existing-source" {
		t.Fatal("existing connection overwritten")
	}
}
func TestUnverifiedRefreshCannotReplaceWorkspaceCredentials(t *testing.T) {
	a := testApp(t)
	old := Connection{API: "https://api.jobadder.com/v2", AccessToken: "existing-access", Fingerprint: "existing-source"}
	if err := a.store.SaveConfig(old); err != nil {
		t.Fatal(err)
	}
	a.connection = old
	a.clientHTTP = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == tokenEndpoint {
			return response(200, `{"access_token":"other-access","refresh_token":"other-refresh","token_type":"Bearer","api":"https://api.jobadder.com/v2"}`), nil
		}
		if r.Header.Get("Authorization") == "Bearer expired" {
			return response(401, `{}`), nil
		}
		return response(200, `{"userId":2,"accountName":"Different agency"}`), nil
	})}
	err := a.accept(context.Background(), Connection{API: old.API, AccessToken: "expired", RefreshToken: "refresh", ClientID: "id", ClientSecret: "secret"})
	if err == nil {
		t.Fatal("different account accepted after refresh")
	}
	stored, err := a.store.Config()
	if err != nil || stored != old || a.connection != old {
		t.Fatal("unverified refresh replaced the existing connection")
	}
}
func TestFormsRejectUnknownFieldsAndNeverExposeSecret(t *testing.T) {
	a := testApp(t)
	if call(a, "POST", "/api/unlock", `{"passphrase":"synthetic secret", "extra":1}`, true).Code != 400 {
		t.Fatal("unknown field accepted")
	}
	a.unlocked = false
	w := call(a, "POST", "/api/unlock", `{"passphrase":"short"}`, true)
	if w.Code != 400 || strings.Contains(w.Body.String(), "short") {
		t.Fatal("passphrase leaked")
	}
	a.unlocked = true
	w = call(a, "POST", "/api/oauth", `{"clientId":"test-id","clientSecret":"private-client-secret"}`, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-client-secret") {
		t.Fatal("client secret exposed")
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	u, _ := url.Parse(result["url"])
	if u.Query().Get("scope") != "read offline_access" {
		t.Fatal("source write scopes requested")
	}
}
