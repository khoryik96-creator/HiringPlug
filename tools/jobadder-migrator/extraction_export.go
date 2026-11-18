package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

const exportPartBytes int64 = 256 << 20

type ObjectPart struct {
	First int64 `json:"first"`
	Last  int64 `json:"last"`
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}
type DownloadTicket struct {
	Part     ObjectPart
	Manifest bool
	Until    time.Time
}
type exportObject struct {
	Ordinal int64 `json:"ordinal"`
	Blob
}

func (s *Store) exportObjects(first, last int64) ([]exportObject, error) {
	rows, err := s.db.Query(`SELECT rowid,id,sha,size FROM blobs WHERE rowid>=? AND rowid<=? ORDER BY rowid`, first, last)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []exportObject
	for rows.Next() {
		var b exportObject
		if err := rows.Scan(&b.Ordinal, &b.ID, &b.SHA, &b.Size); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) exportParts() ([]ObjectPart, error) {
	rows, err := s.db.Query(`SELECT rowid,size FROM blobs ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parts := []ObjectPart{}
	p := ObjectPart{}
	for rows.Next() {
		var id, size int64
		if err := rows.Scan(&id, &size); err != nil {
			return nil, err
		}
		if p.Files > 0 && (p.Bytes+size > exportPartBytes || p.Files >= 1000) {
			parts = append(parts, p)
			p = ObjectPart{}
		}
		if p.First == 0 {
			p.First = id
		}
		p.Last = id
		p.Files++
		p.Bytes += size
	}
	if p.Files > 0 {
		parts = append(parts, p)
	}
	return parts, rows.Err()
}

func (s *Store) prepareObjectPart(ctx context.Context, part ObjectPart) (*os.File, error) {
	objects, err := s.exportObjects(part.First, part.Last)
	if err != nil {
		return nil, err
	}
	var size int64
	for _, b := range objects {
		size += b.Size
	}
	if int64(len(objects)) != part.Files || size != part.Bytes {
		return nil, errors.New("export part changed")
	}
	f, err := createArchiveFile()
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			f.Close()
		}
	}()
	z := zip.NewWriter(f)
	for _, b := range objects {
		header := &zip.FileHeader{Name: "objects/" + b.ID + ".bin", Method: zip.Store}
		entry, err := z.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		r, err := s.OpenBlob(b.ID)
		if err != nil {
			return nil, err
		}
		err = copyContext(ctx, entry, r)
		r.Close()
		if err != nil {
			return nil, err
		}
	}
	entry, err := z.Create("objects.json")
	if err != nil {
		return nil, err
	}
	if err = json.NewEncoder(entry).Encode(objects); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return f, nil
}

func (s *Store) prepareExtractionManifest(ctx context.Context, demo bool) (*os.File, error) {
	report, err := s.extractionReport()
	if err != nil {
		return nil, err
	}
	parts, err := s.exportParts()
	if err != nil {
		return nil, err
	}
	f, err := createArchiveFile()
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			f.Close()
		}
	}()
	z := zip.NewWriter(f)
	entry, err := z.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if err = json.NewEncoder(entry).Encode(map[string]any{"format": "hiringplug-source-extraction-v1", "demo": demo, "createdAt": time.Now().UTC(), "report": report, "parts": parts, "objects": "objects/<blob-id>.bin; original names and associations in sources/fetches metadata", "sourceValues": "Original object bytes are authoritative; indexes are pointers, not transformed records."}); err != nil {
		return nil, err
	}
	// Stream indexes and encrypted metadata decoding without holding them all in
	// RAM. No credentials, OAuth state or workspace key is included.
	queries := []struct {
		name, query string
		encrypted   int
	}{
		{"tasks.jsonl", `SELECT id,extraction_id,route,path,status,http,attempts,blob_id,rows,message FROM extract_tasks ORDER BY id`, -1},
		{"records.jsonl", `SELECT extraction_id,entity,source_id,blob_id,pointer FROM extract_records ORDER BY extraction_id,entity,source_id`, -1},
		{"relationships.jsonl", `SELECT extraction_id,from_entity,from_id,to_entity,to_id,relation,blob_id,pointer FROM extract_edges ORDER BY extraction_id`, -1},
		{"fetches.jsonl", `SELECT id,task_id,blob_id,native,metadata,observed FROM extract_fetches ORDER BY id`, 4},
		{"sources.jsonl", `SELECT id,blob_id,source_type,metadata,status,records,message,created FROM source_imports ORDER BY id`, 3},
		{"source-members.jsonl", `SELECT id,import_id,blob_id,metadata,kind,status,records FROM source_members ORDER BY id`, 3},
		{"source-rows.jsonl", `SELECT member_id,entity,source_id,pointer FROM source_rows ORDER BY member_id`, -1},
	}
	for _, q := range queries {
		entry, err := z.Create(q.name)
		if err != nil {
			return nil, err
		}
		if err := s.exportTable(ctx, entry, q.query, q.encrypted); err != nil {
			return nil, err
		}
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return f, nil
}

func (s *Store) exportTable(ctx context.Context, w io.Writer, query string, encrypted int) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range targets {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		object := map[string]any{}
		for i, key := range columns {
			value := values[i]
			if i == encrypted {
				sealed, ok := value.([]byte)
				if !ok {
					return errors.New("invalid encrypted metadata")
				}
				raw, err := unseal(s.key, sealed)
				if err != nil {
					return err
				}
				var metadata any
				if err := json.Unmarshal(raw, &metadata); err != nil {
					return err
				}
				value = metadata
			}
			object[key] = value
		}
		if err := encoder.Encode(object); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (a *App) downloadPlan(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	ok := a.unlocked && !a.running && !a.transferring && !a.connecting
	a.mu.Unlock()
	if !ok {
		failure(w, "Pause extraction and intake before preparing downloads")
		return
	}
	parts, err := a.store.exportParts()
	if err != nil {
		workspaceFailure(w)
		return
	}
	reply(w, 200, map[string]any{"parts": parts, "plan": exportPlanKey(parts)})
}
func exportPlanKey(parts []ObjectPart) string {
	raw, _ := json.Marshal(parts)
	return taskKey(string(raw))
}
func (a *App) downloadTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Manifest bool   `json:"manifest"`
		Part     int    `json:"part"`
		Plan     string `json:"plan"`
	}
	if !input(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.transferring || a.connecting {
		failure(w, "Pause extraction and intake before downloading")
		return
	}
	parts, err := a.store.exportParts()
	if err != nil {
		workspaceFailure(w)
		return
	}
	if in.Plan != "" && in.Plan != exportPlanKey(parts) {
		failure(w, "Source objects changed. Prepare a new export to keep its manifest and parts consistent.")
		return
	}
	ticket := DownloadTicket{Manifest: in.Manifest, Until: time.Now().Add(2 * time.Minute)}
	if !in.Manifest {
		if in.Part < 0 || in.Part >= len(parts) {
			failure(w, "Unknown download part")
			return
		}
		ticket.Part = parts[in.Part]
	}
	if a.tickets == nil {
		a.tickets = map[string]DownloadTicket{}
	}
	for key, t := range a.tickets {
		if time.Now().After(t.Until) {
			delete(a.tickets, key)
		}
	}
	if len(a.tickets) >= 16 {
		failure(w, "Finish the current downloads first")
		return
	}
	key := randomID()
	a.tickets[key] = ticket
	reply(w, 200, map[string]string{"url": "/download/" + key})
}
func (a *App) downloadExtraction(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	ticket, ok := a.tickets[r.PathValue("ticket")]
	delete(a.tickets, r.PathValue("ticket"))
	a.mu.Unlock()
	if !ok || time.Now().After(ticket.Until) {
		http.Error(w, "Download expired; prepare it again", 401)
		return
	}
	if !a.beginTransfer() {
		failure(w, "Pause other work before downloading")
		return
	}
	defer a.endTransfer()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	var f *os.File
	var err error
	name := "jobadder-extraction-manifest.zip"
	if ticket.Manifest {
		f, err = a.store.prepareExtractionManifest(r.Context(), a.demo)
	} else {
		f, err = a.store.prepareObjectPart(r.Context(), ticket.Part)
		name = fmt.Sprintf("jobadder-objects-%d-%d.zip", ticket.Part.First, ticket.Part.Last)
	}
	if err != nil {
		failure(w, "Archive verification or staging failed; nothing exported")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		workspaceFailure(w)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_ = copyContext(r.Context(), &downloadWriter{w, http.NewResponseController(w)}, f)
}

type downloadWriter struct {
	w http.ResponseWriter
	c *http.ResponseController
}

func (w *downloadWriter) Write(p []byte) (int, error) {
	_ = w.c.SetWriteDeadline(time.Now().Add(2 * time.Minute))
	return w.w.Write(p)
}
