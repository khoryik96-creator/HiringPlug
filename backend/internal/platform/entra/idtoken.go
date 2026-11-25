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
)

// Why a token is refused. ErrInvalid covers everything that is not about
// whose account it is; the wrapped error says more, for the logs only.
var (
	ErrInvalid           = errors.New("entra: the ID token is not valid")
	ErrPersonalAccount   = errors.New("entra: a personal Microsoft account")
	ErrOtherOrganisation = errors.New("entra: an account of another organisation")
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
	Name               string
}

// Emails are the account's addresses (email, and the sign-in name when it
// is one), each once.
func (i Identity) Emails() []string {
	var out []string
	for _, e := range []string{i.Email, i.Username} {
		if strings.Contains(e, "@") && (len(out) == 0 || out[0] != e) {
			out = append(out, e)
		}
	}
	return out
}

type idClaims struct {
	TenantID string `json:"tid"`
	ObjectID string `json:"oid"`
	Nonce    string `json:"nonce"`
	Email    string `json:"email"`
	Username string `json:"preferred_username"`
	Name     string `json:"name"`
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
	}
	return Identity{
		TenantID: tid, ObjectID: oid, Name: strings.TrimSpace(claims.Name),
		Email: strings.ToLower(strings.TrimSpace(claims.Email)), Username: strings.ToLower(strings.TrimSpace(claims.Username)),
	}, nil
}

type keySet struct {
	keys    map[string]*rsa.PublicKey
	fetched time.Time
	missed  map[string]bool // unknown key IDs looked for since
}

// key returns an organisation's signing key kid: from the cache, or fetched
// again when the keys are a day old, or kid is new to them (Microsoft
// rotates its keys). A key ID still unknown after that fetches the keys
// again at most once a minute.
func (c *Client) key(ctx context.Context, tid, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	set, ok := c.keys[tid]
	if ok && now.Sub(set.fetched) < keysFor {
		if k := set.keys[kid]; k != nil {
			return k, nil
		}
		if now.Sub(set.fetched) < refetchAfter && (set.missed[kid] || len(set.missed) >= maxMissed) {
			return nil, errors.New("unknown key")
		}
	}
	keys, err := c.fetchKeys(ctx, tid)
	if err != nil {
		return nil, err
	}
	if len(c.keys) >= maxOrganisations {
		c.keys = map[string]keySet{}
	}
	set = keySet{keys: keys, fetched: now, missed: map[string]bool{}}
	c.keys[tid] = set
	if k := keys[kid]; k != nil {
		return k, nil
	}
	set.missed[kid] = true
	return nil, errors.New("unknown key")
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
