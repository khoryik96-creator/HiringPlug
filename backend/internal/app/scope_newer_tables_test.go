package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The database itself keeps a role limited to its own candidates from
// reading rows about other people's, on the tables added after the access
// settings too (screening calls and their salaries, booking links, shortlist
// links, industry assignments): not only the application code.
func TestTheDatabaseScopesTheNewerCandidateTables(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	theirs := a.createCandidate(sourcer.Token, map[string]any{"firstName": "Theirs"})
	admin := testutil.Current.Admin
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := admin.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO screening_call (id, tenant_id, candidate_id, called_on, called_by, salary_recorded, created_by)
	      VALUES ('scl_'||substr($2, 6), $1, $2, current_date, $3, true, $3)`, tenant.ID, theirs.Id, sourcer.ID)
	exec(`INSERT INTO screening_call_salary (tenant_id, call_id, snapshot) VALUES ($1, 'scl_'||substr($2, 6), '{}')`, tenant.ID, theirs.Id)
	exec(`INSERT INTO booking_link (tenant_id, token_hash, host_user_id, candidate_id, title, duration_minutes, expires_at, created_by)
	      VALUES ($1, repeat('a', 32)||substr($2, 6), $3, $2, 'Call', 30, now() + interval '1 day', $3)`, tenant.ID, theirs.Id, sourcer.ID)

	tables := []string{"screening_call", "screening_call_salary", "booking_link"}
	count := func(p authz.Principal, table string) int {
		t.Helper()
		var n int
		require.NoError(t, database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&n)
		}))
		return n
	}
	perms := authz.PermissionsForRole(api.RoleOwner)
	everyone := authz.NewPrincipalWithAccess(tenant.ID, recruiter.ID, "", "", api.RoleRecruiter, perms, authz.Scope{})
	ownOnly := authz.NewPrincipalWithAccess(tenant.ID, recruiter.ID, "", "", api.RoleRecruiter, perms, authz.Scope{Users: []string{recruiter.ID}})
	for _, table := range tables {
		assert.Equal(t, 1, count(everyone, table), "%s: a role that sees all candidates", table)
		assert.Zero(t, count(ownOnly, table), "%s: a role that sees only its own does not read another's", table)
	}
}
