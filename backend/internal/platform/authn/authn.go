// Package authn checks who is calling: it verifies signed bearer tokens (JWT).
//
// Every token is HiringPlug's own, signed with a shared secret (`hp token`
// mints one for development). Signing in with Microsoft checks Microsoft's
// token once (platform/entra) and then issues one of these, which says how
// the person signed in (amr).
package authn

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	tenantIDRe = regexp.MustCompile(`^tnt_[0-9a-hjkmnp-tv-z]{26}$`)
	userIDRe   = regexp.MustCompile(`^usr_[0-9a-hjkmnp-tv-z]{26}$`)
)

// ErrInvalidToken covers every reason a token is rejected; the reason is in
// the wrapped error for logs, never in the response.
var ErrInvalidToken = errors.New("invalid token")

// How a session was signed in (the token's amr claim, RFC 8176 style).
const (
	MethodPassword  = "pwd"
	MethodMicrosoft = "microsoft"
)

// Claims are what a token says about the caller.
type Claims struct {
	TenantID string `json:"tid"`
	// AMR is how the session was signed in: MethodPassword or MethodMicrosoft
	// (none in tokens from before 1.46.0 and from hp token).
	AMR []string `json:"amr,omitempty"`
	jwt.RegisteredClaims
}

// Method is how the session was signed in ("" when the token does not say).
func (c Claims) Method() string {
	if len(c.AMR) == 1 && (c.AMR[0] == MethodPassword || c.AMR[0] == MethodMicrosoft) {
		return c.AMR[0]
	}
	return ""
}

// Verifier checks and issues tokens.
type Verifier struct {
	secret   []byte
	issuer   string
	audience string
	now      func() time.Time
}

// NewVerifier returns a verifier for HS256 tokens.
func NewVerifier(secret []byte, issuer, audience string) *Verifier {
	return &Verifier{secret: secret, issuer: issuer, audience: audience, now: time.Now}
}

// Verify parses and checks a token: signature, algorithm, issuer, audience,
// expiry, and the shape of the tenant and user IDs.
func (v *Verifier) Verify(token string) (Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return v.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !tenantIDRe.MatchString(c.TenantID) || !userIDRe.MatchString(c.Subject) {
		return Claims{}, fmt.Errorf("%w: malformed tid or sub", ErrInvalidToken)
	}
	return c, nil
}

// Mint issues a token for a user. Used by `hp token` and tests.
func (v *Verifier) Mint(tenantID, userID string, ttl time.Duration) (string, error) {
	return v.MintAt(tenantID, userID, v.now(), ttl)
}

// MintAt issues a token for a user, issued at now.
func (v *Verifier) MintAt(tenantID, userID string, now time.Time, ttl time.Duration) (string, error) {
	return v.MintSession(tenantID, userID, "", now, ttl)
}

// MintSession issues a sign-in token, issued at now, saying how the person
// signed in (method: MethodPassword or MethodMicrosoft).
func (v *Verifier) MintSession(tenantID, userID, method string, now time.Time, ttl time.Duration) (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	c := Claims{
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        base64.RawURLEncoding.EncodeToString(id), // ends one session at sign-out
			Subject:   userID,
			Issuer:    v.issuer,
			Audience:  jwt.ClaimStrings{v.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	if method != "" {
		c.AMR = []string{method}
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(v.secret)
}
