package app_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// An admin role customised before a feature existed still gets that
// feature's admin permission (here: entering the email app details).
func TestACustomisedRoleGetsPermissionsAddedLater(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner, admin := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token
	newer := api.PermissionEmailsAppsManage
	require.Contains(t, authz.PermissionsForRole(api.RoleAdmin), newer)
	defaults := authz.PermissionsForRole(api.RoleAdmin)
	saved := slices.DeleteFunc(slices.Clone(defaults), func(p api.Permission) bool { return p == newer || p == api.PermissionCandidatesMerge })
	a.setRole(owner, "admin", map[string]any{"version": 0, "permissions": saved, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)

	// As if it was saved before the email app details existed.
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE role_setting SET known_permissions = array_remove(known_permissions, $2) WHERE tenant_id = $1 AND role = 'admin'`, tenant.ID, string(newer))
	require.NoError(t, err)

	perms := a.me(admin).Permissions
	assert.Contains(t, perms, newer, "added since: the role's default")
	assert.NotContains(t, perms, api.PermissionCandidatesMerge, "taken away by the owner: stays away")
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/settings/email-apps", admin, nil).Status)
}
