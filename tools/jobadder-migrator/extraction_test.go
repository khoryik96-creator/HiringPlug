package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func routesFor(paths ...string) []ExtractRoute {
	var out []ExtractRoute
	for _, r := range extractionRoutes() {
		for _, p := range paths {
			if r.Path == p {
				out = append(out, r)
			}
		}
	}
	return out
}
func clientFor(h *http.Client) *Client {
	return &Client{Config: Connection{API: "https://api.jobadder.com/v2", AccessToken: "private-test"}, HTTP: h, Wait: func(context.Context, time.Duration) error { return nil }}
}

func TestFullExtractionPreservesNativeBytesLinksAndExactIDs(t *testing.T) {
	s := testStore(t)
	id, err := s.BeginExtraction("synthetic-account", false)
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("source write")
		}
		if strings.Contains(r.URL.Path, "/attachments/9007") && r.Header.Get("Accept") != "" {
			t.Fatal("native conversion requested")
		}
		return demoExtraction(r)
	})
	routes := routesFor("/candidates", "/candidates/{candidateId}", "/candidates/attachments", "/candidates/{candidateId}/attachments/{attach}", "/notes", "/notes/{noteId}", "/notes/{noteId}/attachments/{attachmentId}", "/folders", "/folders/{folderId}", "/opportunities")
	if err := extract(context.Background(), s, clientFor(&http.Client{Transport: transport}), id, routes, func(string) {}); err != nil {
		t.Fatal(err)
	}
	report, err := s.extractionReport()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Scope) != 30 || len(report.Routes) != 240 || report.VerifiedComplete || report.Summary.Files != 2 || report.Summary.Pending != 0 {
		rows, _ := s.db.Query(`SELECT path,status,message FROM extract_tasks`)
		for rows.Next() {
			var path, status, message string
			rows.Scan(&path, &status, &message)
			t.Log(path, status, message)
		}
		rows.Close()
		t.Fatalf("wrong extraction summary: %+v", report.Summary)
	}
	var status string
	var code int
	if err := s.db.QueryRow(`SELECT status,http FROM extract_tasks WHERE extraction_id=? AND route='/opportunities'`, id).Scan(&status, &code); err != nil || status != "denied" || code != 403 {
		t.Fatal("access gap lost", status, code, err)
	}
	var blob string
	if err := s.db.QueryRow(`SELECT blob_id FROM extract_tasks WHERE path='/candidates/7/attachments/9007199254740993'`).Scan(&blob); err != nil {
		t.Fatal("integer ID rounded or file skipped", err)
	}
	r, err := s.OpenBlob(blob)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(r)
	r.Close()
	if err != nil || !bytes.Contains(raw, []byte("Synthetic original document.\r\n")) {
		t.Fatal("native bytes changed", err)
	}
	var edges int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM extract_edges WHERE from_entity='notes' AND to_entity='candidates' AND to_id='7'`).Scan(&edges); err != nil || edges == 0 {
		t.Fatal("note links missing", edges, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM extract_edges WHERE from_entity='folders' AND to_entity='candidates' AND to_id='7'`).Scan(&edges); err != nil || edges == 0 {
		t.Fatal("folder membership missing", edges, err)
	}
	// Resume must not fetch any successful checkpoint again.
	if _, err := s.BeginExtraction("synthetic-account", true); err != nil {
		t.Fatal(err)
	}
	var pending int
	s.db.QueryRow(`SELECT COUNT(*) FROM extract_tasks WHERE status='pending' AND route!='/opportunities'`).Scan(&pending)
	if pending != 0 {
		t.Fatal("successful work reset", pending)
	}
	if _, err := s.BeginExtraction("different-source", true); err == nil {
		t.Fatal("account mixing accepted")
	}
}

func TestPagedExtractionDetectsRepeatsEvenWhenLinksChange(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "repeat"}[repeat], func(t *testing.T) {
			s := testStore(t)
			id, _ := s.BeginExtraction("source", false)
			routes := routesFor("/candidates")
			routes[0].PageSize = 2
			calls := 0
			c := clientFor(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				offset, _ := url.ParseQuery(r.URL.RawQuery)
				n := offset.Get("Offset")
				if n == "0" || repeat {
					return response(200, `{"items":[{"candidateId":1},{"candidateId":2}],"totalCount":4,"links":{"next":"https://evil.example/`+n+`"}}`), nil
				}
				return response(200, `{"items":[{"candidateId":3},{"candidateId":4}],"totalCount":4}`), nil
			})})
			if err := extract(context.Background(), s, c, id, routes, func(string) {}); err != nil {
				t.Fatal(err)
			}
			summary, err := s.ExtractionStatus()
			if err != nil {
				t.Fatal(err)
			}
			if repeat {
				if summary.Gaps != 1 || calls != 2 {
					t.Fatal("repeated page not stopped", summary, calls)
				}
			} else if summary.Records != 4 || summary.Gaps != 0 || calls != 2 {
				t.Fatal("page lost", summary, calls)
			}
		})
	}
}

func TestCancellationAndCooldownKeepResumableTasks(t *testing.T) {
	s := testStore(t)
	id, _ := s.BeginExtraction("source", false)
	ctx, cancel := context.WithCancel(context.Background())
	c := clientFor(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })})
	if err := extract(ctx, s, c, id, routesFor("/candidates"), func(string) {}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.BeginExtraction("source", true); err != nil {
		t.Fatal(err)
	}
	task, err := s.ClaimTask(id)
	if err != nil || task.Route != "/candidates" {
		t.Fatal("checkpoint not retryable", task, err)
	}
	if err := s.TaskResult(task, "pending", 0, ""); err != nil {
		t.Fatal(err)
	}
	c = clientFor(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		res := response(429, "")
		res.Header.Set("Retry-After", "900")
		return res, nil
	})})
	c.SaveRetry = s.SetRetryUntil
	var cooldown *CooldownError
	if err := extract(context.Background(), s, c, id, routesFor("/candidates"), func(string) {}); !errors.As(err, &cooldown) {
		t.Fatal("cooldown lost", err)
	}
	until, _ := s.RetryUntil()
	if time.Until(until) < 14*time.Minute {
		t.Fatal("provider wait shortened")
	}
}

func TestIdentifierScopesDoNotCrossRecordsOrEntityTypes(t *testing.T) {
	routes := routesFor("/candidates/{candidateId}/attachments/{attach}", "/companies/fields/custom/{fieldId}", "/candidates/fields/custom/{fieldId}")
	task := ExtractTask{Export: 1}
	route := ExtractRoute{Root: "candidates"}
	v, _ := decodePage([]byte(`{"items":[{"candidateId":7,"attachments":[{"attachmentId":99}]},{"candidateId":8,"custom":[{"fieldId":3}]}]}`))
	index := indexPage(task, route, v, routes)
	paths := map[string]bool{}
	for _, t := range index.Tasks {
		paths[t.Path] = true
	}
	if !paths["/candidates/7/attachments/99"] || paths["/candidates/8/attachments/99"] || paths["/companies/fields/custom/3"] || !paths["/candidates/fields/custom/3"] {
		t.Fatal("identifier cross-product or namespace mix", paths)
	}
	for _, bad := range []string{"0", "-1", "1/../../users", "1?Limit=0", "9007199254740993.0"} {
		if _, err := makeTask(1, routes[0], map[string]string{"candidateId": "7", "attach": bad}); err == nil {
			t.Fatal("invalid identifier accepted", bad)
		}
	}
}

func TestEncryptedChunkedObjectsRejectTamperAndTruncation(t *testing.T) {
	s := testStore(t)
	raw := bytes.Repeat([]byte("synthetic-private-native\r\n"), 500000)
	b, err := s.PutBlob(context.Background(), bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(s.dir, "objects", b.ID+".enc")
	encrypted, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("synthetic-private-native")) {
		t.Fatal("plaintext object on disk")
	}
	r, err := s.OpenBlob(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	r.Close()
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("large native roundtrip", err)
	}
	for _, change := range []func([]byte) []byte{func(v []byte) []byte { v[len(v)-1] ^= 1; return v }, func(v []byte) []byte { return v[:len(v)-1] }, func(v []byte) []byte { return append(v, 1) }} {
		changed := change(append([]byte(nil), encrypted...))
		if err := os.WriteFile(f, changed, 0600); err != nil {
			t.Fatal(err)
		}
		r, err := s.OpenBlob(b.ID)
		if err == nil {
			_, err = io.Copy(io.Discard, r)
			r.Close()
		}
		if err == nil {
			t.Fatal("corrupted object accepted")
		}
	}
}

func TestNativeRedirectsStripCredentialsAndRejectUnknownHosts(t *testing.T) {
	for _, location := range []string{"https://sample.s3.ap-southeast-2.amazonaws.com/file?signature=test", "https://127.0.0.1/private", "http://sample.s3.amazonaws.com/file", "https://evil.example/file"} {
		visited := false
		c := clientFor(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host == "api.jobadder.com" {
				res := response(302, "")
				res.Header.Set("Location", location)
				return res, nil
			}
			visited = true
			if r.Header.Get("Authorization") != "" || r.Header.Get("Accept") != "" || r.Header.Get("Cookie") != "" {
				t.Error("credentials or conversion Accept leaked")
			}
			return response(200, "native"), nil
		})})
		res, _, err := c.open(context.Background(), "/candidates/7/attachments/99", true)
		allowed := strings.Contains(location, "sample.s3.ap-")
		if allowed {
			if err != nil || !visited {
				t.Fatal("native redirect failed", err)
			}
			res.Body.Close()
		} else if err == nil || visited {
			t.Fatal("unsafe redirect followed", location)
		}
	}
}

func TestSourceArchiveManifestAndTicketAreCompleteAndSingleUse(t *testing.T) {
	a := testApp(t)
	b, err := a.store.PutBlob(context.Background(), strings.NewReader("candidateId,name\r\n7,Synthetic\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := a.store.addImport(b, "source.csv", "jobadder_backup")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.ingestSource(context.Background(), id, b, "source.csv", "candidates", ""); err != nil {
		t.Fatal(err)
	}
	w := call(a, "POST", "/api/download-ticket", `{"manifest":true,"part":0}`, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var ticket struct{ URL string }
	if err := json.Unmarshal(w.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	w = call(a, "GET", ticket.URL, "", false)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	for _, name := range []string{"manifest.json", "sources.jsonl", "source-rows.jsonl", "fetches.jsonl", "relationships.jsonl", "records.jsonl"} {
		if !names[name] {
			t.Fatal("manifest missing", name)
		}
	}
	if w := call(a, "GET", ticket.URL, "", false); w.Code != 401 {
		t.Fatal("ticket replay accepted")
	}
	parts, err := a.store.exportParts()
	if err != nil || len(parts) != 1 {
		t.Fatal(parts, err)
	}
	f, err := a.store.prepareObjectPart(context.Background(), parts[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, _ := f.Stat()
	objects, err := zip.NewReader(f, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.File) != 2 {
		t.Fatal("object/manifest missing")
	}
}
