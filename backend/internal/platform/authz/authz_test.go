package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestRolePermissionsComeFromTheContract(t *testing.T) {
	recruiter := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleRecruiter)
	assert.True(t, recruiter.Can(api.PermissionCandidatesCompensationRead))

	sourcer := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleSourcer)
	assert.True(t, sourcer.Can(api.PermissionCandidatesCreate))
	assert.False(t, sourcer.Can(api.PermissionCandidatesCompensationRead))

	viewer := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleViewer)
	assert.False(t, viewer.Can(api.PermissionCandidatesUpdate))
}

func TestOwnersHaveEveryPermission(t *testing.T) {
	owner := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleOwner)
	for _, p := range System("tnt_x").Permissions() {
		assert.True(t, owner.Can(p), "owner lacks %s", p)
	}
}

func TestSalaryPermissionsAreMarkedSensitive(t *testing.T) {
	assert.True(t, IsSensitive(api.PermissionCandidatesCompensationRead))
	assert.False(t, IsSensitive(api.PermissionCandidatesRead))
}

func TestPermissionCSVIsSortedAndStable(t *testing.T) {
	p := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleViewer)
	assert.Equal(t, "candidates:read,documents:read", p.PermissionCSV())
}
