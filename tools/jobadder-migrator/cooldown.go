package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const maxInlineRetry = 30 * time.Second

type CooldownError struct{ Until time.Time }

func (e *CooldownError) Error() string {
	return "JobAdder is throttled or unavailable. Resume no earlier than " + e.Until.UTC().Format(time.RFC3339) + "."
}

// Large values cause a durable pause, never an early retry or duration overflow.
func retryDelay(header string, attempt int) time.Duration {
	seconds, err := strconv.ParseUint(header, 10, 64)
	if err == nil || errors.Is(err, strconv.ErrRange) {
		const maxDuration = time.Duration(1<<63 - 1)
		if err != nil || seconds > uint64(maxDuration/time.Second) {
			return maxDuration
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(0, time.Until(at))
	}
	return time.Duration(1<<attempt) * time.Second
}

func (s *Store) RetryUntil() (time.Time, error) {
	var value []byte
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key='retry_until'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(value))
}
func (s *Store) SetRetryUntil(until time.Time) error {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	old, err := s.RetryUntil()
	if err != nil {
		return err
	}
	if old.After(until) {
		return nil
	}
	_, err = s.db.Exec(`INSERT INTO metadata VALUES('retry_until',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, []byte(until.UTC().Format(time.RFC3339Nano)))
	return err
}
