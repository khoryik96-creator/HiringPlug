package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"time"
)

func (s *Store) saveFetch(t ExtractTask, b Blob, native bool, metadata any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	sealed, err := seal(s.key, raw)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO extract_fetches(task_id,blob_id,native,metadata,observed) VALUES(?,?,?,?,?)`, t.ID, b.ID, native, sealed, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Checkpoint the page and all work it discovers in one transaction. The encrypted
// original was saved before this transaction; a crash cannot lose its bytes.
func (s *Store) commitPage(t ExtractTask, b Blob, index PageIndex, next *ExtractTask, rows int64, offset int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if t.PageDigest != "" && rows > 0 {
		var previous int64
		err = tx.QueryRow(`SELECT offset FROM extract_pages WHERE extraction_id=? AND chain=? AND digest=?`, t.Export, t.Chain, t.PageDigest).Scan(&previous)
		if err == nil && previous != offset {
			return errors.New("provider repeated a page; collection needs review")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO extract_pages(extraction_id,chain,digest,offset) VALUES(?,?,?,?)`, t.Export, t.Chain, t.PageDigest, offset); err != nil {
			return err
		}
	}
	if next != nil {
		index.Tasks = append(index.Tasks, *next)
	}
	record, err := tx.Prepare(`INSERT OR IGNORE INTO extract_records(extraction_id,entity,source_id,blob_id,pointer) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer record.Close()
	for _, v := range index.Records {
		if _, err = record.Exec(t.Export, v.Entity, v.ID, b.ID, v.Pointer); err != nil {
			return err
		}
	}
	edge, err := tx.Prepare(`INSERT OR IGNORE INTO extract_edges(extraction_id,from_entity,from_id,to_entity,to_id,relation,blob_id,pointer) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer edge.Close()
	for _, v := range index.Edges {
		if _, err = edge.Exec(t.Export, v.From.Entity, v.From.ID, v.To.Entity, v.To.ID, v.Relation, b.ID, v.Pointer); err != nil {
			return err
		}
	}
	queue, err := tx.Prepare(`INSERT OR IGNORE INTO extract_tasks(extraction_id,task_key,route,path,bindings,chain) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer queue.Close()
	for _, v := range index.Tasks {
		bindings, _ := json.Marshal(v.Bindings)
		if _, err = queue.Exec(t.Export, taskKey(v.Path), v.Route, v.Path, bindings, v.Chain); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE extract_tasks SET status='collected',http=200,blob_id=?,rows=?,message='' WHERE id=?`, b.ID, rows, t.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func decodePage(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, errors.New("source response is not valid JSON")
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("source response has trailing data")
	}
	if jsonDepth(v, 0) > 64 {
		return nil, errors.New("original retained; relationship nesting exceeds supported indexing depth")
	}
	return v, nil
}
func jsonDepth(v any, depth int) int {
	if depth > 64 {
		return depth
	}
	maximum := depth
	switch x := v.(type) {
	case []any:
		for _, child := range x {
			n := jsonDepth(child, depth+1)
			if n > maximum {
				maximum = n
			}
		}
	case map[string]any:
		for _, child := range x {
			n := jsonDepth(child, depth+1)
			if n > maximum {
				maximum = n
			}
		}
	}
	return maximum
}

func nextPage(t ExtractTask, r ExtractRoute, v any) (*ExtractTask, int64, int64, error) {
	if !r.Collection {
		return nil, 1, 0, nil
	}
	var items []any
	var total *int64
	hasNext := false
	switch envelope := v.(type) {
	case []any:
		items = envelope
	case map[string]any:
		a, ok := envelope["items"]
		if !ok {
			return nil, 0, 0, errors.New("collection response has no items")
		}
		if a != nil {
			items, ok = a.([]any)
			if !ok {
				return nil, 0, 0, errors.New("collection items are not a list")
			}
		}
		if n, ok := envelope["totalCount"]; ok {
			number, ok := n.(json.Number)
			if !ok {
				return nil, 0, 0, errors.New("invalid collection total")
			}
			count, err := number.Int64()
			if err != nil || count < 0 {
				return nil, 0, 0, errors.New("invalid collection total")
			}
			total = &count
		}
		if links, ok := envelope["links"].(map[string]any); ok {
			hasNext = scalarID(links["next"]) != ""
		}
	default:
		return nil, 0, 0, errors.New("invalid collection response")
	}
	u, _ := url.Parse(t.Path)
	q := u.Query()
	offset, _ := strconv.ParseInt(q.Get(r.Offset), 10, 64)
	count := int64(len(items))
	if r.Offset == "" {
		if hasNext || (total != nil && *total > count) {
			return nil, count, offset, errors.New("unpaged response is incomplete; source adapter needs review")
		}
		return nil, count, offset, nil
	}
	if count == 0 {
		if hasNext || (total != nil && offset < *total) {
			return nil, 0, offset, errors.New("empty page before reported collection end")
		}
		return nil, 0, offset, nil
	}
	if total != nil && offset+count > *total {
		return nil, count, offset, errors.New("collection changed while paging; reconcile and repeat extraction")
	}
	more := hasNext || (total != nil && offset+count < *total) || (total == nil && count >= int64(r.PageSize))
	if !more {
		return nil, count, offset, nil
	}
	q.Set(r.Offset, strconv.FormatInt(offset+count, 10))
	u.RawQuery = q.Encode()
	next := t
	next.ID = 0
	next.Path = u.String()
	return &next, count, offset, nil
}

func extract(ctx context.Context, s *Store, c *Client, id int64, routes []ExtractRoute, progress func(string)) error {
	if err := s.EnqueueTasks(seedTasks(id, routes)); err != nil {
		return err
	}
	byPath := map[string]ExtractRoute{}
	for _, r := range routes {
		byPath[r.Path] = r
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		t, err := s.ClaimTask(id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		r, ok := byPath[t.Route]
		if !ok {
			return errors.New("saved route is not in the bundled allowlist")
		}
		progress(r.Family)
		if err = extractTask(ctx, s, c, t, r, routes); err != nil {
			status, code := "failed", t.HTTP
			var cooldown *CooldownError
			var httpErr *extractHTTPError
			if errors.As(err, &httpErr) {
				code = httpErr.Code
			}
			if errors.As(err, &cooldown) {
				status = "waiting"
			}
			if code == 403 {
				status = "denied"
			}
			if code == 404 || code == 410 {
				status = "unavailable"
			}
			if e := s.TaskResult(t, status, code, err.Error()); e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if status == "waiting" || code == 401 {
				return err
			}
		}
	}
}

func extractTask(ctx context.Context, s *Store, c *Client, t ExtractTask, r ExtractRoute, routes []ExtractRoute) error {
	res, code, err := c.open(ctx, t.Path, r.Binary)
	t.HTTP = code
	if err != nil {
		if e := s.TaskResult(t, "failed", code, err.Error()); e != nil {
			return e
		}
		return &extractHTTPError{code, err}
	}
	defer res.Body.Close()
	if r.Binary {
		b, err := s.PutBlob(ctx, res.Body)
		if err != nil {
			return errors.New("native file could not be saved; retry this checkpoint")
		}
		if err = s.saveFetch(t, b, true, map[string]string{"contentType": res.Header.Get("Content-Type"), "contentDisposition": res.Header.Get("Content-Disposition")}); err != nil {
			return err
		}
		return s.commitPage(t, b, PageIndex{}, nil, 0, 0)
	}
	b, err := s.PutBlob(ctx, res.Body)
	if err != nil {
		return err
	}
	if err = s.saveFetch(t, b, false, map[string]string{"contentType": res.Header.Get("Content-Type")}); err != nil {
		return err
	}
	original, err := s.OpenBlob(b.ID)
	if err != nil {
		return err
	}
	raw, err := readLimited(original, 32<<20)
	_ = original.Close()
	if err != nil {
		return errors.New("original response retained but exceeds JSON indexing bound; use smaller pages or provider export")
	}
	v, err := decodePage(raw)
	if err != nil {
		return err
	}
	next, rows, offset, err := nextPage(t, r, v)
	if err != nil {
		return err
	}
	if r.Collection {
		items := v
		if object, ok := v.(map[string]any); ok {
			items = object["items"]
		}
		canonical, _ := json.Marshal(items)
		t.PageDigest = taskKey(string(canonical))
	}
	return s.commitPage(t, b, indexPage(t, r, v, routes), next, rows, offset)
}

type extractHTTPError struct {
	Code int
	Err  error
}

func (e *extractHTTPError) Error() string { return e.Err.Error() }
func (e *extractHTTPError) Unwrap() error { return e.Err }
