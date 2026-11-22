package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Two records of one company ("Acme Sdn Bhd" and "ACME Berhad", the same
// website) are found and merged: everything about the duplicate moves to the
// company kept, its empty fields are filled, and the duplicate is gone.
func TestCompanyMergeMovesEverythingToTheCompanyKept(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	keep := a.createCompany(token, map[string]any{"name": "Acme Sdn Bhd", "website": "https://www.acme-merge.example/"})
	dup := a.createCompany(token, map[string]any{"name": "ACME Berhad", "website": "acme-merge.example/about", "city": "Penang", "phone": "+60 4-111 2222"})
	other := a.createCompany(token, map[string]any{"name": "Unrelated Trading"})

	var similar api.SimilarCompanies
	a.request(http.MethodGet, "/v1/companies/"+keep.Id+"/similar", token, nil).JSON(&similar)
	require.Len(t, similar.Items, 1)
	assert.Equal(t, dup.Id, similar.Items[0].Id)
	assert.ElementsMatch(t, []api.CompanyDuplicateReason{api.CompanyDuplicateReasonSameWebsite, api.CompanyDuplicateReasonSimilarName}, similar.Items[0].Reasons)
	var groups api.CompanyDuplicateGroups
	a.request(http.MethodGet, "/v1/companies/duplicates", token, nil).JSON(&groups)
	require.Len(t, groups.Items, 1, "one group, found by two reasons")
	assert.Len(t, groups.Items[0].Companies, 2)

	hr := a.createContact(token, map[string]any{"companyId": dup.Id, "firstName": "Aisyah", "lastName": "Rahman"})
	job := a.createJob(token, map[string]any{"companyId": dup.Id, "contactId": hr.Id, "title": "Finance Manager"})
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", token, map[string]any{"subjectType": "company", "subjectId": dup.Id, "body": "Met their CFO."}).Status)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/companies/"+dup.Id+"/terms", token, map[string]any{
		"version": 0, "tiers": []any{rate(nil, nil, "20", 30, 90, nil)}, "salaryCurrency": "MYR", "signedOn": nil, "notes": nil,
	}).Status)

	merge := func(who, into, from string, version int) *response {
		return a.request(http.MethodPost, "/v1/companies/"+into+"/merge", who, map[string]any{"duplicateId": from, "version": version})
	}
	assert.Equal(t, http.StatusForbidden, merge(tenant.Users[api.RoleViewer].Token, keep.Id, dup.Id, keep.Version).Status, "viewers may not merge")
	assert.Equal(t, http.StatusConflict, merge(token, keep.Id, dup.Id, keep.Version+1).Status, "a stale version is refused")
	assert.Equal(t, []string{"duplicateId"}, fieldNames(t, merge(token, keep.Id, keep.Id, keep.Version).Error()))

	res := merge(token, keep.Id, dup.Id, keep.Version)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var merged api.Company
	res.JSON(&merged)
	assert.Equal(t, "Acme Sdn Bhd", merged.Name, "the name kept")
	assert.Equal(t, "Penang", merged.City.MustGet(), "an empty field filled from the duplicate")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/companies/"+dup.Id, token, nil).Status, "the duplicate is gone")

	var c api.Contact
	a.request(http.MethodGet, "/v1/contacts/"+hr.Id, token, nil).JSON(&c)
	assert.Equal(t, keep.Id, c.CompanyId, "the contact moved")
	var j api.JobOrder
	a.request(http.MethodGet, "/v1/jobs/"+job.Id, token, nil).JSON(&j)
	assert.Equal(t, keep.Id, j.CompanyId, "the job moved")
	var notes api.NoteList
	a.request(http.MethodGet, "/v1/notes?subjectType=company&subjectId="+keep.Id, token, nil).JSON(&notes)
	assert.Len(t, notes.Items, 1, "the note moved")
	var terms api.CompanyTerms
	a.request(http.MethodGet, "/v1/companies/"+keep.Id+"/terms", token, nil).JSON(&terms)
	assert.Len(t, terms.Tiers, 1, "the terms moved, the company kept having none")

	a.request(http.MethodGet, "/v1/companies/duplicates", token, nil).JSON(&groups)
	assert.Empty(t, groups.Items, "no duplicates left")
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/companies/"+other.Id, token, nil).Status)
	var activity api.ActivityList
	a.request(http.MethodGet, "/v1/activity?entityType=company&entityId="+keep.Id, token, nil).JSON(&activity)
	require.NotEmpty(t, activity.Items)
	assert.Equal(t, "company.merged", activity.Items[0].Action)
}
