package identity

// The rules for a company's role settings, as pure functions: a role's
// defaults, what may be chosen, and which candidates a user then sees.

import (
	"slices"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Candidate scopes.
const (
	ScopeAll  = "all"
	ScopeTeam = "team"
	ScopeOwn  = "own"
)

// Access is what a role may do in one company, and which candidates it sees.
type Access struct {
	Role          string
	Permissions   []api.Permission
	Scope         string
	JobCandidates bool
	Custom        bool // the company changed it from the defaults
	Version       int  // 0 while it has the defaults
}

// DefaultAccess is a role's access as the contract declares it: its
// permissions and every candidate.
func DefaultAccess(role string) Access {
	return Access{Role: role, Permissions: authz.PermissionsForRole(api.Role(role)), Scope: ScopeAll, JobCandidates: true}
}

// Editable reports whether a company may change a role. The owner always has
// every permission and sees all candidates, so nobody is locked out.
func Editable(role string) bool { return role != string(api.RoleOwner) }

// CheckAccess checks a company's choice for a role and tidies it (each
// permission once, sorted). Managing roles stays with owners.
func CheckAccess(in Access) (Access, error) {
	if !Editable(in.Role) {
		return Access{}, apperr.New(api.ErrorCodeROLENOTEDITABLE, "The owner role always has every permission and sees all candidates.")
	}
	var errs apperr.FieldErrors
	perms := slices.Clone(in.Permissions)
	slices.Sort(perms)
	perms = slices.Compact(perms)
	if slices.Contains(perms, api.PermissionIdentityRolesManage) {
		errs.Add("permissions", "only owners may choose what roles can do")
	}
	if in.Scope != ScopeAll && in.Scope != ScopeTeam && in.Scope != ScopeOwn {
		errs.Add("candidateScope", "must be all, team or own")
	}
	in.Permissions = perms
	return in, errs.Err()
}

// WithLaterDefaults is a company's saved permissions for role, plus the
// permissions added since it was saved (not in known) that role has by
// default: a new feature reaches the roles meant to have it, until the owner
// chooses otherwise. What the owner took away stays away.
func WithLaterDefaults(role string, saved, known []api.Permission) []api.Permission {
	out := slices.Clone(saved)
	for _, perm := range authz.PermissionsForRole(api.Role(role)) {
		if !slices.Contains(known, perm) && !slices.Contains(out, perm) {
			out = append(out, perm)
		}
	}
	slices.Sort(out)
	return out
}

// AllPermissions is every permission there is now (what a saved setting knew).
func AllPermissions() []api.Permission {
	all := authz.Catalog()
	out := make([]api.Permission, len(all))
	for i, p := range all {
		out[i] = p.Permission
	}
	slices.Sort(out)
	return out
}

// ScopeFor is which candidates a user with this access sees: all, or those
// owned by them (own) or by their team (team; just them without a team),
// with JobCandidates also those on jobs these people own.
func ScopeFor(a Access, userID string, teamUserIDs []string) authz.Scope {
	switch a.Scope {
	case ScopeOwn:
		return authz.Scope{Users: []string{userID}, Jobs: a.JobCandidates}
	case ScopeTeam:
		users := slices.Clone(teamUserIDs)
		if !slices.Contains(users, userID) {
			users = append(users, userID)
		}
		return authz.Scope{Users: users, Jobs: a.JobCandidates}
	default:
		return authz.Scope{}
	}
}
