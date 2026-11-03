package integrations

// The integration rules, as plain functions on plain values: no database,
// no HTTP.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// KeyPrefix starts every API key, so the API tells them from sign-in tokens.
const KeyPrefix = "hpk_"

// IsAPIKey reports whether a bearer token is an API key.
func IsAPIKey(token string) bool { return strings.HasPrefix(token, KeyPrefix) }

// HashKey is what is stored of an API key: its SHA-256, in hex.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// IsKeyPrincipal reports whether a principal is an API key (its user id is
// the key's id).
func IsKeyPrincipal(userID string) bool { return strings.HasPrefix(userID, "akey_") }

// Events are those a webhook may listen for (audit actions; the contract's
// WebhookEvent lists the same, which a test checks). Salary changes are not
// among them.
func Events() []string {
	return []string{
		"candidate.created", "candidate.updated", "candidate.merged",
		"application.created", "application.moved", "careers.applied",
		"interview.scheduled", "interview.updated", "interview.feedback_given",
		"offer.approved", "offer.sent", "offer.accepted", "offer.declined", "offer.withdrawn",
		"placement.recorded", "placement.updated",
		"job.created", "job.updated", "company.created", "contact.created", "webhook.ping",
	}
}

// Hook is a webhook's settings.
type Hook struct {
	URL         string
	Events      []string
	Active      bool
	Description *string
}

// NormalizeHook checks a webhook: an HTTPS address (plain HTTP only when
// private networks are allowed, for local testing), without a user name or
// password in it, and known events (each once).
func NormalizeHook(h Hook, allowPrivate bool) (Hook, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	h.URL = strings.TrimSpace(h.URL)
	u, err := url.Parse(h.URL)
	switch {
	case err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http"):
		errs.Add("url", "must be an address like https://example.com/hooks/hiringplug")
	case u.Scheme == "http" && !allowPrivate:
		errs.Add("url", "must use https")
	case u.User != nil:
		errs.Add("url", "must not contain a user name or password")
	}
	known := Events()
	var events []string
	for _, e := range h.Events {
		if !slices.Contains(known, e) {
			errs.Add("events", e+" is not an event")
			continue
		}
		if !slices.Contains(events, e) {
			events = append(events, e)
		}
	}
	if len(events) == 0 && len(errs) == 0 {
		errs.Add("events", "choose at least one event")
	}
	h.Events = events
	if h.Description != nil {
		d := strings.TrimSpace(*h.Description)
		h.Description = &d
		if d == "" {
			h.Description = nil
		}
	}
	return h, errs
}

// Signature signs a delivery: t=<unix time>,v1=<hex HMAC-SHA256 of
// "<unix time>.<body>">. Receivers check it, and that the time is recent.
func Signature(secret string, at time.Time, body []byte) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// retryAfter is how long to wait after each failed attempt; after the last
// one, the delivery fails.
var retryAfter = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 12 * time.Hour}

// MaxAttempts is how many times a delivery is tried.
var MaxAttempts = len(retryAfter) + 1

// NextAttempt is when to try again after attempt n (1-based) failed, or false
// when that was the last.
func NextAttempt(n int, now time.Time) (time.Time, bool) {
	if n < 1 || n > len(retryAfter) {
		return time.Time{}, false
	}
	return now.Add(retryAfter[n-1]), true
}

// Blocked reports whether a webhook may not be delivered to an address: the
// server's own and private networks, unless allowed (local testing).
func Blocked(ip net.IP, allowPrivate bool) bool {
	if allowPrivate {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsUnspecified() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast() ||
		netip.MustParsePrefix("100.64.0.0/10").Contains(addr) // carrier-grade NAT
}
