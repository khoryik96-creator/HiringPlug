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
