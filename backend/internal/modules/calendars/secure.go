package calendars

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
func (s *Service) aead() (cipher.AEAD, error) {
	if len(s.config.KeysSecret) < 32 {
		return nil, errors.New("calendar encryption is not configured")
	}
	key := sha256.Sum256(s.config.KeysSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (s *Service) seal(tenant, user, purpose string, v any) (string, error) {
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
	return base64.RawURLEncoding.EncodeToString(g.Seal(nonce, nonce, raw, []byte("calendar:v1:"+tenant+":"+user+":"+purpose))), nil
}
func (s *Service) open(tenant, user, purpose, value string, out any) error {
	g, err := s.aead()
	if err != nil {
		return err
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(data) < g.NonceSize() {
		return errors.New("calendar credentials cannot be opened; reconnect Outlook")
	}
	raw, err := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte("calendar:v1:"+tenant+":"+user+":"+purpose))
	if err != nil {
		return errors.New("calendar credentials cannot be opened; reconnect Outlook")
	}
	return json.Unmarshal(raw, out)
}

type stateClaim struct {
	Tenant, User, Nonce string
	Expires             int64
}

func (s *Service) signState(c stateClaim) string {
	raw, _ := json.Marshal(c)
	body := base64.RawURLEncoding.EncodeToString(raw)
	m := hmac.New(sha256.New, s.config.KeysSecret)
	_, _ = m.Write([]byte("outlook-calendar-state:" + body))
	return body + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (s *Service) readState(value string) (stateClaim, error) {
	var c stateClaim
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(value) > 4096 || len(s.config.KeysSecret) < 32 {
		return c, errors.New("invalid calendar sign-in")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, err
	}
	m := hmac.New(sha256.New, s.config.KeysSecret)
	_, _ = m.Write([]byte("outlook-calendar-state:" + parts[0]))
	if !hmac.Equal(sig, m.Sum(nil)) {
		return c, errors.New("invalid calendar sign-in")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Tenant == "" || c.User == "" || c.Nonce == "" || c.Expires <= time.Now().Unix() {
		return c, errors.New("expired calendar sign-in")
	}
	return c, nil
}
