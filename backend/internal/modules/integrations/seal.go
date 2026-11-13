package integrations

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// Webhook signing secrets are sealed (AES-256-GCM) before they are stored,
// so a database dump or a backup file does not give them away. The key comes
// from the server's own secret; the company is bound in, so a sealed secret
// copied to another company's webhook does not open.

const sealedPrefix = "v1."

// Sealer seals and opens webhook secrets.
type Sealer struct{ key [32]byte }

// NewSealer derives the sealing key from the server's secret.
func NewSealer(serverSecret []byte) Sealer {
	return Sealer{key: sha256.Sum256(append([]byte("hiringplug-webhook-secret:"), serverSecret...))}
}

func (s Sealer) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal returns the secret as it is stored.
func (s Sealer) Seal(tenantID, secret string) (string, error) {
	gcm, err := s.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sealedPrefix + base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(secret), []byte("webhook:"+tenantID))), nil
}

// ErrSecretLost: the stored secret cannot be opened (the server's secret
// changed since it was saved).
var ErrSecretLost = errors.New("the webhook's signing secret can no longer be opened (the server's secret changed): remove this webhook and add it again")

// Open returns the secret from what is stored. One stored before sealing
// (as it was made: whsec_...) is returned as it is; sealed reports whether
// it was sealed.
func (s Sealer) Open(tenantID, stored string) (secret string, sealed bool, err error) {
	if !strings.HasPrefix(stored, sealedPrefix) {
		return stored, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	gcm, gerr := s.gcm()
	if err != nil || gerr != nil || len(raw) <= gcm.NonceSize() {
		return "", true, ErrSecretLost
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("webhook:"+tenantID))
	if err != nil {
		return "", true, ErrSecretLost
	}
	return string(plain), true, nil
}
