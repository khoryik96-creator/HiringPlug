package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type blockedArchiveWriter struct {
	header           http.Header
	entered, release chan struct{}
	once             sync.Once
	deadline         time.Time
}

func (w *blockedArchiveWriter) Header() http.Header                { return w.header }
func (w *blockedArchiveWriter) WriteHeader(int)                    {}
func (w *blockedArchiveWriter) SetWriteDeadline(d time.Time) error { w.deadline = d; return nil }
func (w *blockedArchiveWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(b), nil
}

func TestStalledArchiveDoesNotBlockStatusOrInventory(t *testing.T) {
	a := testApp(t)
	run, _ := a.store.Start("source", false)
	n := int64(1)
	if err := a.store.SaveResult(run, Resource{Key: "a", Kind: "count"}, "counted", &n, 200, []byte(`{"totalCount":1}`)); err != nil {
		t.Fatal(err)
	}
	w := &blockedArchiveWriter{header: make(http.Header), entered: make(chan struct{}), release: make(chan struct{})}
	exported := make(chan struct{})
	go func() {
		a.originals(w, httptest.NewRequest("GET", a.origin+"/api/originals.zip", nil))
		close(exported)
	}()
	select {
	case <-w.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("export did not start")
	}
	var release sync.Once
	defer func() { release.Do(func() { close(w.release) }); <-exported }()
	if w.deadline.IsZero() {
		t.Fatal("archive write has no deadline")
	}
	status := make(chan *httptest.ResponseRecorder, 1)
	go func() { status <- call(a, "GET", "/api/status", "", true) }()
	select {
	case r := <-status:
		if r.Code != 200 {
			t.Fatal(r.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled export blocked interface")
	}
	// A later run cannot alter the already staged archive's selected run.
	if _, err := a.store.Start("source", false); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(w.release) })
}

func TestArchiveContainsEverySavedResponseVersion(t *testing.T) {
	a := testApp(t)
	run, _ := a.store.Start("source", false)
	r := Resource{Key: "a", Kind: "count"}
	first, second := []byte(`{"note":"Original alowance","salary":1234567890123456789.125}`), []byte(`{"note":"New wording"}`)
	for _, raw := range [][]byte{first, nil, second} {
		if err := a.store.SaveResult(run, r, "needs_review", nil, 200, raw); err != nil {
			t.Fatal(err)
		}
	}
	w := call(a, "GET", "/api/originals.zip", "", true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	foundFirst, foundSecond := false, false
	for _, f := range z.File {
		reader, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(reader)
		_ = reader.Close()
		if bytes.Equal(raw, first) {
			foundFirst = true
		}
		if bytes.Equal(raw, second) {
			foundSecond = true
		}
		if f.Name == "manifest.json" {
			var m struct{ Observations []Observation }
			if json.Unmarshal(raw, &m) != nil || len(m.Observations) != 3 {
				t.Fatal("attempt history missing from manifest")
			}
		}
	}
	if !foundFirst || !foundSecond {
		t.Fatal("archive discarded an observed version")
	}
}

func TestCanceledArchiveRemovesItsPrivateStagingFile(t *testing.T) {
	tmp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, tmp)
	}
	s := testStore(t)
	run, _ := s.Start("source", false)
	if err := s.SaveResult(run, Resource{Key: "a"}, "needs_review", nil, 200, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	report, _ := s.Report()
	obs, _ := s.Observations(run)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if f, err := s.prepareArchive(ctx, report, obs, false); err == nil || f != nil {
		t.Fatal("canceled archive staged successfully")
	}
	files, _ := filepath.Glob(filepath.Join(os.TempDir(), "hiringplug-ja-export-*.zip"))
	if len(files) != 0 {
		t.Fatal("staging file left after cancellation")
	}
}

func TestArchiveStagingIsRemovedOnClose(t *testing.T) {
	f, err := createArchiveFile()
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	if _, err := f.Write([]byte("synthetic private export")); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("export staging file remained on disk", err)
	}
}
