package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/url"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestProviderLogicalFormatsPreserveOriginalsAndUnicode(t *testing.T) {
	fixtures := map[string]string{
		"candidates.csv":   "\ufeffcandidateId,name,notes\r\n9007199254740993,李,\"line one\nline two\"\r\n7,Synthetic,alowance\r\n",
		"candidates.json":  `{"items":[{"candidateId":9007199254740993,"null":null,"empty":""},{"candidateId":7,"notes":"original"}],"unknown":"retain"}`,
		"candidates.jsonl": "{\"candidateId\":9007199254740993}\n{\"candidateId\":7}\n",
	}
	for name, raw := range fixtures {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			b, err := s.PutBlob(context.Background(), strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			id, err := s.addImport(b, name, "jobadder_backup")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ingestSource(context.Background(), id, b, name, "candidates", ""); err != nil {
				t.Fatal(err)
			}
			imports, err := s.sourceImports()
			if err != nil || len(imports) != 1 || imports[0].Records != 2 || imports[0].Status != "indexed_unverified" {
				t.Fatal(imports, err)
			}
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM source_rows WHERE source_id='9007199254740993'`).Scan(&count); err != nil || count != 1 {
				t.Fatal("source ID rounded", count, err)
			}
			r, err := s.OpenBlob(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			r.Close()
			if err != nil || string(got) != raw {
				t.Fatal("original transformed", err)
			}
		})
	}
	for _, little := range []bool{true, false} {
		s := testStore(t)
		raw := []byte{0xff, 0xfe}
		if !little {
			raw = []byte{0xfe, 0xff}
		}
		for _, unit := range utf16.Encode([]rune("candidateId,name\r\n7,李😀\r\n")) {
			p := make([]byte, 2)
			if little {
				binary.LittleEndian.PutUint16(p, unit)
			} else {
				binary.BigEndian.PutUint16(p, unit)
			}
			raw = append(raw, p...)
		}
		count, err := s.indexSourceRows(context.Background(), 1, bytes.NewReader(raw), ".csv", "candidates", "")
		if err != nil || count != 1 {
			t.Fatal("UTF-16 failed", little, count, err)
		}
	}
}

func sourceZip(t *testing.T, name, contents string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(contents))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestBackupIntakeFlagsOpaqueFormatsAndUnsafeArchives(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		raw    []byte
		status string
		fails  bool
	}{
		{"account.bak", []byte("synthetic opaque backup"), "needs_adapter", false},
		{"mailbox.pst", []byte("synthetic opaque mailbox"), "needs_adapter", false},
		{"account.zip", sourceZip(t, "nested/candidates.csv", "candidateId,name\n7,Sample\n"), "indexed_unverified", false},
		{"escape.zip", sourceZip(t, "../../outside.csv", "candidateId\n7\n"), "needs_review", true},
		{"drive.zip", sourceZip(t, "C:\\outside.csv", "candidateId\n7\n"), "needs_review", true},
		{"broken.json", []byte(`{"items":[{"candidateId":7},`), "needs_review", true},
		{"broken.csv", []byte("candidateId,name\n7\n"), "needs_review", true},
		{"duplicate.csv", []byte("candidateId,candidateId\n7,8\n"), "needs_review", true},
		{"surrogate.csv", []byte{0xff, 0xfe, 0, 0xd8}, "needs_review", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			s := testStore(t)
			b, err := s.PutBlob(context.Background(), bytes.NewReader(fixture.raw))
			if err != nil {
				t.Fatal(err)
			}
			id, err := s.addImport(b, fixture.name, "jobadder_backup")
			if err != nil {
				t.Fatal(err)
			}
			err = s.ingestSource(context.Background(), id, b, fixture.name, "candidates", "")
			if (err != nil) != fixture.fails {
				t.Fatal("wrong parse outcome", err)
			}
			imports, err := s.sourceImports()
			if err != nil || imports[0].Status != fixture.status {
				t.Fatal(imports, err)
			}
			if err := s.verifyBlob(context.Background(), b.ID); err != nil {
				t.Fatal("original lost", err)
			}
		})
	}
}

func TestProviderUploadRequiresSessionAndNeverGrantsCompleteness(t *testing.T) {
	a := testApp(t)
	path := "/api/sources?" + url.Values{"name": {"candidate export.csv"}, "source": {"jobadder_backup"}, "entity": {"candidates"}}.Encode()
	if w := call(a, "POST", path, "candidateId\n7\n", false); w.Code != 401 {
		t.Fatal("unauthenticated intake")
	}
	if w := call(a, "POST", path, "candidateId\n7\n", true); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	report, err := a.store.extractionReport()
	if err != nil || report.VerifiedComplete || report.Summary.ID != 0 || len(report.Imports) != 1 || report.Imports[0].Records != 1 {
		t.Fatal("unverified source treated as API-complete", report, err)
	}
	a.running = true
	if w := call(a, "POST", path, "candidateId\n8\n", true); w.Code != 400 {
		t.Fatal("intake raced extraction")
	}
	a.running = false
}

func TestSourceArraysStreamAcrossTransactionBoundaries(t *testing.T) {
	s := testStore(t)
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 2501; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"candidateId":7,"unknown":"unchanged"}`)
	}
	b.WriteString("]")
	count, err := s.indexSourceRows(context.Background(), 1, strings.NewReader(b.String()), ".json", "candidates", "")
	if err != nil || count != 2501 {
		t.Fatal("stream failed", count, err)
	}
	var stored int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM source_rows`).Scan(&stored); err != nil || stored != 2501 {
		t.Fatal("rows lost at batch boundary", stored, err)
	}
}
