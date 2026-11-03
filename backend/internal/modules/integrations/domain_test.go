package integrations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestHooks(t *testing.T) {
	h, errs := NormalizeHook(Hook{URL: " https://hooks.example.com/hp ", Events: []string{"candidate.created", "candidate.created"}}, false)
	assert.Empty(t, errs)
	assert.Equal(t, "https://hooks.example.com/hp", h.URL)
	assert.Equal(t, []string{"candidate.created"}, h.Events)

	for url, want := range map[string]string{
		"http://hooks.example.com":          "must use https",
		"https://user:pw@hooks.example.com": "user name",
		"ftp://hooks.example.com":           "must be an address",
		"hooks.example.com":                 "must be an address",
	} {
		_, errs := NormalizeHook(Hook{URL: url, Events: []string{"candidate.created"}}, false)
		assert.Contains(t, errs[0].Message, want, url)
	}
	_, errs = NormalizeHook(Hook{URL: "http://localhost:8080/x", Events: []string{"candidate.created"}}, true)
	assert.Empty(t, errs, "plain HTTP when testing locally")
	_, errs = NormalizeHook(Hook{URL: "https://x.example", Events: []string{"candidate.compensation_updated"}}, false)
	assert.Equal(t, "events", errs[0].Field, "salary changes are not an event")
}

func TestSignature(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	body := []byte(`{"event":"candidate.created"}`)
	mac := hmac.New(sha256.New, []byte("whsec_x"))
	mac.Write([]byte("1800000000." + string(body)))
	assert.Equal(t, "t=1800000000,v1="+hex.EncodeToString(mac.Sum(nil)), Signature("whsec_x", at, body))
}

func TestRetries(t *testing.T) {
	now := time.Unix(0, 0)
	next, ok := NextAttempt(1, now)
	assert.True(t, ok)
	assert.Equal(t, time.Minute, next.Sub(now))
	_, ok = NextAttempt(MaxAttempts, now)
	assert.False(t, ok, "the last attempt")

	assert.True(t, IsAPIKey("hpk_abc"))
	assert.False(t, IsAPIKey("eyJhbGciOi"))
}

func TestEventsMatchTheContract(t *testing.T) {
	for _, e := range Events() {
		assert.True(t, api.WebhookEvent(e).Valid(), e)
	}
}
