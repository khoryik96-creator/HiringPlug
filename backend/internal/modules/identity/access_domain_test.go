package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func TestARolesChoicesAreTidiedAndTheOwnerIsFixed(t *testing.T) {
	a, err := CheckAccess(Access{Role: "recruiter", Scope: ScopeOwn, Permissions: []api.Permission{api.PermissionJobsRead, api.PermissionCandidatesRead, api.PermissionJobsRead}})
	require.NoError(t, err)
	assert.Equal(t, []api.Permission{api.PermissionCandidatesRead, api.PermissionJobsRead}, a.Permissions, "each once, sorted")

	_, err = CheckAccess(Access{Role: "owner", Scope: ScopeAll})
	require.Error(t, err)
	assert.False(t, Editable("owner"))
	_, err = CheckAccess(Access{Role: "admin", Scope: ScopeAll, Permissions: []api.Permission{api.PermissionIdentityRolesManage}})
	require.Error(t, err, "managing roles stays with owners")
	_, err = CheckAccess(Access{Role: "admin", Scope: "some"})
	require.Error(t, err)

	d := DefaultAccess("sourcer")
	assert.Equal(t, authz.PermissionsForRole(api.RoleSourcer), d.Permissions)
	assert.Equal(t, ScopeAll, d.Scope)
	assert.False(t, d.Custom)
}

func TestWhichCandidatesAUserSees(t *testing.T) {
	assert.Equal(t, authz.Scope{}, ScopeFor(Access{Scope: ScopeAll, JobCandidates: true}, "usr_me", nil), "all")
	assert.Equal(t, authz.Scope{Users: []string{"usr_me"}, Jobs: true}, ScopeFor(Access{Scope: ScopeOwn, JobCandidates: true}, "usr_me", []string{"usr_b"}))
	assert.Equal(t, authz.Scope{Users: []string{"usr_a", "usr_me"}}, ScopeFor(Access{Scope: ScopeTeam}, "usr_me", []string{"usr_a", "usr_me"}))
	assert.Equal(t, authz.Scope{Users: []string{"usr_me"}}, ScopeFor(Access{Scope: ScopeTeam}, "usr_me", nil), "no team: their own")
}

// A permission added after a company customised a role reaches that role
// when the role has it by default; what the owner took away stays away.
func TestLaterPermissionsFollowTheRolesDefault(t *testing.T) {
	defaults := authz.PermissionsForRole(api.RoleAdmin)
	if len(defaults) < 2 {
		t.Skip("needs two admin permissions")
	}
	added, removed := defaults[0], defaults[1]
	var known, saved []api.Permission
	for _, p := range AllPermissions() {
		if p != added { // added in a later version: the saved setting never saw it
			known = append(known, p)
		}
	}
	for _, p := range defaults {
		if p != removed && p != added {
			saved = append(saved, p)
		}
	}
	got := WithLaterDefaults("admin", saved, known)
	assert.Contains(t, got, added, "a new admin permission reaches a customised admin role")
	assert.NotContains(t, got, removed, "one the owner took away stays away")
	assert.Equal(t, saved, WithLaterDefaults("admin", saved, AllPermissions())[:len(saved)], "nothing new: the saved setting as it is")
	assert.NotContains(t, WithLaterDefaults("viewer", nil, known), added, "only to roles that have it by default")
}
