package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnlyTheStartingTabsValueFinishesASignIn(t *testing.T) {
	value := "0123456789abcdefghijklmnopqrstuvwxyzABCDEF"
	sum := sha256.Sum256([]byte(value))
	hash := hex.EncodeToString(sum[:])
	assert.True(t, BindingMatches(hash, value))
	assert.False(t, BindingMatches(hash, value+"x"))
	assert.False(t, BindingMatches(hash, hash), "the hash itself is not the value")
	short := "too-short"
	sum = sha256.Sum256([]byte(short))
	assert.False(t, BindingMatches(hex.EncodeToString(sum[:]), short), "at least 32 characters")
}

func TestChoosingWhomAMicrosoftAccountSignsInAs(t *testing.T) {
	alpha := MicrosoftAccount{TenantID: "tnt_a", Slug: "alpha", UserID: "usr_1"}
	beta := MicrosoftAccount{TenantID: "tnt_b", Slug: "beta", UserID: "usr_2"}
	twin := MicrosoftAccount{TenantID: "tnt_b", Slug: "beta", UserID: "usr_3"}
	found := []MicrosoftAccount{alpha, beta}
	choose := func(found []MicrosoftAccount, tenantID, slug, invited string) []MicrosoftAccount {
		out, _ := ChooseMicrosoftAccounts(found, tenantID, slug, invited)
		return out
	}
	assert.Equal(t, found, choose(found, "", "", ""), "any company: both, to choose from")
	assert.Equal(t, []MicrosoftAccount{beta}, choose(found, "tnt_b", "", ""), "the company it was started for")
	assert.Equal(t, []MicrosoftAccount{alpha}, choose(found, "", " Alpha ", ""), "the company chosen")
	assert.Empty(t, choose(found, "tnt_a", "beta", ""))
	assert.Equal(t, []MicrosoftAccount{alpha}, choose([]MicrosoftAccount{alpha, beta, twin}, "", "", ""),
		"two people at one company: neither")
}

func TestAnOwnerIsNeverMatchedByAnAddressAlone(t *testing.T) {
	owner := MicrosoftAccount{TenantID: "tnt_a", Slug: "alpha", UserID: "usr_owner", Owner: true}
	other := MicrosoftAccount{TenantID: "tnt_b", Slug: "beta", UserID: "usr_2"}
	out, left := ChooseMicrosoftAccounts([]MicrosoftAccount{owner, other}, "", "", "")
	assert.Equal(t, []MicrosoftAccount{other}, out)
	assert.True(t, left, "an owner was left out: they connect the account first")

	linked := owner
	linked.Linked = true
	out, left = ChooseMicrosoftAccounts([]MicrosoftAccount{linked}, "", "", "")
	assert.Equal(t, []MicrosoftAccount{linked}, out, "linked to them: yes")
	assert.False(t, left)

	out, left = ChooseMicrosoftAccounts([]MicrosoftAccount{owner}, "tnt_a", "", "usr_owner")
	assert.Equal(t, []MicrosoftAccount{owner}, out, "their own invitation: yes")
	assert.False(t, left)
	out, _ = ChooseMicrosoftAccounts([]MicrosoftAccount{owner}, "tnt_a", "", "usr_someone_else")
	assert.Empty(t, out, "someone else's invitation: no")
}

func TestPasswordsWhenTheCompanyRequiresMicrosoft(t *testing.T) {
	assert.False(t, PasswordRefused(false, "recruiter"))
	assert.True(t, PasswordRefused(true, "recruiter"))
	assert.False(t, PasswordRefused(true, "owner"), "owners keep their password")
	assert.Equal(t, "none", MicrosoftLinkState(false, false, "recruiter"))
	assert.Equal(t, "available", MicrosoftLinkState(true, false, "recruiter"))
	assert.Equal(t, "required", MicrosoftLinkState(true, true, "recruiter"))
	assert.Equal(t, "available", MicrosoftLinkState(true, true, "owner"))
}
