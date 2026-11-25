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
}

// Exchange is a code exchange the server received.
type Exchange struct {
	Path                                             string
	ClientID, ClientSecret, Code, Redirect, Verifier string
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

// KeyFetches is how often the keys were fetched.
func (s *Server) KeyFetches() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keyFetch
}

// Claims are a valid token's claims for an account, to change as a test
// needs: issued now, for an hour.
func Claims(tenantID, objectID, email, clientID, nonce string) jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss": entra.Issuer(tenantID), "aud": clientID, "tid": tenantID, "oid": objectID,
		"email": email, "preferred_username": email, "name": "Test Person", "nonce": nonce, "ver": "2.0",
		"iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}
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
			Code: f.Get("code"), Redirect: f.Get("redirect_uri"), Verifier: f.Get("code_verifier")})
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
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": Sign(s.t, key, g.Claims), "token_type": "Bearer", "access_token": "unused"})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Challenge is the PKCE challenge of a verifier (S256).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
