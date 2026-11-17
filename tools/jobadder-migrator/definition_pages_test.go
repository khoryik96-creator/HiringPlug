package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func categoryPage(offset, n int) []byte {
	items := make([]map[string]int, n)
	for i := range items {
		items[i] = map[string]int{"categoryId": offset + i + 1}
	}
	raw, _ := json.Marshal(map[string]any{"items": items})
	return raw
}

func TestCategoryDiscoveryPreservesAllPagesAndCountsOnlyAfterCompletion(t *testing.T) {
	s := testStore(t)
	run, _ := s.Start("source", false)
	pages := [][]byte{categoryPage(0, 1000), categoryPage(1000, 1000), categoryPage(2000, 7)}
	calls := 0
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, Wait: wait, HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("Limit") != "1000" || r.URL.Query().Get("Offset") != strconv.Itoa(calls*1000) {
			t.Error("incorrect category pagination")
		}
		body := pages[calls]
		calls++
		return response(200, string(body)), nil
	})}}
	resource := Resource{"categories", "Categories", "/categories", "paged_definitions"}
	if err := discover(context.Background(), s, c, run, []Resource{resource}, false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	report, _ := s.Report()
	result := report.Results[0]
	if calls != 3 || result.Status != "definitions" || result.Count == nil || *result.Count != 2007 {
		t.Fatal("partial category count reported", calls, result)
	}
	observations, err := s.Observations(run)
	if err != nil || len(observations) != 3 {
		t.Fatal("category pages missing", err)
	}
	for i, o := range observations {
		raw, err := s.ObservationOriginal(o.ID)
		if err != nil || !bytes.Equal(raw, pages[i]) {
			t.Fatal("category page changed", err)
		}
		if i < 2 && (o.Count != nil || o.Status != "definition_page") {
			t.Fatal("partial page claimed as full catalogue")
		}
	}
	if err := discover(context.Background(), s, c, run, []Resource{resource}, true, func(string) {}); err != nil || calls != 3 {
		t.Fatal("completed definitions fetched again", err)
	}
}

func TestRepeatedCategoryPageStopsWithOriginalEvidence(t *testing.T) {
	s := testStore(t)
	run, _ := s.Start("source", false)
	raw := categoryPage(0, 1000)
	calls := 0
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, Wait: wait, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return response(200, string(raw)), nil })}}
	resource := Resource{"categories", "Categories", "/categories", "paged_definitions"}
	if err := discover(context.Background(), s, c, run, []Resource{resource}, false, func(string) {}); err == nil || calls != 2 {
		t.Fatal("pagination failed to stop", err, calls)
	}
	r, _ := s.Report()
	if r.Results[0].Count != nil || r.Results[0].Status != "needs_review" {
		t.Fatal("duplicate pages invented total")
	}
	obs, _ := s.Observations(run)
	if len(obs) != 2 {
		t.Fatal("repeated response evidence discarded")
	}
}
