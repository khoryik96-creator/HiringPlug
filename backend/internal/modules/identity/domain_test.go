package identity

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswords(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$"))
	assert.True(t, CheckPassword(h, "correct horse battery"))
	assert.False(t, CheckPassword(h, "correct horse batterY"))
	assert.False(t, CheckPassword("", "x"))
	assert.False(t, CheckPassword("$argon2id$v=19$garbage", "x"))
	other, _ := HashPassword("correct horse battery")
	assert.NotEqual(t, h, other, "salted")
	assert.False(t, CheckPassword(dummyHash(), "correct horse battery"))

	assert.Contains(t, PasswordProblem("short", "a@b.c", "A"), "10 characters")
	assert.Contains(t, PasswordProblem("Password123", "a@b.c", "A"), "too common")
	assert.Contains(t, PasswordProblem("siti@contoh.my", "siti@contoh.my", "Siti"), "email or name")
	assert.Empty(t, PasswordProblem("purple-gecko-41", "siti@contoh.my", "Siti"))
}

func TestTeamRules(t *testing.T) {
	owner := Member{ID: "usr_o", Role: "owner", Active: true}
	admin := Member{ID: "usr_a", Role: "admin", Active: true}
	recruiter := Member{ID: "usr_r", Role: "recruiter", Active: true}

	assert.Empty(t, ChangeProblem(admin, recruiter, "sourcer", true, 1))
	assert.Empty(t, ChangeProblem(admin, recruiter, "recruiter", false, 1), "deactivate")
	assert.Contains(t, ChangeProblem(admin, admin, "recruiter", true, 1), "your own role")
	assert.Contains(t, ChangeProblem(admin, admin, "admin", false, 1), "deactivate yourself")
	assert.Empty(t, ChangeProblem(admin, admin, "admin", true, 1), "nothing changes")
	assert.Contains(t, ChangeProblem(admin, recruiter, "owner", true, 1), "Only an owner")
	assert.Contains(t, ChangeProblem(admin, owner, "admin", true, 2), "Only an owner")
	assert.Empty(t, ChangeProblem(owner, recruiter, "owner", true, 1))
	second := Member{ID: "usr_o2", Role: "owner", Active: true}
	assert.Empty(t, ChangeProblem(owner, second, "admin", true, 2))
	assert.Contains(t, ChangeProblem(second, owner, "owner", false, 1), "at least one active owner")

	assert.Contains(t, InviteProblem("admin", "owner"), "Only an owner")
	assert.Empty(t, InviteProblem("owner", "owner"))

	// Nobody gives or takes a role above their own, even when their role was
	// let to manage the team.
	assert.Contains(t, InviteProblem("recruiter", "admin"), "Only an admin or owner")
	assert.Empty(t, InviteProblem("recruiter", "sourcer"))
	assert.Empty(t, InviteProblem("admin", "admin"))
	sourcer := Member{ID: "usr_s", Role: "sourcer", Active: true}
	assert.Contains(t, ChangeProblem(recruiter, sourcer, "admin", true, 1), "Only an admin or owner")
	assert.Contains(t, ChangeProblem(recruiter, admin, "admin", false, 1), "Only an admin or owner")
	assert.Empty(t, ChangeProblem(recruiter, sourcer, "viewer", true, 1))
	assert.Empty(t, ChangeProblem(recruiter, admin, "admin", true, 1), "nothing changes (e.g. only their team)")
}

// Someone guessing a person's password is cut off, but cannot lock that
// person out: the count is per address and email.
func TestSignInLimitsDoNotLockOutTheVictim(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimits(func() time.Time { return now })
	for i := range 10 {
		assert.True(t, l.signIn("203.0.113.9", "siti@contoh.my"), "guess %d", i+1)
	}
	assert.False(t, l.signIn("203.0.113.9", "siti@contoh.my"), "the guesser is cut off")
	assert.True(t, l.signIn("198.51.100.4", "siti@contoh.my"), "Siti still signs in from her own address")
	assert.True(t, l.signIn("203.0.113.9", "aina@contoh.my"), "per email: a shared office address is not blocked for everyone")
	for range 19 {
		l.signIn("203.0.113.9", "aina@contoh.my")
	}
	assert.False(t, l.signIn("203.0.113.9", "other@contoh.my"), "an address trying many accounts is cut off as a whole")
}
