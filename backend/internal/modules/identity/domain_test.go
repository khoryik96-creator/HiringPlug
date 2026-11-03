package identity

import (
	"strings"
	"testing"

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
}
