package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func TestAPIAddressRestriction(t *testing.T) {
	for _, address := range []string{"http://api.jobadder.com/v2", "https://evil.example/v2", "https://api.jobadder.com.evil.example/v2", "https://api.jobadder.com@evil.example/v2", "https://api.jobadder.com:4433/v2", "https://api.jobadder.com/v2?access_token=private", "https://api.jobadder.com/v2/../other"} {
		if validateAPI(address) == nil {
			t.Errorf("unsafe URL allowed: %s", address)
		}
	}
	for _, address := range []string{"https://api.jobadder.com/v2", "https://api.au1.jobadder.com/v2/"} {
		if err := validateAPI(address); err != nil {
			t.Fatal(err)
		}
	}
}
func TestGetNeverFollowsRedirects(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	c := &Client{Config: Connection{API: source.URL, AccessToken: "private"}, HTTP: secureHTTP(), Wait: func(context.Context, time.Duration) error { return nil }}
	_, _, err := c.get(context.Background(), "/candidates")
	if err == nil || leaked {
		t.Fatal("redirect leaked source authorization")
	}
}
func TestRateLimitAndCancellation(t *testing.T) {
	calls := 0
	var delay time.Duration
	c := &Client{Config: Connection{API: "https://api.jobadder.com/v2"}, HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			r := response(429, `{"private":"not logged"}`)
			r.Header.Set("Retry-After", "7")
			return r, nil
		}
		return response(200, `{"totalCount":0}`), nil
	})}, Wait: func(_ context.Context, d time.Duration) error { delay = d; return nil }}
	if _, _, err := c.get(context.Background(), "/candidates"); err != nil || delay != 7*time.Second || calls != 2 {
		t.Fatal("rate limit ignored", err, delay, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Pace = time.Second
	c.Wait = wait
	if _, _, err := c.get(ctx, "/candidates"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel did not stop wait")
	}
}
func TestTokenExchangeDoesNotExposeProviderErrors(t *testing.T) {
	h := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return response(400, `{"error":"private-secret-or-salary"}`), nil
	})}
	_, err := tokenExchange(context.Background(), h, tokenEndpoint, nil)
	if err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("provider error leaked")
	}
}
func TestResponseSizeIsNotSilentlyTruncated(t *testing.T) {
	if _, err := readLimited(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("truncated source accepted")
	}
}
func TestRetryAfterRetainsDelayWithoutDurationOverflow(t *testing.T) {
	if got := retryDelay("900", 0); got != 15*time.Minute {
		t.Fatal("provider delay shortened", got)
	}
	for _, header := range []string{"9999999999999999", "99999999999999999999999999999999999"} {
		if got := retryDelay(header, 0); got <= 15*time.Minute {
			t.Fatal("large retry interval overflowed", got)
		}
	}
}
