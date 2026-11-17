package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// Observations are append-only. Results describe the latest attempt; evidence
// keeps every saved response, even when a later attempt fails or changes.
type Observation struct {
	ID      int64  `json:"id"`
	Key     string `json:"key"`
	Status  string `json:"status"`
	Count   *int64 `json:"count"`
	HTTP    int    `json:"http"`
	Digest  string `json:"sha256,omitempty"`
	Checked string `json:"checkedAt"`
}

func initObservations(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS observations(
 id INTEGER PRIMARY KEY,run_id INTEGER NOT NULL,key TEXT NOT NULL,status TEXT NOT NULL,
 count INTEGER,http INTEGER NOT NULL,sha TEXT NOT NULL,checked TEXT NOT NULL,body BLOB);
 CREATE INDEX IF NOT EXISTS observations_resource ON observations(run_id,key,id);
 INSERT INTO observations(run_id,key,status,count,http,sha,checked,body)
 SELECT run_id,key,status,count,http,sha,checked,body FROM results r
 WHERE NOT EXISTS (SELECT 1 FROM observations o WHERE o.run_id=r.run_id AND o.key=r.key);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Observations(run int64) ([]Observation, error) {
	out := []Observation{}
	rows, err := s.db.Query(`SELECT id,key,status,count,http,sha,checked FROM observations WHERE run_id=? ORDER BY id`, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o Observation
		var n sql.NullInt64
		if err := rows.Scan(&o.ID, &o.Key, &o.Status, &n, &o.HTTP, &o.Digest, &o.Checked); err != nil {
			return nil, err
		}
		if n.Valid {
			o.Count = &n.Int64
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) ObservationOriginal(id int64) ([]byte, error) {
	var body []byte
	var digest string
	if err := s.db.QueryRow(`SELECT body,sha FROM observations WHERE id=?`, id).Scan(&body, &digest); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("no saved response")
	}
	raw, err := unseal(s.key, body)
	if err != nil {
		return nil, errors.New("saved response could not be decrypted")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("saved response failed integrity check")
	}
	return raw, nil
}
