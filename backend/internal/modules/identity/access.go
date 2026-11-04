package identity

// Roles and access: each company's choice of what a role may do and which
// candidates it sees, applied when a user signs in with a token (every
// request) and to API keys acting as a role. Also teams of users.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Roles in the order the settings page shows them.
var roles = []string{
	string(api.RoleOwner), string(api.RoleAdmin), string(api.RoleRecruiter),
	string(api.RoleSourcer), string(api.RoleHiringManager), string(api.RoleViewer),
}

// TeamGroup is a team of users (e.g. a desk).
type TeamGroup struct {
	ID, Name             string
	Version, MemberCount int
}

// roleAccess is the company's access for a role: its own settings, or the
// defaults. The owner always has the defaults.
func roleAccess(ctx context.Context, q *store.Queries, role string) (Access, error) {
	if !Editable(role) {
		return DefaultAccess(role), nil
	}
	row, err := q.GetRoleSetting(ctx, role)
	if database.IsNoRows(err) {
		return DefaultAccess(role), nil
	}
	if err != nil {
		return Access{}, err
	}
	return accessFromRow(row), nil
}

// principalOf builds the principal of a signed-in user from their company's
// settings for their role: permissions, and which candidates they see.
func principalOf(ctx context.Context, q *store.Queries, u store.UserAuthRow) (authz.Principal, error) {
	a, err := roleAccess(ctx, q, u.Role)
	if err != nil {
		return authz.Principal{}, err
	}
	var team []string
	if a.Scope == ScopeTeam && u.TeamID != nil {
		if team, err = q.TeamUserIDs(ctx, u.TeamID); err != nil {
			return authz.Principal{}, err
		}
	}
	return authz.NewPrincipalWithAccess(u.TenantID, u.ID, u.Email, u.Name, api.Role(u.Role), a.Permissions, ScopeFor(a, u.ID, team)), nil
}

// RolePermissions returns the permissions a role has in tx's company, for
// API keys acting as that role (they see all candidates: they are not a
// person who owns any).
func (m *Module) RolePermissions(ctx context.Context, tx pgx.Tx, role string) ([]api.Permission, error) {
	a, err := roleAccess(ctx, store.New(tx), role)
	return a.Permissions, err
}

// AccessSettings returns every role's access in the company.
func (m *Module) AccessSettings(ctx context.Context, p authz.Principal) ([]Access, error) {
	out := make([]Access, 0, len(roles))
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		for _, r := range roles {
			a, err := roleAccess(ctx, q, r)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return nil
	})
	return out, err
}

// SetAccess saves a company's choice for a role (version 0 while it has the
// defaults). Users get it on their next request.
func (m *Module) SetAccess(ctx context.Context, p authz.Principal, in Access, version int) (Access, error) {
	in, err := CheckAccess(in)
	if err != nil {
		return Access{}, err
	}
	var out Access
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		perms := make([]string, len(in.Permissions))
		for i, perm := range in.Permissions {
			perms[i] = string(perm)
		}
		var row store.RoleSetting
		if version == 0 {
			row, err = q.InsertRoleSetting(ctx, store.InsertRoleSettingParams{Role: in.Role, Permissions: perms, CandidateScope: in.Scope, JobCandidates: in.JobCandidates})
		} else {
			row, err = q.UpdateRoleSetting(ctx, store.UpdateRoleSettingParams{
				Role: in.Role, Permissions: perms, CandidateScope: in.Scope, JobCandidates: in.JobCandidates, Version: int32(version), //nolint:gosec // a version is small
			})
		}
		if database.IsNoRows(err) {
			return apperr.StaleVersion("role settings")
		}
		if err != nil {
			return err
		}
		out = accessFromRow(row)
		return audit.Record(ctx, tx, audit.Entry{
			Action: "role.access_changed", EntityType: "role", EntityID: in.Role,
			Changes: map[string]any{"permissions": perms, "candidateScope": in.Scope, "jobCandidates": in.JobCandidates},
		})
	})
	return out, err
}

// ResetAccess gives a role its defaults back.
func (m *Module) ResetAccess(ctx context.Context, p authz.Principal, role string) (Access, error) {
	if !Editable(role) {
		return Access{}, apperr.New(api.ErrorCodeROLENOTEDITABLE, "The owner role always has every permission and sees all candidates.")
	}
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).DeleteRoleSetting(ctx, role); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "role.access_reset", EntityType: "role", EntityID: role})
	})
	return DefaultAccess(role), err
}

// Teams returns the company's teams with how many active people are in each.
func (m *Module) Teams(ctx context.Context, p authz.Principal) ([]TeamGroup, error) {
	var out []TeamGroup
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListTeamGroups(ctx)
		for _, r := range rows {
			out = append(out, TeamGroup{ID: r.ID, Name: r.Name, Version: int(r.Version), MemberCount: int(r.MemberCount)})
		}
		return err
	})
	return out, err
}

// CreateTeam adds a team.
func (m *Module) CreateTeam(ctx context.Context, p authz.Principal, name string) (TeamGroup, error) {
	name, err := teamName(name)
	if err != nil {
		return TeamGroup{}, err
	}
	var out TeamGroup
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).InsertTeamGroup(ctx, name)
		if database.IsUniqueViolation(err, "team_group_name_idx") {
			return sameTeamName()
		}
		if err != nil {
			return err
		}
		out = TeamGroup{ID: row.ID, Name: row.Name, Version: int(row.Version)}
		return audit.Record(ctx, tx, audit.Entry{Action: "team.created", EntityType: "team", EntityID: row.ID, Changes: map[string]any{"name": name}})
	})
	return out, err
}

// RenameTeam renames a team.
func (m *Module) RenameTeam(ctx context.Context, p authz.Principal, id string, version int, name string) (TeamGroup, error) {
	name, err := teamName(name)
	if err != nil {
		return TeamGroup{}, err
	}
	var out TeamGroup
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.RenameTeamGroup(ctx, store.RenameTeamGroupParams{ID: id, Name: name, Version: int32(version)}) //nolint:gosec // a version is small
		if database.IsUniqueViolation(err, "team_group_name_idx") {
			return sameTeamName()
		}
		if database.IsNoRows(err) {
			if _, err := q.GetTeamGroup(ctx, id); database.IsNoRows(err) {
				return apperr.NotFound("team")
			}
			return apperr.StaleVersion("team")
		}
		if err != nil {
			return err
		}
		out = TeamGroup{ID: row.ID, Name: row.Name, Version: int(row.Version)}
		return audit.Record(ctx, tx, audit.Entry{Action: "team.renamed", EntityType: "team", EntityID: id, Changes: map[string]any{"name": name}})
	})
	return out, err
}

// DeleteTeam removes a team; its people stay, in no team.
func (m *Module) DeleteTeam(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		n, err := store.New(tx).DeleteTeamGroup(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("team")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "team.deleted", EntityType: "team", EntityID: id})
	})
}

func teamName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		var errs apperr.FieldErrors
		errs.Add("name", "is required")
		return "", errs.Err()
	}
	return name, nil
}

func sameTeamName() error {
	var errs apperr.FieldErrors
	errs.Add("name", "there is already a team with this name")
	return errs.Err()
}

func accessFromRow(r store.RoleSetting) Access {
	perms := make([]api.Permission, len(r.Permissions))
	for i, perm := range r.Permissions {
		perms[i] = api.Permission(perm)
	}
	return Access{Role: r.Role, Permissions: perms, Scope: r.CandidateScope, JobCandidates: r.JobCandidates, Custom: true, Version: int(r.Version)}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
