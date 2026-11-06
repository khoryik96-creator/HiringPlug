package aiproviders

// The rules, as plain functions on plain values: no database, no HTTP.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Providers are the providers a company can enter a key for, in the order
// they are shown.
var Providers = []string{"deepseek", "claude", "openai"}

// Choices are what a company can switch to: a provider, the server's own
// provider ("server") or none ("off").
var Choices = append([]string{"server", "off"}, Providers...)

// DefaultModels is the model each provider starts with (CV Studio's choices,
// the newest at the time of writing).
var DefaultModels = map[string]string{"deepseek": "deepseek-v4-flash", "claude": "claude-opus-5-5", "openai": "gpt-5.5"}

var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,99}$`)

// CheckInput checks a provider's model and, when given, its API key.
func CheckInput(model string, key *string) (string, *string, error) {
	var errs apperr.FieldErrors
	model = strings.TrimSpace(model)
	if !modelName.MatchString(model) {
		errs.Add("model", "must be a model name, e.g. deepseek-v4-flash")
	}
	if key != nil {
		k := strings.TrimSpace(*key)
		switch {
		case len(k) < 8 || len(k) > 500:
			errs.Add("apiKey", "must be the provider's API key")
		case strings.ContainsFunc(k, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII }):
			errs.Add("apiKey", "must not contain spaces or other characters a key cannot have")
		}
		key = &k
	}
	return model, key, errs.Err()
}

// Hint is what is shown of a saved key: its last four characters.
func Hint(key string) string {
	if len(key) <= 4 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}

// sealVersion starts every sealed key, so the format can change later.
const sealVersion = "v1."

// Seal encrypts an API key for the AI worker: AES-256-GCM, the key derived
// from the secret with SHA-256, a random 12-byte nonce, the provider as
// associated data (a key sealed for one provider cannot be passed off as
// another's). The result is "v1." + base64url(nonce || ciphertext || tag).
// The AI worker's hiringplug_ai.keys.open_key reverses it.
func Seal(random io.Reader, secret []byte, provider, key string) (string, error) {
	sum := sha256.Sum256(secret)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return "", fmt.Errorf("seal: %w", err)
	}
	out := gcm.Seal(nonce, nonce, []byte(key), associated(provider))
	return sealVersion + base64.RawURLEncoding.EncodeToString(out), nil
}

func associated(provider string) []byte { return []byte("hiringplug-ai-key:" + provider) }

// Entry is a provider's saved settings (the key only sealed).
type Entry struct {
	Provider, Model, SealedKey string
}

// Pick is which provider the AI uses: the company's choice when it has a
// key, the server's own provider for "server" (or no choice yet), none
// otherwise. entry is nil when the server's own provider is used or when
// the AI is off (on tells which).
func Pick(choice string, serverOn bool, entries []Entry) (entry *Entry, on bool) {
	switch choice {
	case "", "server":
		return nil, serverOn
	case "off":
		return nil, false
	}
	i := slices.IndexFunc(entries, func(e Entry) bool { return e.Provider == choice })
	if i < 0 {
		return nil, false
	}
	return &entries[i], true
}
