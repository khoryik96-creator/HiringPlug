package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) bin(token string, body map[string]any) *response {
	a.t.Helper()
	return a.request(http.MethodPost, "/v1/bin", token, body)
}

func (a *testApp) binList(token string) []api.BinEntry {
	a.t.Helper()
	var list api.BinList
	res := a.request(http.MethodGet, "/v1/bin", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	return list.Items
}

// purgeNow makes the Bin's 30 days pass for this tenant and runs the purge jobs.
func purgeNow(t *testing.T, a *testApp, tenantID string) {
	t.Helper()
	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE bin_entry SET purge_after = now() - interval '1 minute' WHERE tenant_id = $1`, tenantID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'bin.purge' AND status = 'pending'`, tenantID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenantID)
}

func TestOnlyAdminsBinACandidateWhoThenDisappearsEverywhereUntilRestored(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	email := "siti." + tenant.ID + "@mail.test"
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": email})
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Accountant"})
	a.addToJob(recruiter, job.Id, siti.Id)

	assert.Equal(t, http.StatusForbidden, a.bin(recruiter, map[string]any{"kind": "candidate", "id": siti.Id}).Status, "admins only")
	var check api.BinCheck
	res := a.request(http.MethodGet, "/v1/bin/check?kind=candidate&id="+siti.Id, admin, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&check)
	assert.Equal(t, "Siti Aisyah", check.Label)
	assert.Equal(t, 1, check.Applications)
	assert.True(t, check.Blocked.IsNull())

	res = a.bin(admin, map[string]any{"kind": "candidate", "id": siti.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var entry api.BinEntry
	res.JSON(&entry)
	assert.Equal(t, "Siti Aisyah", entry.Label)
	assert.WithinDuration(t, entry.BinnedAt.AddDate(0, 0, 30), entry.PurgeAfter, time.Second)

	// Gone everywhere: the candidate, lists, the job's pipeline.
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+siti.Id, recruiter, nil).Status)
	assert.NotContains(t, string(a.request(http.MethodGet, "/v1/candidates", recruiter, nil).Body), siti.Id)
	assert.NotContains(t, string(a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).Body), siti.Id)
	assert.Equal(t, http.StatusNotFound, a.bin(admin, map[string]any{"kind": "candidate", "id": siti.Id}).Status, "already in the Bin")
	// Their email is free while they are in the Bin; restoring then says so.
	twin := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "email": email})
	res = a.request(http.MethodPost, "/v1/bin/"+entry.Id+"/restore", admin, nil)
	assert.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeBINRESTORECONFLICT, res.Error().Error.Code)
	a.request(http.MethodPatch, "/v1/candidates/"+twin.Id, recruiter, map[string]any{"version": twin.Version, "email": nil})

	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/bin/"+entry.Id+"/restore", admin, nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+siti.Id, recruiter, nil).Status)
	assert.Contains(t, string(a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).Body), siti.Id, "back in the pipeline")
	assert.Empty(t, a.binList(admin))

	var actions []string
	rows, err := testutil.Current.Admin.Query(context.Background(), `SELECT action FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 ORDER BY id`, tenant.ID, siti.Id)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		actions = append(actions, s)
	}
	assert.Subset(t, actions, []string{"candidate.binned", "candidate.restored"}, "recorded")
}

func TestABinnedCandidateIsDeletedForGoodAfter30Days(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	admin := tenant.Users[api.RoleAdmin].Token
	siti := a.createCandidate(admin, map[string]any{"firstName": "Siti"})
	company := a.createCompany(admin, map[string]any{"name": "Contoh"})
	app := a.addToJob(admin, a.createJob(admin, map[string]any{"companyId": company.Id, "title": "Accountant"}).Id, siti.Id)
	a.request(http.MethodPost, "/v1/candidates/"+siti.Id+"/notes", admin, map[string]any{"body": "Good call"})
	res := a.bin(admin, map[string]any{"kind": "candidate", "id": siti.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)

	purgeNow(t, a, tenant.ID)
	var left int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM candidate WHERE id = $1) + (SELECT count(*) FROM application WHERE id = $2)
		+ (SELECT count(*) FROM note WHERE subject_id = $1) + (SELECT count(*) FROM bin_entry WHERE record_id = $1)`, siti.Id, app.Id).Scan(&left))
	assert.Zero(t, left, "the candidate, their pipeline entry, notes and the Bin entry are gone")
}

// What the AI said about a candidate goes with them, and the audit log, kept
// for good, never names them.
func TestADeletedCandidateLeavesNoAIAnswersOrNameBehind(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	admin := tenant.Users[api.RoleAdmin]
	siti := a.createCandidate(admin.Token, map[string]any{"firstName": "Siti", "lastName": "Zulaikha"})
	company := a.createCompany(admin.Token, map[string]any{"name": "Contoh"})
	job := a.createJob(admin.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(admin.Token, job.Id, siti.Id)
	for _, s := range [][2]string{{"candidate", siti.Id}, {"application", app.Id}, {"job", job.Id}} {
		_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO assistant_answer (tenant_id, user_id, subject_type, subject_id, kind, status, answer)
			VALUES ($1, $2, $3, $4, 'summary', 'done', 'Siti led the month-end close.')`, tenant.ID, admin.ID, s[0], s[1])
		require.NoError(t, err)
	}
	require.Equal(t, http.StatusCreated, a.bin(admin.Token, map[string]any{"kind": "candidate", "id": siti.Id}).Status)
	purgeNow(t, a, tenant.ID)

	var left []string
	rows, err := testutil.Current.Admin.Query(ctx, `SELECT subject_type FROM assistant_answer WHERE tenant_id = $1 ORDER BY subject_type`, tenant.ID)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		left = append(left, s)
	}
	assert.Equal(t, []string{"job"}, left, "the candidate's and their application's answers are gone; the job stays")
	var named int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND changes::text LIKE '%Zulaikha%'`, tenant.ID).Scan(&named))
	assert.Zero(t, named, "no audit entry names them")
}

func TestACompanysContactsAndJobsGoWithItOrMoveToTheCompanyChosen(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	old := a.createCompany(owner, map[string]any{"name": "Old Name Sdn Bhd"})
	other := a.createCompany(owner, map[string]any{"name": "New Name Sdn Bhd"})
	hr := a.createContact(owner, map[string]any{"companyId": old.Id, "firstName": "Aisha", "email": "aisha." + tenant.ID + "@old.test"})
	job := a.createJob(owner, map[string]any{"companyId": old.Id, "title": "Auditor"})

	var check api.BinCheck
	a.request(http.MethodGet, "/v1/bin/check?kind=company&id="+old.Id, owner, nil).JSON(&check)
	assert.Equal(t, 1, check.Contacts)
	assert.Equal(t, 1, check.Jobs)

	// Moved: they stay, at the chosen company.
	res := a.bin(owner, map[string]any{"kind": "company", "id": old.Id, "moveTo": other.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	assert.Equal(t, other.Id, a.contact(owner, hr.Id).CompanyId)
	var j api.JobOrder
	a.request(http.MethodGet, "/v1/jobs/"+job.Id, owner, nil).JSON(&j)
	assert.Equal(t, other.Id, j.CompanyId)

	// Not moved: they go to the Bin with it, and come back with it.
	res = a.bin(owner, map[string]any{"kind": "company", "id": other.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var entry api.BinEntry
	res.JSON(&entry)
	assert.Equal(t, 1, entry.Contacts)
	assert.Equal(t, 1, entry.Jobs)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/contacts/"+hr.Id, owner, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/"+job.Id, owner, nil).Status)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/bin/"+entry.Id+"/restore", owner, nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/jobs/"+job.Id, owner, nil).Status)

	// Deleted for good, with them.
	res = a.bin(owner, map[string]any{"kind": "company", "id": other.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	purgeNow(t, a, tenant.ID)
	var left int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM company WHERE id IN ($1, $2))
		+ (SELECT count(*) FROM contact WHERE id = $3) + (SELECT count(*) FROM job_order WHERE id = $4)`, old.Id, other.Id, hr.Id, job.Id).Scan(&left))
	assert.Zero(t, left, "both companies (each 30 days in the Bin), the contact and the job are gone")
}

func TestARecordWithAPlacementIsNeverBinned(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin)
	admin := tenant.Users[api.RoleAdmin].Token
	company := a.createCompany(admin, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(admin, map[string]any{"companyId": company.Id, "title": "Accountant"})
	siti := a.createCandidate(admin, map[string]any{"firstName": "Siti"})
	app := a.addToJob(admin, job.Id, siti.Id)
	a.placement(http.MethodPost, "/v1/applications/"+app.Id+"/placement", admin, map[string]any{
		"startDate": "2026-12-01",
		"money":     map[string]any{"salary": map[string]any{"amount": "8500.00", "currency": "MYR", "period": "month"}, "feePercent": "20"},
	}, http.StatusCreated)

	for _, body := range []map[string]any{{"kind": "candidate", "id": siti.Id}, {"kind": "company", "id": company.Id}} {
		res := a.bin(admin, body)
		assert.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeBINBLOCKED, res.Error().Error.Code)
	}
	var check api.BinCheck
	a.request(http.MethodGet, "/v1/bin/check?kind=candidate&id="+siti.Id, admin, nil).JSON(&check)
	assert.Contains(t, check.Blocked.MustGet(), "placement")
	// Moving the company's jobs elsewhere first lets the company go.
	other := a.createCompany(admin, map[string]any{"name": "Contoh Group"})
	assert.Equal(t, http.StatusCreated, a.bin(admin, map[string]any{"kind": "company", "id": company.Id, "moveTo": other.Id}).Status)
}
