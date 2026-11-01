// Package authn checks who is calling: it verifies signed bearer tokens (JWT).
//
// Development and tests use tokens signed with a shared secret (`hp token`
// mints one). For production single sign-on (Google, Microsoft), put an OIDC
// provider in front and swap Verify for JWKS-based verification; the rest of
// the app only sees the Claims.
package authn

import (
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

// Claims are what a token says about the caller.
type Claims struct {
	TenantID string `json:"tid"`
	jwt.RegisteredClaims
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
	now := v.now()
	c := Claims{
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    v.issuer,
			Audience:  jwt.ClaimStrings{v.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(v.secret)
}
