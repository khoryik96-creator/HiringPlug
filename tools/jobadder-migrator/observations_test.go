package main

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestFailedRetryPreservesOriginalAndAllObservedVersions(t *testing.T) {
	s := testStore(t)
	run, _ := s.Start("source", false)
	r := Resource{"candidate", "Candidate", "/candidates", "count"}
	first := []byte(`{"items":[{"note":"Original alowance","amount":9007199254740993.125,"empty":null}]}`)
	second := []byte(`{"items":[{"note":"Changed source wording","amount":1}]}`)
	body, status := first, 200
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, Wait: wait, HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return response(status, string(body)), nil })}}
	if err := discover(context.Background(), s, c, run, []Resource{r}, false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Report()
	status = 403
	body = nil
	if err := discover(context.Background(), s, c, run, []Resource{r}, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Original(run, r.Key)
	if err != nil || !bytes.Equal(raw, first) {
		t.Fatal("failed retry erased original", err)
	}
	after, _ := s.Report()
	result := after.Results[0]
	if result.Status != "denied" || result.Count != nil || result.HTTP != 403 || result.Digest != before.Results[0].Digest || result.EvidenceChecked != before.Results[0].EvidenceChecked {
		t.Fatal("attempt status misrepresents retained evidence")
	}
	body = second
	status = 200
	if err := discover(context.Background(), s, c, run, []Resource{r}, true, func(string) {}); err != nil {
		t.Fatal(err)
	}
	observations, err := s.Observations(run)
	if err != nil || len(observations) != 3 {
		t.Fatal("attempt history incomplete", err)
	}
	for i, want := range map[int][]byte{0: first, 2: second} {
		got, err := s.ObservationOriginal(observations[i].ID)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("observed version changed", err)
		}
	}
	if observations[1].Digest != "" || observations[1].HTTP != 403 {
		t.Fatal("failed attempt invented evidence")
	}
}

func TestExistingWorkspaceEvidenceIsBackfilledOnce(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	run, _ := s.Start("source", false)
	raw := []byte(`{"totalCount":1,"wording":"Original"}`)
	n := int64(1)
	if err := s.SaveResult(run, Resource{Key: "candidates", Kind: "count"}, "counted", &n, 200, raw); err != nil {
		t.Fatal(err)
	}
	// Simulate the first release's schema, whose encrypted evidence is in results.
	if _, err := s.db.Exec(`DROP TABLE observations`); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	for i := 0; i < 2; i++ {
		s, err = OpenStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Unlock(testPass); err != nil {
			t.Fatal(err)
		}
		obs, err := s.Observations(run)
		if err != nil || len(obs) != 1 {
			t.Fatal("legacy evidence lost or duplicated", err)
		}
		got, err := s.ObservationOriginal(obs[0].ID)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("legacy payload changed", err)
		}
		_ = s.Close()
	}
}

func TestObservationAndLatestResultCommitAtomically(t *testing.T) {
	s := testStore(t)
	run, _ := s.Start("source", false)
	_, err := s.db.Exec(`CREATE TRIGGER reject_result BEFORE INSERT ON results BEGIN SELECT RAISE(ABORT,'synthetic checkpoint failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveResult(run, Resource{Key: "a"}, "needs_review", nil, 200, []byte(`{}`)); err == nil {
		t.Fatal("failed checkpoint accepted")
	}
	obs, err := s.Observations(run)
	if err != nil || len(obs) != 0 {
		t.Fatal("partial observation committed", err)
	}
}
