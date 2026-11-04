package identity

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// TeamMember is a user as team admin shows them.
type TeamMember = store.ListTeamRow

// Invitation is a new (or renewed) invitation.
type Invitation struct {
	Member    TeamMember
	Link      string
	ExpiresAt time.Time
	Emailed   bool
}

func teamProblem(message string) error { return apperr.New(api.ErrorCodeTEAMCHANGENOTALLOWED, message) }

// Team returns everyone in the company, invited and deactivated too.
func (m *Module) Team(ctx context.Context, p authz.Principal) ([]TeamMember, error) {
	var out []TeamMember
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = store.New(tx).ListTeam(ctx)
		return err
	})
	return out, err
}

// Invite adds someone to the team as role and emails them an invitation to
// choose a password.
func (m *Module) Invite(ctx context.Context, p authz.Principal, email, name, role string) (Invitation, error) {
	email, name = normEmail(email), strings.Join(strings.Fields(name), " ")
	if problem := InviteProblem(string(p.Role), role); problem != "" {
		return Invitation{}, teamProblem(problem)
	}
	var out Invitation
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.InviteUser(ctx, store.InviteUserParams{Email: email, Name: name, Role: role})
		if database.IsUniqueViolation(err, "app_user_tenant_id_email_key") {
			return apperr.Validation(apperr.FieldErrors{{Field: "email", Message: "is already in the team"}})
		}
		if err != nil {
			return err
		}
		out, err = m.sendInvite(ctx, tx, q, p, TeamMember(row))
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.invited", EntityType: "user", EntityID: row.ID, Changes: map[string]any{"email": email, "role": role}})
	})
	return out, err
}

// ResendInvite sends someone who has not joined a new invitation.
func (m *Module) ResendInvite(ctx context.Context, p authz.Principal, userID string) (Invitation, error) {
	var out Invitation
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.TeamMember(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("user")
		}
		if err != nil {
			return err
		}
		if !row.Invited {
			return teamProblem("They have already joined: they can reset their password from the sign-in page.")
		}
		if problem := InviteProblem(string(p.Role), row.Role); problem != "" {
			return teamProblem(problem)
		}
		out, err = m.sendInvite(ctx, tx, q, p, TeamMember(row))
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.invited_again", EntityType: "user", EntityID: userID})
	})
	return out, err
}

func (m *Module) sendInvite(ctx context.Context, tx pgx.Tx, q *store.Queries, p authz.Principal, member TeamMember) (Invitation, error) {
	link, expires, err := m.link(ctx, q, member.ID, "invite", InviteValid)
	if err != nil {
		return Invitation{}, err
	}
	company, err := q.GetTenant(ctx, p.TenantID)
	if err != nil {
		return Invitation{}, err
	}
	role := strings.ReplaceAll(member.Role, "_", " ")
	body := "Hello " + member.Name + ",\n\n" + p.Name + " invited you to join " + company.Name + " on HiringPlug as " + role +
		".\n\nChoose your password here (the link works for 7 days):\n" + link + "\n\nHiringPlug"
	if err := m.mailer.Notify(ctx, tx, p.TenantID, []string{member.Email}, p.Name+" invited you to "+company.Name+" on HiringPlug", body); err != nil {
		return Invitation{}, err
	}
	return Invitation{Member: member, Link: link, ExpiresAt: expires, Emailed: true}, nil
}

// UpdateMember changes someone's role or team, or deactivates or reactivates
// them, by the team rules (ChangeProblem). Deactivating signs them out. team
// is nil to keep their team; a nil *string inside it means no team.
func (m *Module) UpdateMember(ctx context.Context, p authz.Principal, userID string, role *string, active *bool, team **string) (TeamMember, error) {
	var out TeamMember
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.TeamMember(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("user")
		}
		if err != nil {
			return err
		}
		newRole, newActive := row.Role, row.Active
		if role != nil {
			newRole = *role
		}
		if active != nil {
			newActive = *active
		}
		if row.Invited && newActive {
			return teamProblem("They have not joined yet: resend the invitation instead.")
		}
		owners, err := q.ActiveOwners(ctx)
		if err != nil {
			return err
		}
		actor := Member{ID: p.UserID, Role: string(p.Role), Active: true}
		if problem := ChangeProblem(actor, Member{ID: row.ID, Role: row.Role, Active: row.Active}, newRole, newActive, int(owners)); problem != "" {
			return teamProblem(problem)
		}
		newTeam := row.TeamID
		if team != nil {
			newTeam = *team
			if newTeam != nil {
				if _, err := q.GetTeamGroup(ctx, *newTeam); database.IsNoRows(err) {
					var errs apperr.FieldErrors
					errs.Add("teamId", "no such team")
					return errs.Err()
				} else if err != nil {
					return err
				}
			}
		}
		updated, err := q.UpdateMember(ctx, store.UpdateMemberParams{ID: userID, Role: newRole, Active: newActive, TeamID: newTeam})
		if err != nil {
			return err
		}
		out = TeamMember(updated)
		return audit.Record(ctx, tx, audit.Entry{Action: "user.updated", EntityType: "user", EntityID: userID,
			Changes: audit.Diff(map[string]any{"role": row.Role, "active": row.Active, "teamId": deref(row.TeamID)},
				map[string]any{"role": newRole, "active": newActive, "teamId": deref(newTeam)})})
	})
	return out, err
}

// Status is how team admin shows a user: invited, active or deactivated.
func Status(member TeamMember) string {
	switch {
	case member.Invited:
		return "invited"
	case member.Active:
		return "active"
	default:
		return "deactivated"
	}
}
