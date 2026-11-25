package identity

// Two-step sign-in, as pure functions: time-based one-time codes (RFC 6238:
// HMAC-SHA1, 30-second steps, 6 digits, as every authenticator app makes
// them), recovery codes, and the short-lived challenge that carries a
// person from their password to their code.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 codes are HMAC-SHA1: what authenticator apps compute
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	totpStep        = 30 * time.Second
	totpDigits      = 6
	recoveryCodeLen = 10 // characters, shown as two groups of five
	recoveryCodes   = 10
	challengeLife   = 10 * time.Minute
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret is a new authenticator secret (160 random bits, base32).
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable")
	}
	return b32.EncodeToString(b)
}

// TOTPStep is the 30-second step a moment falls in.
func TOTPStep(t time.Time) int64 { return t.Unix() / int64(totpStep/time.Second) }

// TOTPCode is the code for a secret at a step.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // steps are positive
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000), nil
}

// CheckTOTP finds the step a code is for: now's, or one step either side
// (a phone's clock a little off); ok is false for a wrong code or one used
// already (its step not after lastStep).
func CheckTOTP(secret, code string, now time.Time, lastStep int64) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	current := TOTPStep(now)
	for _, s := range []int64{current, current - 1, current + 1} {
		want, err := TOTPCode(secret, s)
		if err == nil && s > lastStep && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// TOTPURI is what an authenticator app scans (otpauth://), naming the company
// and the person.
func TOTPURI(secret, company, email string) string {
	label := url.PathEscape("HiringPlug (" + company + "):" + email)
	q := url.Values{"secret": {secret}, "issuer": {"HiringPlug"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

const recoveryAlphabet = "abcdefghjkmnpqrstuvwxyz23456789" // no 0/o, 1/l/i

// NewRecoveryCodes are ten single-use codes ("abcde-fghjk"), and their hashes
// to keep.
func NewRecoveryCodes() (codes, hashes []string) {
	for range recoveryCodes {
		b := make([]byte, recoveryCodeLen)
		if _, err := rand.Read(b); err != nil {
			panic("secure randomness unavailable")
		}
		for i := range b {
			b[i] = recoveryAlphabet[int(b[i])%len(recoveryAlphabet)]
		}
		code := string(b[:5]) + "-" + string(b[5:])
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes
}

// HashRecoveryCode is how a recovery code is kept and found: case, spaces and
// the dash aside.
func HashRecoveryCode(code string) string {
	norm := strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	sum := sha256.Sum256([]byte("hiringplug-recovery:" + norm))
	return hex.EncodeToString(sum[:])
}

// LooksLikeRecoveryCode tells a recovery code from an app's 6-digit code.
func LooksLikeRecoveryCode(code string) bool {
	return len(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code))) == recoveryCodeLen
}

// Challenge is a signed, short-lived note that a person gave the right
// password: Setup when they must first add an authenticator app.
// ValidAfter is when their sessions had last been ended (Unix seconds, 0
// for never): a challenge from before a later end (a password change or
// reset) no longer works.
type Challenge struct {
	TenantID   string `json:"t"`
	UserID     string `json:"u"`
	Setup      bool   `json:"s,omitempty"`
	ValidAfter int64  `json:"v,omitempty"`
	Expires    int64  `json:"e"`
}

var errChallenge = errors.New("two-step challenge not valid")

// SignChallenge writes a challenge as "payload.signature".
func SignChallenge(key []byte, c Challenge) string {
	raw, _ := json.Marshal(c) //nolint:errchkjson // plain fields
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + challengeMAC(key, payload)
}

// ReadChallenge checks a challenge's signature and expiry.
func ReadChallenge(key []byte, token string, now time.Time) (Challenge, error) {
	payload, mac, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(mac), []byte(challengeMAC(key, payload))) {
		return Challenge{}, errChallenge
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Challenge{}, errChallenge
	}
	var c Challenge
	if json.Unmarshal(raw, &c) != nil || c.TenantID == "" || c.UserID == "" || now.Unix() > c.Expires {
		return Challenge{}, errChallenge
	}
	return c, nil
}

func challengeMAC(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("hiringplug-two-step:" + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
