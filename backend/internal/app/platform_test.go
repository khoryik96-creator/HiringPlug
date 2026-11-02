package app_test

// Tests for the guarantees every module relies on: sign-in, permissions,
// request IDs, tenant isolation in the database itself, and the wiring checks
// that stop a new module from being half-connected.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestHealthAndDocs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodGet, "/health/live", "", nil).Status)
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodGet, "/health/ready", "", nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/docs", "", nil).Status)
	spec := a.request(http.MethodGet, "/openapi.json", "", nil).Map()
	assert.Contains(t, spec["paths"], "/v1/candidates")
}

func TestSignInIsRequired(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]

	expired, err := testutil.Verifier().Mint(tenant.ID, user.ID, -time.Minute)
	require.NoError(t, err)
	otherSecret, err := authn.NewVerifier([]byte("some-other-secret-0123456789abcdefgh"), "hiringplug", "hiringplug-api").Mint(tenant.ID, user.ID, time.Hour)
	require.NoError(t, err)
	strangerID := "usr_01jbz6w0m0k8q9x2r3s4t5v6w7"
	stranger, err := testutil.Verifier().Mint(tenant.ID, strangerID, time.Hour)
	require.NoError(t, err)

	for name, token := range map[string]string{
		"no token": "", "garbage": "not-a-jwt", "expired": expired, "wrong key": otherSecret, "unknown user": stranger,
	} {
		t.Run(name, func(t *testing.T) {
			res := a.request(http.MethodGet, "/v1/candidates", token, nil)
			require.Equal(t, http.StatusUnauthorized, res.Status)
			assert.Equal(t, api.ErrorCodeUNAUTHENTICATED, res.Error().Error.Code)
		})
	}

	t.Run("a deactivated user is signed out at once", func(t *testing.T) {
		_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE app_user SET active = false WHERE id = $1`, user.ID)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/me", user.Token, nil).Status)
	})
}

func TestMeListsTheRolePermissions(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)

	var me api.Me
	a.request(http.MethodGet, "/v1/me", tenant.Users[api.RoleRecruiter].Token, nil).JSON(&me)
	assert.Equal(t, api.RoleRecruiter, me.Role)
	assert.Contains(t, me.Permissions, api.PermissionCandidatesCompensationRead)

	a.request(http.MethodGet, "/v1/me", tenant.Users[api.RoleSourcer].Token, nil).JSON(&me)
	assert.NotContains(t, me.Permissions, api.PermissionCandidatesCompensationRead)
	assert.Contains(t, me.Permissions, api.PermissionCandidatesCreate)
}

func TestRequestIDsAndUnknownRoutes(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)

	res := a.request(http.MethodGet, "/v1/nope", "", nil, "X-Request-Id", "support-ticket-42")
	assert.Equal(t, http.StatusNotFound, res.Status)
	assert.Equal(t, "support-ticket-42", res.Error().Error.RequestId, "a caller's request ID is kept")

	res = a.request(http.MethodDelete, "/v1/candidates", "", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, res.Status)
	assert.Equal(t, api.ErrorCodeMETHODNOTALLOWED, res.Error().Error.Code)

	res = a.request(http.MethodGet, "/v1/me", "", nil, "X-Request-Id", "bad id with spaces")
	assert.Regexp(t, `^req_[0-9a-z]{26}$`, res.Header.Get("X-Request-Id"), "unsafe IDs are replaced")
}

// TestEveryTenantTableHasRowLevelSecurity is the guard that stops a new table
// from accidentally being shared between tenants.
// A table with row-level security but no policy for a command silently
// changes nothing for it (no error): e.g. moving notes when merging
// candidates. Every command must be covered, or the gap listed here with why.
func TestRowLevelSecurityCoversEveryCommand(t *testing.T) {
	t.Parallel()
	exempt := map[string]string{} // "table:COMMAND": why it needs no policy
	rows, err := testutil.Current.Admin.Query(context.Background(), `
		SELECT c.relname, cmd
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN (VALUES ('SELECT'), ('INSERT'), ('UPDATE'), ('DELETE')) AS cmds(cmd)
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relrowsecurity
		  AND NOT EXISTS (SELECT 1 FROM pg_policies p WHERE p.tablename = c.relname AND p.cmd IN (cmds.cmd, 'ALL'))
		ORDER BY 1, 2`)
	require.NoError(t, err)
	missing, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (string, error) {
		var table, cmd string
		return table + ":" + cmd, r.Scan(&table, &cmd)
	})
	require.NoError(t, err)
	var unexplained []string
	for _, m := range missing {
		if exempt[m] == "" {
			unexplained = append(unexplained, m)
		}
	}
	assert.Empty(t, unexplained, "add a policy for these commands (table:command)")
}

func TestEveryTenantTableHasRowLevelSecurity(t *testing.T) {
	t.Parallel()
	// Tables without RLS on purpose, and why.
	exempt := map[string]string{
		"job": "workers serve every tenant; each job runs in a transaction scoped to its tenant",
	}
	rows, err := testutil.Current.Admin.Query(context.Background(), `
		SELECT c.relname, c.relrowsecurity,
		       EXISTS (SELECT 1 FROM pg_policies p WHERE p.tablename = c.relname)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		  AND EXISTS (SELECT 1 FROM information_schema.columns col
		              WHERE col.table_name = c.relname AND col.column_name = 'tenant_id')`)
	require.NoError(t, err)
	type row struct {
		name             string
		rls, hasPolicies bool
	}
	tables, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		return x, r.Scan(&x.name, &x.rls, &x.hasPolicies)
	})
	require.NoError(t, err)
	require.NotEmpty(t, tables)
	for _, tbl := range tables {
		if _, ok := exempt[tbl.name]; ok {
			continue
		}
		assert.True(t, tbl.rls && tbl.hasPolicies,
			"table %q has a tenant_id but no row-level security; add ENABLE ROW LEVEL SECURITY and a tenant_isolation policy in its migration", tbl.name)
	}
}

func TestTheDatabaseEnforcesIsolationEvenForRawSQL(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	c := a.createCandidate(tenant.Users[api.RoleRecruiter].Token, map[string]any{"firstName": "Raw"})
	res := a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", tenant.Users[api.RoleRecruiter].Token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "5000", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	})
	require.Equal(t, http.StatusOK, res.Status)
	pool := testutil.Current.App

	count := func(p authz.Principal, table string) int {
		var n int
		err := database.InTx(ctx, pool, p, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE candidate_id = $1 OR $1 = ''", c.Id).Scan(&n)
		})
		require.NoError(t, err)
		return n
	}
	t.Run("no tenant set: nothing is visible", func(t *testing.T) {
		var n int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM candidate").Scan(&n))
		assert.Zero(t, n)
	})
	t.Run("another tenant: nothing is visible", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleOwner)
		assert.Zero(t, count(authz.System(other.ID), "candidate_compensation"))
	})
	t.Run("same tenant without the salary permission: salary rows are invisible", func(t *testing.T) {
		assert.Zero(t, count(authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleSourcer), "candidate_compensation"))
	})
	t.Run("same tenant with the salary permission: visible", func(t *testing.T) {
		assert.Equal(t, 1, count(authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleRecruiter), "candidate_compensation"))
	})
	t.Run("writing into another tenant is refused", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleOwner)
		err := database.InTx(ctx, pool, authz.System(tenant.ID), func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO candidate (tenant_id, first_name, created_by) VALUES ($1, 'Sneaky', 'x')`, other.ID)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "row-level security")
	})
}

func TestTheAppRefusesPrivilegedDatabaseRoles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	require.NoError(t, database.CheckRestrictedRole(ctx, testutil.Current.App))
	err := database.CheckRestrictedRole(ctx, testutil.Current.Admin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bypasses tenant isolation")
}

// TestEveryCoreJobHasAHandler stops a module from declaring a job in the
// contract that the Go worker cannot run.
func TestEveryCoreJobHasAHandler(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	handlers, err := a.modules.JobHandlers()
	require.NoError(t, err)
	for _, kind := range jobqueue.KindsForQueue(jobqueue.QueueCore) {
		assert.Contains(t, handlers, kind, "job %q (queue core) has no Go handler; add it to the module's JobHandlers", kind)
	}
	for kind := range handlers {
		assert.Equal(t, jobqueue.QueueCore, jobqueue.Kinds()[kind].Queue, "Go handles %q but the contract puts it on another queue", kind)
	}
}
