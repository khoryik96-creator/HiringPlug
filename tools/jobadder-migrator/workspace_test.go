package main

import (
	"strings"
	"testing"
	"time"
)

func TestUnreadableProgressReturnsErrorAndBlocksNewRuns(t *testing.T) {
	a := testApp(t)
	a.connection.Fingerprint = "source"
	_, _ = a.store.Start("source", false)
	if _, err := a.store.db.Exec(`DROP TABLE results`); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ method, path, body string }{{"GET", "/api/status", ""}, {"POST", "/api/start", `{"resume":false}`}} {
		w := call(a, request.method, request.path, request.body, true)
		if w.Code != 503 || !strings.Contains(w.Body.String(), "Workspace progress could not be read") || strings.Contains(w.Body.String(), "no such table") {
			t.Fatal("storage failure hidden or exposed internal details", w.Code, w.Body.String())
		}
	}
	var n int
	if err := a.store.db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&n); err != nil || n != 1 {
		t.Fatal("new run created in unhealthy workspace", err)
	}
}

func TestProviderCooldownBlocksStartWithoutChangingRun(t *testing.T) {
	a := testApp(t)
	a.connection.Fingerprint = "source"
	run, _ := a.store.Start("source", false)
	if err := a.store.SetRetryUntil(time.Now().Add(15 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	w := call(a, "POST", "/api/start", `{"resume":false}`, true)
	if w.Code != 429 || a.running {
		t.Fatal("provider cooldown bypassed")
	}
	report, _ := a.store.Report()
	if report.Run != run {
		t.Fatal("cooldown replaced latest run")
	}
	w = call(a, "GET", "/api/status", "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "retryAt") {
		t.Fatal("cooldown missing from interface status")
	}
}

func TestShutdownRefusesNewHandlersBeforeStoreCloses(t *testing.T) {
	a := testApp(t)
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
	if w := call(a, "GET", "/api/status", "", true); w.Code != 503 {
		t.Fatal("closing application accepted new request")
	}
	a.work.Wait()
}
