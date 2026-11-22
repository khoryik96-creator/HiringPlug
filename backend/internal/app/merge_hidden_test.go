package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A merge moves what the person merging cannot see too: records in the Bin
// (which used to block the merge for everyone) and a colleague's Apollo
// lookups (which used to be deleted with the duplicate).
func TestCompanyMergeTakesTheBinAndColleaguesLookups(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	keep := a.createCompany(recruiter.Token, map[string]any{"name": "Binned Merge Sdn Bhd"})
	dup := a.createCompany(recruiter.Token, map[string]any{"name": "Binned Merge Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": dup.Id, "title": "Old role"})
	gone := a.createContact(recruiter.Token, map[string]any{"companyId": dup.Id, "firstName": "Left", "lastName": "Long Ago"})
	require.Equal(t, http.StatusCreated, a.bin(admin.Token, map[string]any{"kind": "contact", "id": gone.Id}).Status)
	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job_order SET binned_at = now() WHERE id = $1`, job.Id) // jobs go to the Bin with their company
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO hr_apollo_lookup (tenant_id, user_id, id, flow_id, company_id, linkedin_company_id,
		profile_url, person_name, linkedin_company_name, linkedin_company_url, state)
		VALUES ($1, $2, repeat('a', 64), 'flow', $3, '123', 'https://www.linkedin.com/in/x', 'X', 'Merge', 'https://www.linkedin.com/company/x', 'found')`,
		tenant.ID, admin.ID, dup.Id)
	require.NoError(t, err)

	res := a.request(http.MethodPost, "/v1/companies/"+keep.Id+"/merge", recruiter.Token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var jobCompany, contactCompany, lookupCompany string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT company_id FROM job_order WHERE id = $1`, job.Id).Scan(&jobCompany))
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT company_id FROM contact WHERE id = $1`, gone.Id).Scan(&contactCompany))
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT company_id FROM hr_apollo_lookup WHERE tenant_id = $1`, tenant.ID).Scan(&lookupCompany))
	assert.Equal(t, keep.Id, jobCompany, "the binned job moved")
	assert.Equal(t, keep.Id, contactCompany, "the binned contact moved")
	assert.Equal(t, keep.Id, lookupCompany, "the colleague's Apollo lookup moved, not deleted")
}

// A colleague's Outlook thread linked to the duplicate follows it (it used
// to be deleted with the duplicate); joined notes too long for a contact are
// refused before anything moves; colleagues who share the office line are
// not duplicates, the same mobile is.
func TestContactMergeKeepsColleaguesThreadsAndLongNotes(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	co := a.createCompany(recruiter.Token, map[string]any{"name": "Thread Merge Berhad"})
	keep := a.createContact(recruiter.Token, map[string]any{"companyId": co.Id, "firstName": "Farah", "lastName": "Ismail",
		"phone": "+60 3-2222 1111", "mobile": "+60 19-555 0101", "notes": strings.Repeat("k", 2500)})
	dup := a.createContact(recruiter.Token, map[string]any{"companyId": co.Id, "firstName": "Farah", "lastName": "Ismail (HR)",
		"phone": "+60 3-2222 1111", "notes": strings.Repeat("d", 2000)})
	colleague := a.createContact(recruiter.Token, map[string]any{"companyId": co.Id, "firstName": "Wong", "lastName": "Kar Wai", "phone": "+60 3-2222 1111"})
	twin := a.createContact(recruiter.Token, map[string]any{"companyId": co.Id, "firstName": "F.", "lastName": "Binti Ismail", "mobile": "60195550101"})

	var similar api.SimilarContacts
	a.request(http.MethodGet, "/v1/contacts/"+keep.Id+"/similar", recruiter.Token, nil).JSON(&similar)
	reasons := map[string][]api.ContactDuplicateReason{}
	for _, s := range similar.Items {
		reasons[s.Id] = s.Reasons
	}
	assert.NotContains(t, reasons, colleague.Id, "the shared office line is not a reason")
	assert.Contains(t, reasons[twin.Id], api.ContactDuplicateReasonSamePhone, "the same mobile is")

	res := a.request(http.MethodPost, "/v1/contacts/"+keep.Id+"/merge", recruiter.Token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	assert.Equal(t, []string{"duplicateId"}, fieldNames(t, res.Error()), "4,500 characters of notes together")
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/contacts/"+dup.Id, recruiter.Token, nil).Status, "nothing moved")

	ctx := context.Background()
	_, err := testutil.Current.Admin.Exec(ctx, `
		WITH m AS (INSERT INTO email_mailbox (tenant_id, user_id, provider, address, sealed_secret) VALUES ($1, $2, 'microsoft', 'admin@example.com', 'x') RETURNING id),
		     s AS (INSERT INTO email_outlook_sync (mailbox_id, tenant_id, user_id, generation) SELECT id, $1, $2, '1' FROM m RETURNING mailbox_id)
		INSERT INTO email_outlook_link (mailbox_id, tenant_id, user_id, conversation, contact_id) SELECT mailbox_id, $1, $2, 'thread-1', $3 FROM s`,
		tenant.ID, admin.ID, twin.Id)
	require.NoError(t, err)
	res = a.request(http.MethodPost, "/v1/contacts/"+keep.Id+"/merge", recruiter.Token, map[string]any{"duplicateId": twin.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var linked string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT contact_id FROM email_outlook_link WHERE tenant_id = $1`, tenant.ID).Scan(&linked))
	assert.Equal(t, keep.Id, linked, "the colleague's thread moved, not deleted")
}
