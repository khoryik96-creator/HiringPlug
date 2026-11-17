package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	lock *flock.Flock
	db   *sql.DB
	key  []byte
}
type Result struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Count   *int64 `json:"count"`
	HTTP    int    `json:"http"`
	Digest  string `json:"sha256,omitempty"`
	Checked string `json:"checkedAt,omitempty"`
}
type Report struct {
	Run      int64    `json:"run"`
	Source   string   `json:"sourceFingerprint"`
	Started  string   `json:"startedAt"`
	Status   string   `json:"status"`
	Results  []Result `json:"results"`
	Coverage []string `json:"coverage"`
}

func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(dir, "workspace.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		return nil, errors.New("workspace is already open or could not be locked")
	}
	success := false
	defer func() {
		if !success {
			_ = lock.Unlock()
		}
	}()
	file := filepath.Join(dir, "migration.sqlite")
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	db, err := sql.Open("sqlite", file)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS runs(id INTEGER PRIMARY KEY,source TEXT NOT NULL,started TEXT NOT NULL,status TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS results(run_id INTEGER NOT NULL,key TEXT NOT NULL,label TEXT NOT NULL,kind TEXT NOT NULL,status TEXT NOT NULL,count INTEGER,http INTEGER NOT NULL,sha TEXT NOT NULL,checked TEXT NOT NULL,body BLOB,PRIMARY KEY(run_id,key));`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	success = true
	return &Store{db: db, lock: lock}, nil
}
func (s *Store) Close() error { err := s.db.Close(); _ = s.lock.Unlock(); return err }
func (s *Store) Unlock(pass string) error {
	var salt []byte
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key='salt'`).Scan(&salt)
	if errors.Is(err, sql.ErrNoRows) {
		salt = make([]byte, 16)
		if _, err = rand.Read(salt); err != nil {
			return err
		}
		_, err = s.db.Exec(`INSERT INTO metadata VALUES('salt',?)`, salt)
	}
	if err != nil {
		return err
	}
	key, err := derive(pass, salt)
	if err != nil {
		return err
	}
	var check []byte
	err = s.db.QueryRow(`SELECT value FROM metadata WHERE key='check'`).Scan(&check)
	if errors.Is(err, sql.ErrNoRows) {
		check, err = seal(key, []byte("workspace verified"))
		if err == nil {
			_, err = s.db.Exec(`INSERT INTO metadata VALUES('check',?)`, check)
		}
	} else if err == nil {
		var plain []byte
		plain, err = unseal(key, check)
		if err == nil && !bytes.Equal(plain, []byte("workspace verified")) {
			err = errors.New("invalid key")
		}
	}
	if err != nil {
		return errors.New("workspace could not be unlocked; check the passphrase")
	}
	s.key = key
	return nil
}
func (s *Store) SaveConfig(c Connection) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	body, err := seal(s.key, raw)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO metadata VALUES('connection',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, body)
	return err
}
func (s *Store) Config() (Connection, error) {
	var c Connection
	var body []byte
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key='connection'`).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	raw, err := unseal(s.key, body)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}
func (s *Store) Start(source string, resume bool) (int64, error) {
	var id int64
	var previous string
	err := s.db.QueryRow(`SELECT id,source FROM runs ORDER BY id DESC LIMIT 1`).Scan(&id, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil && previous != source {
		return 0, errors.New("workspace belongs to another source identity; use a separate workspace")
	}
	if resume && err == nil {
		_, err = s.db.Exec(`UPDATE runs SET status='running' WHERE id=?`, id)
		return id, err
	}
	r, err := s.db.Exec(`INSERT INTO runs(source,started,status) VALUES(?,?,'running')`, source, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}
func (s *Store) SaveResult(run int64, r Resource, status string, count *int64, httpCode int, raw []byte) error {
	var body []byte
	digest := ""
	if raw != nil {
		sum := sha256.Sum256(raw)
		digest = hex.EncodeToString(sum[:])
		var err error
		body, err = seal(s.key, raw)
		if err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT INTO results VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(run_id,key) DO UPDATE SET status=excluded.status,count=excluded.count,http=excluded.http,sha=excluded.sha,checked=excluded.checked,body=excluded.body`, run, r.Key, r.Label, r.Kind, status, count, httpCode, digest, time.Now().UTC().Format(time.RFC3339), body)
	return err
}
func (s *Store) Finish(run int64, status string) error {
	_, err := s.db.Exec(`UPDATE runs SET status=? WHERE id=?`, status, run)
	return err
}
func (s *Store) Report() (Report, error) {
	out := Report{Results: []Result{}, Coverage: Coverage}
	err := s.db.QueryRow(`SELECT id,source,started,status FROM runs ORDER BY id DESC LIMIT 1`).Scan(&out.Run, &out.Source, &out.Started, &out.Status)
	if errors.Is(err, sql.ErrNoRows) {
		out.Status = "not_started"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	rows, err := s.db.Query(`SELECT key,label,kind,status,count,http,sha,checked FROM results WHERE run_id=? ORDER BY key`, out.Run)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Result
		var n sql.NullInt64
		if err = rows.Scan(&r.Key, &r.Label, &r.Kind, &r.Status, &n, &r.HTTP, &r.Digest, &r.Checked); err != nil {
			return out, err
		}
		if n.Valid {
			r.Count = &n.Int64
		}
		out.Results = append(out.Results, r)
	}
	return out, rows.Err()
}
func (s *Store) Original(run int64, key string) ([]byte, error) {
	var raw []byte
	var digest string
	err := s.db.QueryRow(`SELECT body,sha FROM results WHERE run_id=? AND key=?`, run, key).Scan(&raw, &digest)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("no saved response")
	}
	plain, err := unseal(s.key, raw)
	if err != nil {
		return nil, errors.New("saved response could not be decrypted")
	}
	sum := sha256.Sum256(plain)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("saved response failed integrity check")
	}
	return plain, nil
}
