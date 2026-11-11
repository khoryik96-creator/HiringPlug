package emails

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

type mailboxSecret struct {
	Password     string    `json:"password,omitempty"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
}

func (s *Service) sealMailbox(tenant, user, provider string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	gcm, err := s.mailboxCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, raw, []byte("mailbox:v1:"+tenant+":"+user+":"+provider))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}
func (s *Service) openMailbox(tenant, user, provider, sealed string, out any) error {
	gcm, err := s.mailboxCipher()
	if err != nil {
		return err
	}
	data, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(data) < gcm.NonceSize() {
		return errors.New("mailbox credentials cannot be opened; reconnect the mailbox")
	}
	raw, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("mailbox:v1:"+tenant+":"+user+":"+provider))
	if err != nil {
		return errors.New("mailbox credentials cannot be opened; reconnect the mailbox")
	}
	return json.Unmarshal(raw, out)
}
func (s *Service) mailboxCipher() (cipher.AEAD, error) {
	if len(s.connections.KeysSecret) < 32 {
		return nil, errors.New("mailbox encryption is not configured")
	}
	key := sha256.Sum256(s.connections.KeysSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
