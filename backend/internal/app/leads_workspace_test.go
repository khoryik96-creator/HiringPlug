package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A lead's favourites and saved ad text can only refer to a lead of the same
// company: the database refuses another tenant's lead even to the server's
// own connection (RLS does not apply to foreign keys).
func TestLeadFavouritesAndEvidenceStayWithinTheTenant(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	mine := testutil.Current.NewTenant(t, api.RoleRecruiter)
	theirs := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := mine.Users[api.RoleRecruiter].ID
	lead := func(tenant string) string {
		var id string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `INSERT INTO job_lead (tenant_id, company_name, title, created_by)
			VALUES ($1, 'Maju Bhd', 'Accountant', 'test') RETURNING id`, tenant).Scan(&id))
		return id
	}
	own, other := lead(mine.ID), lead(theirs.ID)

	_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO job_lead_favorite (tenant_id, user_id, lead_id) VALUES ($1, $2, $3)`, mine.ID, user, other)
	assert.Error(t, err, "a favourite of another tenant's lead")
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO job_lead_evidence (tenant_id, lead_id, description) VALUES ($1, $2, 'ad')`, mine.ID, other)
	assert.Error(t, err, "evidence for another tenant's lead, which would take its slot")

	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO job_lead_favorite (tenant_id, user_id, lead_id) VALUES ($1, $2, $3)`, mine.ID, user, own)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO job_lead_evidence (tenant_id, lead_id, description) VALUES ($1, $2, 'ad')`, mine.ID, own)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO job_lead_evidence (tenant_id, lead_id, description) VALUES ($1, $2, 'ad')`, theirs.ID, other)
	require.NoError(t, err, "their own lead's evidence is theirs to write")
}
