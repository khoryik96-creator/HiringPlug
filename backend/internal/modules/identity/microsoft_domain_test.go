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
	choose := func(found []MicrosoftAccount, tenantID, slug string) []MicrosoftAccount {
		out, _ := ChooseMicrosoftAccounts(found, tenantID, slug)
		return out
	}
	assert.Equal(t, found, choose(found, "", ""), "any company: both, to choose from")
	assert.Equal(t, []MicrosoftAccount{beta}, choose(found, "tnt_b", ""), "the company it was started for")
	assert.Equal(t, []MicrosoftAccount{alpha}, choose(found, "", " Alpha "), "the company chosen")
	assert.Empty(t, choose(found, "tnt_a", "beta"))
	assert.Equal(t, []MicrosoftAccount{alpha}, choose([]MicrosoftAccount{alpha, beta, twin}, "", ""),
		"two people at one company: neither")
}

// Owners, and people whose Microsoft account was unlinked, link one only on
// purpose (Your account, an invitation or reconnect link: not through
// ChooseMicrosoftAccounts), never by its address.
func TestOwnersAndTheUnlinkedAreNeverMatchedByAnAddressAlone(t *testing.T) {
	owner := MicrosoftAccount{TenantID: "tnt_a", Slug: "alpha", UserID: "usr_owner", Owner: true}
	unlinked := MicrosoftAccount{TenantID: "tnt_c", Slug: "gamma", UserID: "usr_3", ConnectOnly: true}
	other := MicrosoftAccount{TenantID: "tnt_b", Slug: "beta", UserID: "usr_2"}
	out, left := ChooseMicrosoftAccounts([]MicrosoftAccount{owner, unlinked, other}, "", "")
	assert.Equal(t, []MicrosoftAccount{other}, out)
	assert.True(t, left, "left out: they connect the account first")

	for _, a := range []MicrosoftAccount{owner, unlinked} {
		a.Linked = true
		out, left = ChooseMicrosoftAccounts([]MicrosoftAccount{a}, "", "")
		assert.Equal(t, []MicrosoftAccount{a}, out, "linked to them: yes")
		assert.False(t, left)
	}
	assert.True(t, MayLinkByAddress(false, false))
	assert.False(t, MayLinkByAddress(true, false))
	assert.False(t, MayLinkByAddress(false, true))
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
