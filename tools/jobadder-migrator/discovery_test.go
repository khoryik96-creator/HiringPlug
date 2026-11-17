package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testPass = "synthetic workspace passphrase"

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err = s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestDiscoveryPreservesOriginalBytesAndUnknowns(t *testing.T) {
	raw := []byte("{\n  \"totalCount\": 80000, \"items\": [], \"custom\": {\"salary\":9007199254740993.125, \"wording\":\"alowance\", \"empty\":\"\", \"nil\":null, \"name\":\"李\"}\n}")
	calls := map[string]int{}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("source write attempted")
		}
		if r.Header.Get("Authorization") != "Bearer secret-test" {
			t.Error("missing auth")
		}
		calls[r.URL.Path]++
		if r.URL.Path == "/contacts" {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":"sensitive source details"}`))
			return
		}
		if r.URL.Query().Get("Limit") != "0" {
			t.Error("discovery must request counts, not all records")
		}
		_, _ = w.Write(raw)
	}))
	defer source.Close()
	s := testStore(t)
	run, err := s.Start("source-A", false)
	if err != nil {
		t.Fatal(err)
	}
	resources := []Resource{{"candidates", "Candidates", "/candidates", "count"}, {"contacts", "Contacts", "/contacts", "count"}}
	c := &Client{Config: Connection{API: source.URL, AccessToken: "secret-test"}, HTTP: secureHTTP(), Wait: wait}
	if err = discover(context.Background(), s, c, run, resources, false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Original(run, "candidates")
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("source fields or original bytes changed", err)
	}
	report, err := s.Report()
	if err != nil {
		t.Fatal(err)
	}
	if *report.Results[0].Count != 80000 || report.Results[1].Count != nil || report.Results[1].Status != "denied" {
		t.Fatalf("wrong coverage: %+v", report.Results)
	}
	if _, err = s.Original(run, "contacts"); err == nil {
		t.Fatal("error bodies must not be archived")
	}
	if err = discover(context.Background(), s, c, run, resources, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if calls["/candidates"] != 1 || calls["/contacts"] != 2 {
		t.Fatal("resume did not skip completed resources and retry unresolved resources")
	}
	if _, err = s.Start("source-B", false); err == nil {
		t.Fatal("workspace allowed another source account")
	}
}
func TestWorkspaceEncryptedAndLocked(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(dir); err == nil {
		t.Fatal("second process/store could open workspace")
	}
	if err = s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	c := Connection{API: "https://api.jobadder.com/v2", AccessToken: "private-access-value", RefreshToken: "private-refresh-value", ClientSecret: "private-client-secret"}
	if err = s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	paths, _ := filepath.Glob(filepath.Join(dir, "migration.sqlite*"))
	for _, p := range paths {
		body, _ := os.ReadFile(p)
		if bytes.Contains(body, []byte(c.AccessToken)) || bytes.Contains(body, []byte(c.ClientSecret)) {
			t.Fatal("unencrypted credentials in workspace")
		}
	}
	reopened, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.Unlock("a wrong but sufficiently long phrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if err = reopened.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Config()
	if err != nil || got.AccessToken != c.AccessToken || got.ClientSecret != c.ClientSecret {
		t.Fatal("saved credentials not recovered", err)
	}
}
func TestNoInventedTotals(t *testing.T) {
	for _, raw := range []string{`{"items":[{"id":1}]}`, `{"totalCount":null}`, `{"totalCount":-1}`, `{"totalCount":2.5}`, `null`, `[]`} {
		if _, err := countResponse([]byte(raw), "count"); err == nil {
			t.Errorf("invented count from %s", raw)
		}
	}
	n, err := countResponse([]byte(`[]`), "definitions")
	if err != nil || n != 0 {
		t.Fatal("unpaged empty definitions should count as zero")
	}
	n, err = countResponse([]byte(`{"totalCount":0,"items":[]}`), "count")
	if err != nil || n != 0 {
		t.Fatal("explicit zero not handled")
	}
}
func TestPauseAndResumeKeepsSuccessfulCheckpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"totalCount":1}`)) }))
	defer source.Close()
	s := testStore(t)
	run, _ := s.Start("source", false)
	c := &Client{Config: Connection{API: source.URL}, HTTP: secureHTTP(), Wait: wait}
	resources := []Resource{{"a", "A", "/a", "count"}, {"b", "B", "/b", "count"}}
	err := discover(ctx, s, c, run, resources, false, func(label string) {
		if label == "B" {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("pause did not stop requests")
	}
	report, _ := s.Report()
	if len(report.Results) != 1 {
		t.Fatal("completed checkpoint not retained")
	}
	if err = discover(context.Background(), s, c, run, resources, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("resume duplicated a successful resource")
	}
}
func TestCatalogPathsAreReadOnlyAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range Catalog() {
		if seen[r.Key] {
			t.Fatal("duplicate resource")
		}
		seen[r.Key] = true
		if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "?") {
			t.Fatal("unsafe catalog path")
		}
	}
	data, _ := json.Marshal(Coverage)
	if !bytes.Contains(data, []byte("not a completed migration")) {
		t.Fatal("coverage limitations missing")
	}
}
func TestCredentialRotationSavedBeforeRetriedRequest(t *testing.T) {
	saved := false
	apiCalls := 0
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == tokenEndpoint {
			if r.Method != "POST" {
				t.Error("token exchange method")
			}
			return response(200, `{"access_token":"rotated-access","refresh_token":"rotated-refresh","token_type":"Bearer","api":"https://api.jobadder.com/v2"}`), nil
		}
		apiCalls++
		if apiCalls == 1 {
			return response(401, `{}`), nil
		}
		if !saved || r.Header.Get("Authorization") != "Bearer rotated-access" {
			t.Error("refresh not saved before retry")
		}
		return response(200, `{"totalCount":1}`), nil
	})}
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2", AccessToken: "expired", RefreshToken: "old", ClientID: "id", ClientSecret: "secret"}, HTTP: h, Wait: func(context.Context, time.Duration) error { return nil }, Save: func(c Connection) error { saved = c.RefreshToken == "rotated-refresh"; return nil }}
	if _, _, err := c.get(context.Background(), "/candidates?Limit=0"); err != nil {
		t.Fatal(err)
	}
	if !saved || apiCalls != 2 {
		t.Fatal("refresh rotation failed")
	}
}
func TestDisconnectedSourcePausesInsteadOfRetryingEveryResource(t *testing.T) {
	s := testStore(t)
	run, _ := s.Start("source", false)
	calls := 0
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("network unavailable")
	})}, Wait: func(context.Context, time.Duration) error { return nil }}
	resources := []Resource{{"a", "A", "/a", "count"}, {"b", "B", "/b", "count"}}
	if err := discover(context.Background(), s, c, run, resources, false, func(string) {}); err == nil {
		t.Fatal("disconnected inventory did not pause")
	}
	r, err := s.Report()
	if err != nil || calls != 4 || len(r.Results) != 1 || r.Results[0].Count != nil {
		t.Fatal("connection failure retried across all resources or invented a count")
	}
}
