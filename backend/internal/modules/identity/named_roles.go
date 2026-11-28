package identity

// Named roles: roles of the company's own ("Junior recruiter"), each built on
// one of the five editable roles, with its own permissions and candidate
// scope. A person given one keeps its base role as their role (their rank:
// who may manage whom); their access is the named role's.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// NamedRole is a role of the company's own.
type NamedRole struct {
	ID, Name, BaseRole string
	Access             Access // Role is the base role
	Members            int    // active or invited people given it
	Version            int
}

func namedRoleFrom(r store.NamedRole, members int32) NamedRole {
	perms := func(in []string) []api.Permission {
		out := make([]api.Permission, len(in))
		for i, p := range in {
			out[i] = api.Permission(p)
		}
		return out
	}
	a := Access{Role: r.BaseRole, Permissions: WithLaterDefaults(r.BaseRole, perms(r.Permissions), perms(r.KnownPermissions)),
		Scope: r.CandidateScope, JobCandidates: r.JobCandidates, Custom: true, Version: int(r.Version)}
	return NamedRole{ID: r.ID, Name: r.Name, BaseRole: r.BaseRole, Access: a, Members: int(members), Version: int(r.Version)}
}

func namedRoleName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	var errs apperr.FieldErrors
	switch {
	case name == "":
		errs.Add("name", "is required")
	case len([]rune(name)) > 60:
		errs.Add("name", "must be at most 60 characters")
	}
	return name, errs.Err()
}

func sameRoleName() error {
	return apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "there is already a role with this name"}})
}

func permStrings(in []api.Permission) []string {
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = string(p)
	}
	return out
}

func knownPermissions() []string { return permStrings(AllPermissions()) }

// NamedRoles lists the company's named roles.
func (m *Module) NamedRoles(ctx context.Context, p authz.Principal) ([]NamedRole, error) {
	var out []NamedRole
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListNamedRoles(ctx)
		for _, r := range rows {
			out = append(out, namedRoleFrom(store.NamedRole{ID: r.ID, TenantID: r.TenantID, Version: r.Version, Name: r.Name, BaseRole: r.BaseRole,
				Permissions: r.Permissions, KnownPermissions: r.KnownPermissions, CandidateScope: r.CandidateScope, JobCandidates: r.JobCandidates,
				UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}, r.MemberCount))
		}
		return err
	})
	return out, err
}

// CreateNamedRole adds a named role, starting from its base role's access in
// the company as it is now.
func (m *Module) CreateNamedRole(ctx context.Context, p authz.Principal, name, baseRole string) (NamedRole, error) {
	name, err := namedRoleName(name)
	if err != nil {
		return NamedRole{}, err
	}
	if !Editable(baseRole) || !validRole(baseRole) {
		return NamedRole{}, apperr.Validation(apperr.FieldErrors{{Field: "baseRole", Message: "must be admin, recruiter, sourcer, hiring_manager or viewer"}})
	}
	var out NamedRole
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		base, err := roleAccess(ctx, q, baseRole)
		if err != nil {
			return err
		}
		row, err := q.InsertNamedRole(ctx, store.InsertNamedRoleParams{Name: name, BaseRole: baseRole, Permissions: permStrings(base.Permissions),
			KnownPermissions: knownPermissions(), CandidateScope: base.Scope, JobCandidates: base.JobCandidates})
		if database.IsUniqueViolation(err, "named_role_name_idx") {
			return sameRoleName()
		}
		if err != nil {
			return err
		}
		out = namedRoleFrom(row, 0)
		return audit.Record(ctx, tx, audit.Entry{Action: "role.named_created", EntityType: "role", EntityID: row.ID,
			Changes: map[string]any{"name": name, "baseRole": baseRole}})
	})
	return out, err
}

// UpdateNamedRole renames a named role and sets its access (its base role
// stays). People given it get the change on their next request.
func (m *Module) UpdateNamedRole(ctx context.Context, p authz.Principal, id string, version int, name string, in Access) (NamedRole, error) {
	name, err := namedRoleName(name)
	if err != nil {
		return NamedRole{}, err
	}
	var out NamedRole
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		current, err := q.GetNamedRole(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("role")
		}
		if err != nil {
			return err
		}
		in.Role = current.BaseRole
		checked, err := CheckAccess(in)
		if err != nil {
			return err
		}
		perms := permStrings(checked.Permissions)
		row, err := q.UpdateNamedRole(ctx, store.UpdateNamedRoleParams{ID: id, Name: name, Permissions: perms, KnownPermissions: knownPermissions(),
			CandidateScope: checked.Scope, JobCandidates: checked.JobCandidates, Version: int32(version)}) //nolint:gosec // a version is small
		if database.IsUniqueViolation(err, "named_role_name_idx") {
			return sameRoleName()
		}
		if database.IsNoRows(err) {
			return apperr.StaleVersion("role")
		}
		if err != nil {
			return err
		}
		users, err := q.NamedRoleUsers(ctx, &id)
		if err != nil {
			return err
		}
		out = namedRoleFrom(row, int32(len(users))) //nolint:gosec // at most 20
		return audit.Record(ctx, tx, audit.Entry{Action: "role.named_changed", EntityType: "role", EntityID: id,
			Changes: map[string]any{"name": name, "permissions": perms, "candidateScope": checked.Scope, "jobCandidates": checked.JobCandidates}})
	})
	return out, err
}

// DeleteNamedRole removes a named role nobody is given.
func (m *Module) DeleteNamedRole(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetNamedRole(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("role")
		}
		if err != nil {
			return err
		}
		users, err := q.NamedRoleUsers(ctx, &id)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return apperr.Validation(apperr.FieldErrors{{Field: "role",
				Message: fmt.Sprintf("%s is still given to %s: give them another role first", row.Name, strings.Join(users, ", "))}})
		}
		if _, err := q.DeleteNamedRole(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "role.named_deleted", EntityType: "role", EntityID: id, Changes: map[string]any{"name": row.Name}})
	})
}

// namedRoleAccess is the access of a person's named role.
func namedRoleAccess(ctx context.Context, q *store.Queries, id string) (Access, error) {
	row, err := q.GetNamedRole(ctx, id)
	if err != nil {
		return Access{}, err
	}
	return namedRoleFrom(row, 0).Access, nil
}

func validRole(role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
