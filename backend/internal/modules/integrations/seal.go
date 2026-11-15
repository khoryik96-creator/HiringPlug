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
// so a database dump or a backup does not give them away. The key comes from
// HP_WEBHOOK_KEYS_SECRET, a setting of its own: changing the sign-in secret
// never breaks webhooks. The company is bound in, so a sealed secret copied
// to another company's webhook does not open. Without that setting, secrets
// are kept as they are.

const sealedPrefix = "v1."

// Sealer seals and opens webhook secrets.
type Sealer struct {
	key    *[32]byte // nil: secrets are kept as they are
	legacy *[32]byte // the key of an earlier version (derived from the sign-in secret), to open what it sealed
}

func derive(secret []byte) *[32]byte {
	if len(secret) == 0 {
		return nil
	}
	k := sha256.Sum256(append([]byte("hiringplug-webhook-secret:"), secret...))
	return &k
}

// NewSealer seals with keysSecret (HP_WEBHOOK_KEYS_SECRET; empty: no
// sealing). legacySecret is the sign-in secret, which an earlier version
// sealed with: what it sealed still opens, and is sealed again with the key.
func NewSealer(keysSecret, legacySecret []byte) Sealer {
	return Sealer{key: derive(keysSecret), legacy: derive(legacySecret)}
}

func gcmOf(key *[32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal returns the secret as it is stored.
func (s Sealer) Seal(tenantID, secret string) (string, error) {
	if s.key == nil {
		return secret, nil
	}
	gcm, err := gcmOf(s.key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sealedPrefix + base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(secret), []byte("webhook:"+tenantID))), nil
}

// ErrSecretLost: the stored secret cannot be opened (HP_WEBHOOK_KEYS_SECRET
// changed or was removed since it was saved).
var ErrSecretLost = errors.New("the webhook's signing secret can no longer be opened (HP_WEBHOOK_KEYS_SECRET changed or was removed): remove this webhook and add it again")

// Open returns the secret from what is stored, and whether it should be
// stored again (kept as it was while a key is set, or sealed by an earlier
// version's key).
func (s Sealer) Open(tenantID, stored string) (secret string, restore bool, err error) {
	if !strings.HasPrefix(stored, sealedPrefix) {
		return stored, s.key != nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	if err != nil {
		return "", false, ErrSecretLost
	}
	for i, key := range []*[32]byte{s.key, s.legacy} {
		if key == nil {
			continue
		}
		gcm, err := gcmOf(key)
		if err != nil || len(raw) <= gcm.NonceSize() {
			return "", false, ErrSecretLost
		}
		if plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("webhook:"+tenantID)); err == nil {
			return string(plain), i == 1, nil
		}
	}
	return "", false, ErrSecretLost
}
