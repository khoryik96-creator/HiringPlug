package authz

import (
	"slices"
	"strings"
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

// The CSV goes into the database session (row-level security), so it must be
// the same for the same permissions every time. Checked without listing the
// viewer's permissions, which grow with every module.
func TestPermissionCSVIsSortedAndStable(t *testing.T) {
	p := NewPrincipal("tnt_x", "usr_x", "", "", api.RoleViewer)
	csv := p.PermissionCSV()
	parts := strings.Split(csv, ",")
	assert.True(t, slices.IsSorted(parts), csv)
	assert.Contains(t, parts, string(api.PermissionCandidatesRead))
	assert.NotContains(t, parts, string(api.PermissionCandidatesUpdate))
	for range 5 {
		assert.Equal(t, csv, NewPrincipal("tnt_x", "usr_x", "", "", api.RoleViewer).PermissionCSV())
	}
}

func TestALimitedScopeNeverMeansEveryone(t *testing.T) {
	all := NewPrincipal("t", "u", "", "", api.RoleRecruiter)
	assert.Empty(t, all.ScopeUsersSetting(), "the contract's roles see all candidates")
	assert.Empty(t, all.ScopeJobsSetting())

	own := NewPrincipalWithAccess("t", "usr_1", "", "", api.RoleRecruiter, []api.Permission{api.PermissionCandidatesRead, "made:up"}, Scope{Users: []string{"usr_1", "usr_2"}, Jobs: true})
	assert.Equal(t, "usr_1,usr_2", own.ScopeUsersSetting())
	assert.Equal(t, "on", own.ScopeJobsSetting())
	assert.Equal(t, []api.Permission{api.PermissionCandidatesRead}, own.Permissions(), "unknown permissions are ignored")

	nobody := NewPrincipalWithAccess("t", "u", "", "", api.RoleViewer, nil, Scope{Users: []string{}, Jobs: true})
	assert.Equal(t, "-", nobody.ScopeUsersSetting(), "an empty list would mean all")
	assert.Equal(t, "", NewPrincipalWithAccess("t", "u", "", "", api.RoleViewer, nil, Scope{Jobs: true}).ScopeJobsSetting(), "jobs only narrow a limited scope")
}

func TestTheCatalogHasEveryPermissionWithItsWords(t *testing.T) {
	c := Catalog()
	assert.Len(t, c, len(System("t").Permissions()))
	for _, p := range c {
		assert.NotEmpty(t, p.Description, p.Permission)
		assert.NotEmpty(t, p.Module, p.Permission)
	}
	assert.Equal(t, "assistant", c[0].Module, "sorted by module")
}
