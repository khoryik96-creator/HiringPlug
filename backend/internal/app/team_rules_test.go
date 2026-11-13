package app_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var inviteToken = regexp.MustCompile(`/#/invite/([0-9a-f]{64})$`)

// An invitation sent to the wrong person can be cancelled: its link stops
// working at once, they leave the team list, and can be invited again.
func TestAnInvitationCanBeCancelled(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	email := "wrong." + tenant.ID + "@test.local"
	invite := func(token, role string) api.Invitation {
		t.Helper()
		res := a.request(http.MethodPost, "/v1/team/invites", token, map[string]any{"email": email, "name": "Wrong Person", "role": role})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var inv api.Invitation
		res.JSON(&inv)
		return inv
	}
	inv := invite(admin.Token, "recruiter")
	link := inviteToken.FindStringSubmatch(inv.Link)[1]
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Status)

	path := "/v1/team/" + inv.Member.Id + "/invite"
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, path, recruiter.Token, nil).Status, "team managers only")
	require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, path, admin.Token, nil).Status)
	assert.Equal(t, http.StatusGone, a.request(http.MethodGet, "/v1/auth/links/"+link, "", nil).Status, "the link stops working at once")
	res := a.request(http.MethodPost, "/v1/auth/links/"+link, "", map[string]any{"password": "purple-gecko-41"})
	assert.Equal(t, http.StatusGone, res.Status, "and cannot be used to join: %s", res.Body)
	var team struct{ Items []api.TeamMember }
	a.request(http.MethodGet, "/v1/team", admin.Token, nil).JSON(&team)
	for _, m := range team.Items {
		assert.NotEqual(t, inv.Member.Id, m.Id, "gone from the team list")
	}
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, path, admin.Token, nil).Status)

	again := invite(admin.Token, "viewer")
	assert.NotEqual(t, inv.Member.Id, again.Member.Id, "the same address can be invited again")

	res = a.request(http.MethodDelete, "/v1/team/"+recruiter.ID+"/invite", admin.Token, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "someone who joined is deactivated instead: %s", res.Body)

	boss := a.request(http.MethodPost, "/v1/team/invites", owner.Token, map[string]any{"email": "boss." + tenant.ID + "@test.local", "name": "Boss", "role": "owner"})
	require.Equal(t, http.StatusCreated, boss.Status, "%s", boss.Body)
	var bossInv api.Invitation
	boss.JSON(&bossInv)
	res = a.request(http.MethodDelete, "/v1/team/"+bossInv.Member.Id+"/invite", admin.Token, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, res.Status, "an owner's invitation is the owners' to cancel")
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/team/"+bossInv.Member.Id+"/invite", owner.Token, nil).Status)
}

// Being let to manage the team or the integrations is not being let to make
// an admin: nobody gives a role above their own.
func TestNobodyGivesARoleAboveTheirOwn(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer)
	owner, admin, recruiter, sourcer := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	// The owner lets recruiters manage the team and the integrations.
	perms := append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionIdentityTeamManage, api.PermissionIntegrationsManage)
	a.setRole(owner.Token, "recruiter", map[string]any{"version": 0, "permissions": perms, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)

	invite := func(token, address, role string) *response {
		return a.request(http.MethodPost, "/v1/team/invites", token, map[string]any{"email": address + "." + tenant.ID + "@test.local", "name": "New Person", "role": role})
	}
	require.Equal(t, http.StatusCreated, invite(recruiter.Token, "mate", "sourcer").Status, "a teammate: yes")
	res := invite(recruiter.Token, "me2", "admin")
	require.Equal(t, http.StatusUnprocessableEntity, res.Status, "an admin: no: %s", res.Body)
	assert.Contains(t, res.Error().Error.Message, "Only an admin or owner")
	require.Equal(t, http.StatusCreated, invite(admin.Token, "admin2", "admin").Status, "admins make admins")

	patch := func(token, userID string, body map[string]any) int {
		return a.request(http.MethodPatch, "/v1/team/"+userID, token, body).Status
	}
	assert.Equal(t, http.StatusUnprocessableEntity, patch(recruiter.Token, sourcer.ID, map[string]any{"role": "admin"}), "nor promote one")
	assert.Equal(t, http.StatusUnprocessableEntity, patch(recruiter.Token, admin.ID, map[string]any{"active": false}), "nor deactivate an admin")
	assert.Equal(t, http.StatusUnprocessableEntity, patch(recruiter.Token, admin.ID, map[string]any{"role": "viewer"}), "nor demote one")
	assert.Equal(t, http.StatusOK, patch(recruiter.Token, sourcer.ID, map[string]any{"role": "viewer"}), "a teammate's role: yes")

	key := func(token, role string) int {
		return a.request(http.MethodPost, "/v1/api-keys", token, map[string]any{"name": "Sync", "role": role}).Status
	}
	assert.Equal(t, http.StatusForbidden, key(recruiter.Token, "admin"), "nor a key that acts as an admin")
	assert.Equal(t, http.StatusCreated, key(recruiter.Token, "recruiter"))
	assert.Equal(t, http.StatusCreated, key(admin.Token, "admin"))
}
