package entra

import (
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

// Why a token is refused. ErrInvalid covers everything that is not about
// whose account it is; the wrapped error says more, for the logs only.
var (
	ErrInvalid           = errors.New("entra: the ID token is not valid")
	ErrPersonalAccount   = errors.New("entra: a personal Microsoft account")
	ErrOtherOrganisation = errors.New("entra: an account of another organisation")
	// ErrGuestAccount: a guest in the organisation (acct 1, or a guest's
	// UPN, name_home.domain#EXT#@...): an account of another organisation,
	// invited in.
	ErrGuestAccount = errors.New("entra: a guest account")
)

const (
	// leeway allows for clocks a little apart.
	leeway = 2 * time.Minute
	// keysFor is how long Microsoft's published keys are kept.
	keysFor = 24 * time.Hour
	// refetchAfter: a key ID still unknown after a fetch fetches again at most this often.
	refetchAfter = time.Minute
	// maxMissed: after this many unknown key IDs, only refetchAfter allows another fetch.
	maxMissed = 5
	// maxOrganisations bounds the key cache.
	maxOrganisations = 1000
)

// Expect is what the token must say.
type Expect struct {
	ClientID     string // the app's client ID: the audience
	Nonce        string // sent when the sign-in started
	Organisation string // the company's organisation; "" accepts any (but never personal accounts)
}

// Identity is the Microsoft account a valid token is for.
type Identity struct {
	TenantID, ObjectID string // organisation and object ID (GUIDs, lower case)
	Email, Username    string // lower case; either may be empty
	// EmailVerified: Microsoft says the email's domain is one the
	// organisation verified (the optional claim xms_edov). Without it the
	// email is whatever an administrator typed, and proves nothing.
	EmailVerified bool
	// Member: Microsoft says the account is a member of the organisation,
	// not a guest (the optional claim acct, 0). False when the token has no
	// acct claim: a guest then cannot be told apart, since a guest's
	// preferred_username is its home address (the #EXT# name is in upn, an
	// optional claim too).
	Member bool
	Name   string
}

// Addresses are the addresses a team member may be recognised by: the
// sign-in name (preferred_username: the UPN, in a domain the organisation
// verified), and the email only when its domain is verified too. None
// unless Microsoft says the account is a member (acct 0): a guest's
// preferred_username is its home address, which may be anyone's. Lower
// case, each once.
func (i Identity) Addresses() []string {
	if !i.Member {
		return nil
	}
	var out []string
	add := func(e string) {
		if strings.Contains(e, "@") && !isGuestName(e) && (len(out) == 0 || out[0] != e) {
			out = append(out, e)
		}
	}
	add(i.Username)
	if i.EmailVerified {
		add(i.Email)
	}
	return out
}

// isGuestName reports whether a sign-in name is a guest's (an account of
// another organisation invited in: name_home.domain#EXT#@org.onmicrosoft.com).
func isGuestName(name string) bool { return strings.Contains(strings.ToLower(name), "#ext#") }

// flag reads a claim Microsoft may send as true, "true", 1 or "1".
type flag bool

func (f *flag) UnmarshalJSON(b []byte) error {
	switch strings.Trim(strings.ToLower(string(b)), `"`) {
	case "true", "1":
		*f = true
	default:
		*f = false
	}
	return nil
}

type idClaims struct {
	TenantID string `json:"tid"`
	ObjectID string `json:"oid"`
	Nonce    string `json:"nonce"`
	Email    string `json:"email"`
	Username string `json:"preferred_username"`
	Name     string `json:"name"`
	// Optional claims (Entra › Token configuration): the email's domain is
	// verified, the account is a member (0) or a guest (1; nil: not sent),
	// and its UPN (a guest's has #EXT#).
	EmailVerified flag   `json:"xms_edov"`
	Account       *flag  `json:"acct"`
	UPN           string `json:"upn"`
	jwt.RegisteredClaims
}

// Validate checks an ID token from Microsoft and returns whose it is.
func (c *Client) Validate(ctx context.Context, raw string, want Expect) (Identity, error) {
	if want.ClientID == "" || want.Nonce == "" {
		return Identity{}, fmt.Errorf("%w: nothing to compare with", ErrInvalid)
	}
	var claims idClaims
	var refused error // a personal account or another organisation, said by the token itself
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithAudience(want.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(leeway),
		jwt.WithTimeFunc(c.now),
	)
	_, err := parser.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		tid := NormalizeID(claims.TenantID)
		switch {
		case tid == ConsumersTenant:
			refused = ErrPersonalAccount // refusing needs no signature
			return nil, refused
		case !IsID(tid):
			return nil, errors.New("no organisation")
		case want.Organisation != "" && tid != want.Organisation:
			refused = ErrOtherOrganisation
			return nil, refused
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("no key ID")
		}
		return c.key(ctx, tid, kid)
	})
	if refused != nil {
		return Identity{}, refused
	}
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	tid, oid := NormalizeID(claims.TenantID), NormalizeID(claims.ObjectID)
	switch {
	case claims.Issuer != Issuer(tid):
		return Identity{}, fmt.Errorf("%w: issuer is not the token's organisation", ErrInvalid)
	case subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(want.Nonce)) != 1:
		return Identity{}, fmt.Errorf("%w: wrong nonce", ErrInvalid)
	case !IsID(oid):
		return Identity{}, fmt.Errorf("%w: no object ID", ErrInvalid)
	case len(claims.Audience) != 1:
		return Identity{}, fmt.Errorf("%w: several audiences", ErrInvalid)
	case (claims.Account != nil && bool(*claims.Account)) || isGuestName(claims.UPN) || isGuestName(claims.Username) || isGuestName(claims.Email):
		return Identity{}, ErrGuestAccount
	}
	return Identity{
		TenantID: tid, ObjectID: oid, Name: strings.TrimSpace(claims.Name),
		Email: strings.ToLower(strings.TrimSpace(claims.Email)), Username: strings.ToLower(strings.TrimSpace(claims.Username)),
		EmailVerified: bool(claims.EmailVerified), Member: claims.Account != nil,
	}, nil
}

type keySet struct {
	keys    map[string]*rsa.PublicKey
	fetched time.Time
	// missed are the unknown key IDs looked for in a run of fetches less
	// than refetchAfter apart: kept across those fetches, so key IDs made up
	// one after another cannot make every token fetch the keys again.
	missed map[string]bool
}

var errUnknownKey = errors.New("unknown key")

// key returns an organisation's signing key kid: from the cache, or fetched
// again when the keys are a day old, or kid is new to them (Microsoft
// rotates its keys). Unknown key IDs fetch the keys again at most maxMissed
// times in a row, then once refetchAfter has passed. The keys are fetched
// without holding the lock (other organisations are never kept waiting),
// once at a time per organisation.
func (c *Client) key(ctx context.Context, tid, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	now := c.now()
	set, ok := c.keys[tid]
	seen := set.fetched // the keys looked at (zero: none)
	if ok && now.Sub(set.fetched) < keysFor {
		if k := set.keys[kid]; k != nil {
			c.mu.Unlock()
			return k, nil
		}
		if now.Sub(set.fetched) < refetchAfter && (set.missed[kid] || len(set.missed) >= maxMissed) {
			c.mu.Unlock()
			return nil, errUnknownKey
		}
	}
	c.mu.Unlock()
	// Whoever asks first fetches; the others wait for that answer, each as
	// long as its own request lasts. The fetch is not cancelled with the
	// first asker (the HTTP client's timeout bounds it).
	flight := c.fetching.DoChan(tid, func() (any, error) {
		c.mu.Lock()
		if cur, had := c.keys[tid]; had && cur.fetched.After(seen) {
			// Fetched by another request since these were looked at (it
			// finished just before this one asked): no need to fetch again.
			c.mu.Unlock()
			return cur.keys, nil
		}
		c.mu.Unlock()
		keys, err := c.fetchKeys(context.WithoutCancel(ctx), tid)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		at := c.now()
		missed := map[string]bool{}
		if old, had := c.keys[tid]; had && at.Sub(old.fetched) < refetchAfter {
			missed = old.missed
		}
		if len(c.keys) >= maxOrganisations {
			c.keys = map[string]keySet{}
		}
		c.keys[tid] = keySet{keys: keys, fetched: at, missed: missed}
		return keys, nil
	})
	var got singleflight.Result
	select {
	case got = <-flight:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if got.Err != nil {
		return nil, got.Err
	}
	if k := got.Val.(map[string]*rsa.PublicKey)[kid]; k != nil {
		return k, nil
	}
	c.mu.Lock()
	if set, ok := c.keys[tid]; ok {
		set.missed[kid] = true
	}
	c.mu.Unlock()
	return nil, errUnknownKey
}

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (c *Client) fetchKeys(ctx context.Context, tid string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.login+"/"+tid+"/discovery/v2.0/keys", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("entra: the keys could not be fetched")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxAnswer))
		return nil, fmt.Errorf("entra: the keys service answered %d", res.StatusCode)
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(&doc); err != nil {
		return nil, errors.New("entra: the keys could not be read")
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") || k.Kid == "" {
			continue
		}
		if pub := rsaKey(k.N, k.E); pub != nil {
			out[k.Kid] = pub
		}
	}
	return out, nil
}

// rsaKey builds a public key of at least 2048 bits from its JWK parts.
func rsaKey(n, e string) *rsa.PublicKey {
	nb, err1 := base64.RawURLEncoding.DecodeString(n)
	eb, err2 := base64.RawURLEncoding.DecodeString(e)
	if err1 != nil || err2 != nil || len(eb) == 0 || len(eb) > 4 {
		return nil
	}
	exp := 0
	for _, b := range eb {
		exp = exp<<8 | int(b)
	}
	mod := new(big.Int).SetBytes(nb)
	if mod.BitLen() < 2048 || exp < 3 {
		return nil
	}
	return &rsa.PublicKey{N: mod, E: exp}
}
