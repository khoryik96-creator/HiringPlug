package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// An agency's first look at the industries links the records it already has
// to the catalogue by their old labels, a statement per label: thousands of
// records must not take a statement (and a catalogue read) each.
func TestIndustriesSeedLinksExistingRecordsByLabel(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner]
	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx, `
		INSERT INTO candidate (tenant_id, first_name, industry, created_by)
		SELECT $1, 'Seed ' || g, CASE WHEN g % 3 = 0 THEN 'Underwater Welding' ELSE 'Education' END, $2
		FROM generate_series(1, 3000) g`, tenant.ID, owner.ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `
		INSERT INTO company (tenant_id, name, industry, created_by) VALUES ($1, 'Seed school', 'Education', $2), ($1, 'Seed welders', 'Underwater Welding', $2)`,
		tenant.ID, owner.ID)
	require.NoError(t, err)

	start := time.Now()
	c := industryCatalogue(t, a, owner.Token)
	// A statement per record took over a minute here (hours for a million).
	assert.Less(t, time.Since(start), 15*time.Second, "linking 3,000 records")
	welding := industryNamed(t, c, "Underwater Welding")
	assert.True(t, welding.Retired, "an unknown old label stays for the records that had it")
	assert.Equal(t, 1000, welding.CandidateCount)
	assert.Equal(t, 1, welding.CompanyCount)
	education := industryNamed(t, c, "Education")
	assert.Equal(t, 2000, education.CandidateCount)
	assert.Equal(t, 1, education.CompanyCount)

	r := a.request("POST", "/v1/candidates/search", owner.Token, map[string]any{"industry": []string{welding.Id}, "limit": 100})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var page api.CandidateSearchPage
	r.JSON(&page)
	assert.Len(t, page.Items, 100)
	assert.NotNil(t, page.NextCursor)
}

// A group counts each record once, however many of its subindustries the
// record has; only those who manage Settings see the counts.
func TestIndustryCountsRollUpOnce(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, owner)
	bank, insurance := industryNamed(t, c, "FSI – Banking"), industryNamed(t, c, "FSI - Insurance")
	require.NotNil(t, bank.ParentId)
	require.Equal(t, bank.ParentId, insurance.ParentId)
	r := a.request("POST", "/v1/candidates", owner, map[string]any{"firstName": "Synthetic Two Subindustries",
		"industrySelections": map[string]any{"catalogueVersion": c.Version, "ids": []string{bank.Id, insurance.Id}, "primaryId": bank.Id}})
	require.Equal(t, 201, r.Status, "%s", r.Body)

	c = industryCatalogue(t, a, owner)
	assert.Equal(t, 1, industryNamed(t, c, "FSI – Banking").CandidateCount)
	assert.Equal(t, 1, industryNamed(t, c, "FSI - Insurance").CandidateCount)
	for _, i := range c.Items {
		if i.Id == *bank.ParentId {
			assert.Equal(t, 1, i.CandidateCount, "the group counts the candidate once")
		}
	}
	for _, i := range industryCatalogue(t, a, tenant.Users[api.RoleRecruiter].Token).Items {
		assert.Zero(t, i.CandidateCount, "no counts without Settings rights")
	}
	// The dropdowns read the catalogue without counts (slow with many records).
	r = a.request("GET", "/v1/industries", owner, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var plain api.IndustryCatalogue
	r.JSON(&plain)
	for _, i := range plain.Items {
		assert.Zero(t, i.CandidateCount, "counted only when asked")
	}
}
