// Package requestid gives every request a correlation ID.
//
// The ID is returned in the X-Request-Id header and in every error body, is
// on every log line, is stored on audit entries, and is copied onto every job
// the request queues (and the jobs those jobs queue, in Go and Python). One
// ID therefore finds everything that happened because of one click.
package requestid

import (
	"context"
	"crypto/rand"
	"regexp"
)

// Header is the HTTP header carrying the ID.
const Header = "X-Request-Id"

var valid = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// New returns a fresh ID such as req_8f3k2m9q4r7t1v6w0x5y2z8a3b.
func New() string {
	b := make([]byte, 26)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "req_" + string(b)
}

// Sanitize returns id if it is safe to log and echo back, otherwise "".
func Sanitize(id string) string {
	if valid.MatchString(id) {
		return id
	}
	return ""
}

type ctxKey struct{}

// With stores id in ctx.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the request ID in ctx, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
