package identity

// The sign-in and team rules, as plain functions on plain values: no
// database, no HTTP.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Argon2id settings (OWASP's minimum: 19 MiB, 2 passes, 1 lane).
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
)

// HashPassword returns a salted Argon2id hash in the usual
// $argon2id$v=19$m=..,t=..,p=..$salt$hash form.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

var errBadHash = errors.New("not an argon2id hash")

// CheckPassword reports whether password matches hash (in constant time).
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[4])
	want, err2 := enc.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want))) //nolint:gosec // a key length
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is checked when no account matches, so an unknown email takes
// as long as a wrong password (no hint which emails exist).
var dummyHash = sync.OnceValue(func() string {
	h, err := HashPassword("not anyone's password")
	if err != nil {
		panic(errBadHash)
	}
	return h
})

// commonPasswords are refused whatever their length.
var commonPasswords = []string{"password123", "1234567890", "qwertyuiop", "password1234", "letmein1234", "iloveyou123", "abcdefghij", "0123456789"}

// PasswordProblem says what is wrong with a new password, or "".
func PasswordProblem(password, email, name string) string {
	p := strings.ToLower(password)
	switch {
	case len([]rune(password)) < 10:
		return "use at least 10 characters"
	case len(password) > 200:
		return "use at most 200 characters"
	case p == strings.ToLower(email) || p == strings.ToLower(strings.ReplaceAll(name, " ", "")):
		return "do not use your email or name"
	}
	for _, c := range commonPasswords {
		if p == c {
			return "this password is too common"
		}
	}
	return ""
}

// Member is a user as the team rules see them.
type Member struct {
	ID, Role string
	Active   bool
}

// InviteProblem says why actor may not invite someone as role, or "".
func InviteProblem(actorRole, role string) string {
	if role == "owner" && actorRole != "owner" {
		return "Only an owner can make someone an owner."
	}
	return ""
}

// ChangeProblem says why actor may not change target to (role, active), or
// "". activeOwners counts the company's active owners now.
//   - nobody changes their own role or deactivates themselves (no lock-out);
//   - only owners make, change or deactivate owners;
//   - the last active owner stays an active owner.
func ChangeProblem(actor, target Member, role string, active bool, activeOwners int) string {
	switch {
	case actor.ID == target.ID && (role != target.Role || !active):
		return "You cannot change your own role or deactivate yourself: ask another admin or owner."
	case (target.Role == "owner" || role == "owner") && actor.Role != "owner" && (role != target.Role || active != target.Active):
		return "Only an owner can make, change or deactivate an owner."
	case target.Role == "owner" && target.Active && (role != "owner" || !active) && activeOwners <= 1:
		return "The company needs at least one active owner: make someone else an owner first."
	}
	return ""
}
