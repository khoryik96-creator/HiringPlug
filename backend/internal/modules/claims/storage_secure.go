package claims

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// The storage account's tokens, and a sign-in's verifier and code, are sealed
// (AES-256-GCM, keyed from HP_MAIL_KEYS_SECRET) with the company and purpose
// bound in: a value copied to another company or use does not open.

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Storage) aead() (cipher.AEAD, error) {
	if len(s.config.KeysSecret) < 32 {
		return nil, errors.New("claims storage encryption is not configured")
	}
	key := sha256.Sum256(append([]byte("claim-storage:"), s.config.KeysSecret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealContext(tenant, purpose string) []byte {
	return []byte("claim-storage:v1:" + tenant + ":" + purpose)
}

func (s *Storage) seal(tenant, purpose string, v any) (string, error) {
	g, err := s.aead()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(g.Seal(nonce, nonce, raw, sealContext(tenant, purpose))), nil
}

var errSealed = errors.New("the claims storage credentials cannot be opened; connect the account again")

func (s *Storage) open(tenant, purpose, value string, out any) error {
	g, err := s.aead()
	if err != nil {
		return err
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) < g.NonceSize() {
		return errSealed
	}
	raw, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], sealContext(tenant, purpose))
	if err != nil {
		return errSealed
	}
	return json.Unmarshal(raw, out)
}

// stateClaim is what the provider hands back to the callback: who started
// the sign-in, signed so it cannot be made up.
type stateClaim struct {
	Tenant, User, Provider, Nonce string
	Expires                       int64
}

func (s *Storage) signState(c stateClaim) string {
	raw, _ := json.Marshal(c)
	body := base64.RawURLEncoding.EncodeToString(raw)
	m := hmac.New(sha256.New, s.config.KeysSecret)
	_, _ = m.Write([]byte("claim-storage-state:" + body))
	return body + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (s *Storage) readState(value string) (stateClaim, error) {
	var c stateClaim
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(value) > 4096 || len(s.config.KeysSecret) < 32 {
		return c, errors.New("invalid claims storage sign-in")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, err
	}
	m := hmac.New(sha256.New, s.config.KeysSecret)
	_, _ = m.Write([]byte("claim-storage-state:" + parts[0]))
	if !hmac.Equal(sig, m.Sum(nil)) {
		return c, errors.New("invalid claims storage sign-in")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Tenant == "" || c.User == "" || c.Nonce == "" || c.Expires <= time.Now().Unix() {
		return c, errors.New("expired claims storage sign-in")
	}
	return c, nil
}
