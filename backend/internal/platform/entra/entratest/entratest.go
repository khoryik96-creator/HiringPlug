// Package entratest is a fake Microsoft sign-in service for tests: a token
// endpoint that answers a code with an ID token signed by a test RSA key, and
// the keys (JWKS) to check it. Tests say what each code's token holds.
package entratest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
)

// Key is a signing key and the ID it is published under.
type Key struct {
	ID      string
	Private *rsa.PrivateKey
}

// NewKey makes a 2048-bit test key.
func NewKey(t testing.TB, id string) Key {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return Key{ID: id, Private: k}
}

// Grant is what one code gives: the token's claims and how it is signed.
// Zero values get sensible defaults (see Token).
type Grant struct {
	Claims jwt.MapClaims
	Key    *Key // nil: the server's current key
	// Graph tokens too (a sign-in that asked for Outlook): given when
	// RefreshToken is set, with what was granted (Scope).
	AccessToken, RefreshToken, Scope string
	ExpiresIn                        int // seconds; 3600 when 0
}

// Exchange is a code exchange the server received.
type Exchange struct {
	Path                                             string
	ClientID, ClientSecret, Code, Redirect, Verifier string
	Scope                                            string // what the exchange asked for
}

// Server is the fake sign-in service.
type Server struct {
	*httptest.Server
	t testing.TB

	mu        sync.Mutex
	published []Key // in the JWKS
	signWith  Key
	grants    map[string]Grant
	exchanges []Exchange
	keyFetch  int
	held      map[string]chan struct{} // organisations whose keys are held back
	waiting   int
}

// New starts a fake sign-in service publishing one key.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, grants: map[string]Grant{}}
	k := NewKey(t, "test-key-1")
	s.published, s.signWith = []Key{k}, k
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Rotate publishes a new key and signs with it from now on.
func (s *Server) Rotate(id string) Key {
	k := NewKey(s.t, id)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, k)
	s.signWith = k
	return k
}

// Grant makes code give a token with claims.
func (s *Server) Grant(code string, g Grant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[code] = g
}

// Exchanges are the code exchanges received so far.
func (s *Server) Exchanges() []Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Exchange(nil), s.exchanges...)
}

// SigningKey is the key tokens are signed with now (published).
func (s *Server) SigningKey() Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signWith
}

// KeyFetchesWaiting is how many key fetches are held back (HoldKeys) now.
func (s *Server) KeyFetchesWaiting() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waiting
}

// KeyFetches is how often the keys were fetched.
func (s *Server) KeyFetches() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keyFetch
}

// HoldKeys makes fetching an organisation's keys wait until release is
// called (to see who else is kept waiting meanwhile).
func (s *Server) HoldKeys(tenantID string) (release func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	if s.held == nil {
		s.held = map[string]chan struct{}{}
	}
	s.held[tenantID] = gate
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

// Claims are a valid token's claims for an account, to change as a test
// needs: issued now, for an hour; a member of the organisation (the
// optional claim acct, 0)
// whose sign-in name is email, and whose email Microsoft does not vouch for
// (no xms_edov: see VerifiedEmail).
func Claims(tenantID, objectID, email, clientID, nonce string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss": entra.Issuer(tenantID), "aud": clientID, "tid": tenantID, "oid": objectID,
		"email": email, "preferred_username": email, "name": "Test Person", "nonce": nonce, "ver": "2.0", "acct": 0,
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
}

// VerifiedEmail is what Microsoft adds when the app asks for the optional
// claim xms_edov (Entra › Token configuration): whether the email's domain
// is one the organisation verified.
func VerifiedEmail(c jwt.MapClaims, verified bool) jwt.MapClaims {
	c["xms_edov"] = verified
	return c
}

// Guest makes claims a guest's, as Microsoft sends them: an account of
// another organisation invited in. Its preferred_username is its home
// address (its email, here); its UPN (the optional claim upn) is
// name_home.domain#EXT#@... acct is the optional claim (1: guest); upn
// says whether the upn claim is sent.
func Guest(c jwt.MapClaims, withAcct, upn bool) jwt.MapClaims {
	if withAcct {
		c["acct"] = 1
	} else {
		delete(c, "acct")
	}
	email, _ := c["email"].(string)
	c["preferred_username"] = email
	if upn && email != "" {
		c["upn"] = GuestUPN(email, "contoso.onmicrosoft.com")
	}
	return c
}

// WithoutAcct leaves the optional claim acct out, as Microsoft does until
// it is added in Entra (Token configuration).
func WithoutAcct(c jwt.MapClaims) jwt.MapClaims {
	delete(c, "acct")
	return c
}

// GuestUPN is the sign-in name an organisation gives a guest with email.
func GuestUPN(email, orgDomain string) string {
	return strings.ReplaceAll(email, "@", "_") + "#EXT#@" + orgDomain
}

// Sign signs claims with key.
func Sign(t testing.TB, k Key, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = k.ID
	raw, err := tok.SignedString(k.Private)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/discovery/v2.0/keys"):
		s.mu.Lock()
		gate := s.held[strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]]
		if gate != nil {
			s.waiting++
		}
		s.mu.Unlock()
		if gate != nil {
			<-gate
			s.mu.Lock()
			s.waiting--
			s.mu.Unlock()
		}
		s.mu.Lock()
		s.keyFetch++
		keys := make([]map[string]string, 0, len(s.published))
		for _, k := range s.published {
			keys = append(keys, map[string]string{
				"kty": "RSA", "use": "sig", "kid": k.ID,
				"n": base64.RawURLEncoding.EncodeToString(k.Private.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.Private.E)).Bytes()),
			})
		}
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	case strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") && r.Method == http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f := r.PostForm
		s.mu.Lock()
		s.exchanges = append(s.exchanges, Exchange{Path: r.URL.Path, ClientID: f.Get("client_id"), ClientSecret: f.Get("client_secret"),
			Code: f.Get("code"), Redirect: f.Get("redirect_uri"), Verifier: f.Get("code_verifier"), Scope: f.Get("scope")})
		g, ok := s.grants[f.Get("code")]
		key := s.signWith
		s.mu.Unlock()
		if !ok || f.Get("grant_type") != "authorization_code" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"AADSTS70008: <script>secret</script>"}`))
			return
		}
		if g.Key != nil {
			key = *g.Key
		}
		answer := map[string]any{"id_token": Sign(s.t, key, g.Claims), "token_type": "Bearer", "access_token": "unused"}
		if g.RefreshToken != "" {
			expires := g.ExpiresIn
			if expires == 0 {
				expires = 3600
			}
			answer["access_token"], answer["refresh_token"], answer["scope"], answer["expires_in"] = g.AccessToken, g.RefreshToken, g.Scope, expires
		}
		_ = json.NewEncoder(w).Encode(answer)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Challenge is the PKCE challenge of a verifier (S256).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
