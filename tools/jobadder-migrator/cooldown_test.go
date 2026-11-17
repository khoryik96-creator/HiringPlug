package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestLongRetryAfterPausesAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	calls := 0
	h := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		r := response(429, `{}`)
		r.Header.Set("Retry-After", "900")
		return r, nil
	})}
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, HTTP: h, LoadRetry: s.RetryUntil, SaveRetry: s.SetRetryUntil,
		Wait: func(context.Context, time.Duration) error {
			t.Fatal("long delay should pause without sleeping")
			return nil
		}}
	before := time.Now()
	_, code, err := c.get(context.Background(), "/candidates?Limit=0")
	var cooldown *CooldownError
	if !errors.As(err, &cooldown) || code != 429 || calls != 1 || cooldown.Until.Before(before.Add(15*time.Minute)) {
		t.Fatal("long delay was shortened or retried", err, calls)
	}
	_ = s.Close()
	s, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Unlock(testPass); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRetryUntil(time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	until, err := s.RetryUntil()
	if err != nil || until.Before(cooldown.Until) {
		t.Fatal("retry deadline shortened after restart", err)
	}
	c.LoadRetry = s.RetryUntil
	c.SaveRetry = s.SetRetryUntil
	_, code, err = c.get(context.Background(), "/users/current")
	if !errors.As(err, &cooldown) || code != 0 || calls != 1 {
		t.Fatal("restarting bypassed provider cooldown", err, calls)
	}
}

func TestHTTPDateRetryAfterIsNotCapped(t *testing.T) {
	header := time.Now().Add(15 * time.Minute).UTC().Format(http.TimeFormat)
	if got := retryDelay(header, 0); got < 14*time.Minute || got > 15*time.Minute {
		t.Fatal("HTTP-date delay shortened", got)
	}
}

func TestFinalThrottledAttemptStillPreservesRetryDeadline(t *testing.T) {
	s := testStore(t)
	calls, waits := 0, 0
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, LoadRetry: s.RetryUntil, SaveRetry: s.SetRetryUntil,
		HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			r := response(429, `{}`)
			r.Header.Set("Retry-After", "7")
			return r, nil
		})},
		Wait: func(context.Context, time.Duration) error { waits++; return nil }}
	_, _, err := c.get(context.Background(), "/candidates?Limit=0")
	var cooldown *CooldownError
	if !errors.As(err, &cooldown) || calls != 4 || waits != 3 {
		t.Fatal("retry bound changed", err, calls, waits)
	}
	_, _, err = c.get(context.Background(), "/candidates?Limit=0")
	if !errors.As(err, &cooldown) || calls != 4 {
		t.Fatal("resume bypassed final retry deadline", err, calls)
	}
}

func TestPauseDuringRetryDoesNotResetProviderWait(t *testing.T) {
	s := testStore(t)
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, LoadRetry: s.RetryUntil, SaveRetry: s.SetRetryUntil,
		HTTP: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			r := response(429, `{}`)
			r.Header.Set("Retry-After", "7")
			return r, nil
		})},
		Wait: func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }}
	if _, _, err := c.get(ctx, "/candidates?Limit=0"); !errors.Is(err, context.Canceled) {
		t.Fatal("pause ignored", err)
	}
	var cooldown *CooldownError
	if _, _, err := c.get(context.Background(), "/candidates?Limit=0"); !errors.As(err, &cooldown) || calls != 1 {
		t.Fatal("pause bypassed provider wait", err, calls)
	}
}
