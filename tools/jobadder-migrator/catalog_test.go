package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestDiscoveryMatchesPublishedResponseEnvelopes(t *testing.T) {
	// These small, offline fixtures retain the public API's wire shapes and
	// count support. They contain no account data and require no network access.
	raw, err := os.ReadFile("testdata/discovery-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Resources []struct {
			Path      string
			Kind      string
			Schema    string
			LimitZero bool `json:"limitZero"`
			Body      json.RawMessage
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Resources) != len(Catalog()) {
		t.Fatal("catalogue/contract coverage mismatch")
	}
	kinds := map[string]string{}
	bodies := map[string][]byte{}
	for _, r := range fixture.Resources {
		if r.LimitZero != (r.Kind == "count" || r.Kind == "paged_definitions") || r.Schema == "" {
			t.Fatalf("invalid contract fixture for %s", r.Path)
		}
		kinds[r.Path] = r.Kind
		bodies[r.Path] = r.Body
	}
	for _, r := range Catalog() {
		if kinds[r.Path] != r.Kind {
			t.Fatalf("unsupported request mode for %s", r.Path)
		}
	}
	calls := 0
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, Wait: wait, HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Path[len("/v2"):]
		kind, ok := kinds[path]
		if !ok || kind == "deferred" || r.Method != "GET" {
			t.Errorf("unsupported source request: %s", path)
		}
		if (r.URL.Query().Get("Limit") == "0") != (kind == "count") || (kind == "paged_definitions" && (r.URL.Query().Get("Limit") != "1000" || r.URL.Query().Get("Offset") != "0")) {
			t.Errorf("incorrect pagination for %s", path)
		}
		calls++
		return response(200, string(bodies[path])), nil
	})}}
	s := testStore(t)
	run, _ := s.Start("source", false)
	if err := discover(context.Background(), s, c, run, Catalog(), false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Report()
	if err != nil || len(r.Results) != 47 || calls != 46 {
		t.Fatal("incomplete contract scan", err, calls)
	}
	for _, result := range r.Results {
		if result.Kind == "deferred" {
			if result.Status != "deferred" || result.Count != nil || result.HTTP != 0 {
				t.Fatal("candidate-level resource was probed")
			}
			continue
		}
		if result.Count == nil || *result.Count != 0 || (result.Status != "counted" && result.Status != "definitions") {
			t.Fatalf("valid API response rejected: %+v", result)
		}
	}
}

func TestDefinitionItemsAreUnpagedAndPreserved(t *testing.T) {
	for _, body := range []string{`{"items":[]}`, `{"items":null}`, `{"items":[{"fieldId":1,"name":"Text Field 1","type":"Text","values":["Keep alowance",null]}],"links":{"self":"https://api.jobadder.com/v2/candidates/fields/custom"}}`} {
		n, err := countResponse([]byte(body), "definitions")
		if err != nil || n < 0 {
			t.Fatal("valid definition envelope rejected", err)
		}
	}
	for _, body := range []string{`{}`, `{"items":3}`, `{"items":{}}`, `{"items":[],"links":{"next":"https://api.jobadder.com/v2/next"}}`, `{"items":[],"totalCount":10}`} {
		if _, err := countResponse([]byte(body), "definitions"); err == nil {
			t.Fatalf("incomplete/invalid definitions accepted: %s", body)
		}
	}
}
