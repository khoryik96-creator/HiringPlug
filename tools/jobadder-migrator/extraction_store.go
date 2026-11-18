package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

func initExtraction(db *sql.DB) error {
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS blobs(id TEXT PRIMARY KEY,sha TEXT NOT NULL,size INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS extractions(id INTEGER PRIMARY KEY,source TEXT NOT NULL,status TEXT NOT NULL,started TEXT NOT NULL,finished TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS extract_tasks(id INTEGER PRIMARY KEY,extraction_id INTEGER NOT NULL,task_key TEXT NOT NULL,route TEXT NOT NULL,path TEXT NOT NULL,bindings BLOB NOT NULL,chain TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'pending',http INTEGER NOT NULL DEFAULT 0,attempts INTEGER NOT NULL DEFAULT 0,blob_id TEXT,rows INTEGER NOT NULL DEFAULT 0,message TEXT NOT NULL DEFAULT '',UNIQUE(extraction_id,task_key));
 CREATE INDEX IF NOT EXISTS extract_task_pending ON extract_tasks(extraction_id,status,id);
 CREATE TABLE IF NOT EXISTS extract_fetches(id INTEGER PRIMARY KEY,task_id INTEGER NOT NULL,blob_id TEXT NOT NULL,native INTEGER NOT NULL DEFAULT 0,metadata BLOB NOT NULL,observed TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS extract_fetch_task ON extract_fetches(task_id);
 CREATE INDEX IF NOT EXISTS extract_fetch_blob ON extract_fetches(blob_id);
 CREATE TABLE IF NOT EXISTS extract_records(extraction_id INTEGER NOT NULL,entity TEXT NOT NULL,source_id TEXT NOT NULL,blob_id TEXT NOT NULL,pointer TEXT NOT NULL,PRIMARY KEY(extraction_id,entity,source_id,blob_id,pointer));
 CREATE TABLE IF NOT EXISTS extract_edges(extraction_id INTEGER NOT NULL,from_entity TEXT NOT NULL,from_id TEXT NOT NULL,to_entity TEXT NOT NULL,to_id TEXT NOT NULL,relation TEXT NOT NULL,blob_id TEXT NOT NULL,pointer TEXT NOT NULL,UNIQUE(extraction_id,from_entity,from_id,to_entity,to_id,relation));
 CREATE TABLE IF NOT EXISTS extract_pages(extraction_id INTEGER NOT NULL,chain TEXT NOT NULL,digest TEXT NOT NULL,offset INTEGER NOT NULL,PRIMARY KEY(extraction_id,chain,digest));
 CREATE TABLE IF NOT EXISTS source_imports(id INTEGER PRIMARY KEY,blob_id TEXT NOT NULL,source_type TEXT NOT NULL,metadata BLOB NOT NULL,status TEXT NOT NULL,records INTEGER NOT NULL DEFAULT 0,message TEXT NOT NULL DEFAULT '',created TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS source_members(id INTEGER PRIMARY KEY,import_id INTEGER NOT NULL,blob_id TEXT NOT NULL,metadata BLOB NOT NULL,kind TEXT NOT NULL,status TEXT NOT NULL,records INTEGER NOT NULL DEFAULT 0);
 CREATE INDEX IF NOT EXISTS source_members_import ON source_members(import_id);
 CREATE TABLE IF NOT EXISTS source_rows(member_id INTEGER NOT NULL,entity TEXT NOT NULL,source_id TEXT NOT NULL,pointer TEXT NOT NULL,UNIQUE(member_id,entity,source_id,pointer));
 UPDATE source_imports SET status='needs_review',message='Original retained; intake was interrupted. Re-upload or supply a supported logical export.' WHERE status='processing';
 UPDATE source_members SET status='index_interrupted' WHERE status='processing';`)
	return err
}

type ExtractTask struct {
	PageDigest string            `json:"-"`
	ID         int64             `json:"id"`
	Export     int64             `json:"-"`
	Route      string            `json:"route"`
	Path       string            `json:"path"`
	Bindings   map[string]string `json:"-"`
	Chain      string            `json:"-"`
	Status     string            `json:"status"`
	HTTP       int               `json:"http"`
	Attempts   int               `json:"attempts"`
	BlobID     string            `json:"blobId,omitempty"`
	Rows       int64             `json:"rows"`
	Message    string            `json:"message,omitempty"`
}
type ExtractionSummary struct {
	ID        int64  `json:"id"`
	Source    string `json:"sourceFingerprint"`
	Status    string `json:"status"`
	Started   string `json:"startedAt"`
	Finished  string `json:"finishedAt,omitempty"`
	Tasks     int64  `json:"tasks"`
	Pending   int64  `json:"pending"`
	Collected int64  `json:"collected"`
	Gaps      int64  `json:"gaps"`
	Records   int64  `json:"records"`
	Files     int64  `json:"files"`
	Bytes     int64  `json:"bytes"`
}

func (s *Store) BeginExtraction(source string, resume bool) (int64, error) {
	var id int64
	var previous string
	err := s.db.QueryRow(`SELECT id,source FROM extractions ORDER BY id DESC LIMIT 1`).Scan(&id, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil && source != previous {
		return 0, errors.New("extraction workspace belongs to another source")
	}
	if resume && err == nil {
		_, err = s.db.Exec(`UPDATE extract_tasks SET status='pending' WHERE extraction_id=? AND status IN ('running','failed','denied','unavailable','waiting');UPDATE extractions SET status='running',finished='' WHERE id=?`, id, id)
		return id, err
	}
	r, err := s.db.Exec(`INSERT INTO extractions(source,status,started) VALUES(?,'running',?)`, source, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}
func taskKey(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}
func (s *Store) EnqueueTasks(tasks []ExtractTask) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO extract_tasks(extraction_id,task_key,route,path,bindings,chain) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, t := range tasks {
		raw, _ := json.Marshal(t.Bindings)
		if _, err := stmt.Exec(t.Export, taskKey(t.Path), t.Route, t.Path, raw, t.Chain); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) ClaimTask(export int64) (ExtractTask, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ExtractTask{}, err
	}
	defer tx.Rollback()
	var t ExtractTask
	var bindings []byte
	t.Export = export
	err = tx.QueryRow(`SELECT id,route,path,bindings,chain,attempts FROM extract_tasks WHERE extraction_id=? AND status='pending' ORDER BY id LIMIT 1`, export).Scan(&t.ID, &t.Route, &t.Path, &bindings, &t.Chain, &t.Attempts)
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(bindings, &t.Bindings); err != nil {
		return t, err
	}
	if _, err := tx.Exec(`UPDATE extract_tasks SET status='running',attempts=attempts+1 WHERE id=?`, t.ID); err != nil {
		return t, err
	}
	return t, tx.Commit()
}
func (s *Store) TaskResult(t ExtractTask, status string, httpCode int, message string) error {
	_, err := s.db.Exec(`UPDATE extract_tasks SET status=?,http=?,message=? WHERE id=?`, status, httpCode, message, t.ID)
	return err
}
func (s *Store) ExtractionStatus() (ExtractionSummary, error) {
	var r ExtractionSummary
	err := s.db.QueryRow(`SELECT id,source,status,started,finished FROM extractions ORDER BY id DESC LIMIT 1`).Scan(&r.ID, &r.Source, &r.Status, &r.Started, &r.Finished)
	if errors.Is(err, sql.ErrNoRows) {
		r.Status = "not_started"
		return r, nil
	}
	if err != nil {
		return r, err
	}
	err = s.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(status IN ('pending','running')),0),COALESCE(SUM(status='collected'),0),COALESCE(SUM(status NOT IN ('pending','running','collected')),0) FROM extract_tasks WHERE extraction_id=?`, r.ID).Scan(&r.Tasks, &r.Pending, &r.Collected, &r.Gaps)
	if err != nil {
		return r, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT entity,source_id FROM extract_records WHERE extraction_id=? GROUP BY entity,source_id)`, r.ID).Scan(&r.Records); err != nil {
		return r, err
	}
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(f.native),0),COALESCE(SUM(b.size),0) FROM extract_fetches f JOIN blobs b ON b.id=f.blob_id JOIN extract_tasks t ON t.id=f.task_id WHERE t.extraction_id=?`, r.ID).Scan(&r.Files, &r.Bytes); err != nil {
		return r, err
	}
	return r, nil
}
