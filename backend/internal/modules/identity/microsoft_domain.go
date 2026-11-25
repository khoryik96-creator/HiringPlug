package identity

// Sign in with Microsoft: the rules, as pure functions (unit tested).

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// Why signing in with Microsoft did not work (details.reason of
// MICROSOFT_SIGN_IN_FAILED). Nothing more precise is ever said: not whether
// a team member was deactivated, nor anything Microsoft answered.
const (
	ReasonNotOnTeam         = "not_on_team"
	ReasonOtherOrganisation = "other_organisation"
	ReasonPersonalAccount   = "personal_account"
	ReasonFailed            = "failed"
)

// BindingMatches reports whether binding is the value whose SHA-256 (hex)
// the sign-in was started with, in the same browser tab.
func BindingMatches(bindingHash, binding string) bool {
	if len(binding) < 32 {
		return false
	}
	sum := sha256.Sum256([]byte(binding))
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(bindingHash)) == 1
}

// MicrosoftAccount is a team member a Microsoft account may sign in as.
type MicrosoftAccount struct {
	TenantID, TenantName, Slug, UserID string
	Linked                             bool // matched by the account linked to them, not by email
}

// ChooseMicrosoftAccounts narrows the team members a Microsoft account
// matches to those it may sign in as: at the company the sign-in was started
// for (tenantID, if any) and the company chosen (slug, if any). A company
// where it matches more than one person (its email and sign-in name are two
// people's) is left out: nobody can tell which they meant.
func ChooseMicrosoftAccounts(found []MicrosoftAccount, tenantID, slug string) []MicrosoftAccount {
	slug = strings.ToLower(strings.TrimSpace(slug))
	perTenant := map[string]int{}
	for _, a := range found {
		perTenant[a.TenantID]++
	}
	var out []MicrosoftAccount
	for _, a := range found {
		if perTenant[a.TenantID] != 1 || (tenantID != "" && a.TenantID != tenantID) || (slug != "" && a.Slug != slug) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// MicrosoftLinkState is whether a person's invitation link may be used with
// a password, given the company's Microsoft settings: "none" (Microsoft is
// off), "available" (either), or "required" (Microsoft only; owners may
// always use a password).
func MicrosoftLinkState(signInOn, required bool, role string) string {
	switch {
	case !signInOn:
		return "none"
	case required && role != "owner":
		return "required"
	default:
		return "available"
	}
}

// PasswordRefused reports whether a password may not be used (signing in,
// resetting, choosing or changing one, or a session signed in with one):
// the company requires Microsoft and the person is not an owner.
func PasswordRefused(microsoftRequired bool, role string) bool {
	return microsoftRequired && role != "owner"
}
