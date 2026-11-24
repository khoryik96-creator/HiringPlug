package app_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Showings counted at the same moment never wait on each other in a circle
// (a deadlock): each counts its candidates' lines in one order, by id, so
// one waiting for a line another holds has taken none after it.
func TestSalaryViewsAreCountedInOneOrder(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter]
	p := principalOf(t, tenant.ID, recruiter)
	ids := []string{a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti"}).Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Ali"}).Id}
	sort.Strings(ids)
	low, high := ids[0], ids[1]
	ctx := context.Background()
	seen := func(tx pgx.Tx, ids ...string) error {
		return a.modules.AuditLog.SalarySeen(ctx, tx, salaryview.Candidate, ids...)
	}

	// One showing holds the first candidate's line for today.
	held, release, holding := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holding <- database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error {
			if err := seen(tx, low); err != nil {
				close(held)
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	// Another, of both candidates (named last first), waits for that line.
	both := make(chan error, 1)
	go func() {
		both <- database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error { return seen(tx, high, low) })
	}()
	require.Eventually(t, func() bool {
		var waiting int
		err := testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%app_record_salary_view%'`).Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "the second showing waits for the first")

	// It has not taken the other candidate's line while it waits: a third
	// showing of that one goes ahead.
	third := database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '1s'"); err != nil {
			return err
		}
		return seen(tx, high)
	})
	close(release)
	require.NoError(t, <-holding)
	require.NoError(t, <-both)
	assert.NoError(t, third, "the later candidate's line was held while waiting for the earlier one")
}
