package app_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Owners add roles of their own, built on an editable role: someone given
// one gets its access (not their base role's), keeps its base role as their
// role, and goes back to their role's own access when given a plain role.
// Only owners shape roles; one still given to someone is not removed.
func TestNamedRoles(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	recruiterID := tenant.Users[api.RoleRecruiter].ID
	perms := func(token string) []api.Permission { return a.me(token).Permissions }
	require.Contains(t, perms(recruiter), api.PermissionCandidatesCompensationRead, "recruiters see salaries by default")

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/access/named-roles", admin, map[string]any{"name": "Junior recruiter", "baseRole": "recruiter"}).Status, "owners only")
	res := a.request(http.MethodPost, "/v1/access/named-roles", owner, map[string]any{"name": "Junior recruiter", "baseRole": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var junior api.NamedRole
	res.JSON(&junior)
	assert.Equal(t, api.Role("recruiter"), junior.BaseRole)
	assert.Contains(t, junior.Permissions, api.PermissionCandidatesCompensationRead, "starts from the base role's access")
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/access/named-roles", owner, map[string]any{"name": "junior RECRUITER", "baseRole": "sourcer"}).Status, "names are unique")
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/access/named-roles", owner, map[string]any{"name": "Boss", "baseRole": "owner"}).Status, "not on the owner")

	without := slices.DeleteFunc(slices.Clone(junior.Permissions), func(p api.Permission) bool { return p == api.PermissionCandidatesCompensationRead })
	res = a.request(http.MethodPut, "/v1/access/named-roles/"+junior.Id, owner, map[string]any{"name": "Junior recruiter", "version": junior.Version,
		"permissions": without, "candidateScope": "own", "jobCandidates": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&junior)

	// The admin (who manages the team) gives it to the recruiter.
	res = a.request(http.MethodPatch, "/v1/team/"+recruiterID, admin, map[string]any{"namedRoleId": junior.Id})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var member api.TeamMember
	res.JSON(&member)
	require.NotNil(t, member.NamedRoleId.MustGet())
	assert.Equal(t, junior.Id, member.NamedRoleId.MustGet())
	assert.Equal(t, api.Role("recruiter"), member.Role, "their role is its base role")
	assert.NotContains(t, perms(recruiter), api.PermissionCandidatesCompensationRead, "the named role's access, at once")
	var list api.NamedRoleList
	a.request(http.MethodGet, "/v1/access/named-roles", admin, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, 1, list.Items[0].Members)

	res = a.request(http.MethodDelete, "/v1/access/named-roles/"+junior.Id, owner, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status, "still given to someone: %s", res.Body)

	// A plain role takes the named role away: back to the role's own access.
	res = a.request(http.MethodPatch, "/v1/team/"+recruiterID, admin, map[string]any{"role": "recruiter"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&member)
	assert.True(t, member.NamedRoleId.IsNull())
	assert.Contains(t, perms(recruiter), api.PermissionCandidatesCompensationRead)
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/access/named-roles/"+junior.Id, owner, nil).Status)
}
