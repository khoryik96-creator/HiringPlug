package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Two records of one person at a company ("Dato' Aisyah Rahman" and
// "Rahman Aisyah") are found and merged: the duplicate's jobs, notes and
// email move to the contact kept, and the duplicate is gone. Contacts at
// different companies are not merged (they may have changed jobs).
func TestContactMergeMovesEverythingToTheContactKept(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	co := a.createCompany(token, map[string]any{"name": "Merge Foods Berhad"})
	elsewhere := a.createCompany(token, map[string]any{"name": "Elsewhere Holdings"})
	keep := a.createContact(token, map[string]any{"companyId": co.Id, "firstName": "Dato' Aisyah", "lastName": "Rahman", "title": "HR Director"})
	dup := a.createContact(token, map[string]any{"companyId": co.Id, "firstName": "Rahman", "lastName": "Aisyah",
		"email": "aisyah.merge@example.com", "mobile": "+60 12-345 6789", "notes": "Prefers WhatsApp."})
	moved := a.createContact(token, map[string]any{"companyId": elsewhere.Id, "firstName": "Aisyah", "lastName": "Rahman"})

	var similar api.SimilarContacts
	a.request(http.MethodGet, "/v1/contacts/"+keep.Id+"/similar", token, nil).JSON(&similar)
	ids := []string{}
	for _, s := range similar.Items {
		ids = append(ids, s.Id)
		assert.Contains(t, s.Reasons, api.ContactDuplicateReasonSameName)
		assert.NotContains(t, s.Reasons, api.ContactDuplicateReasonSimilarName, "the same name is not also a similar one")
	}
	assert.ElementsMatch(t, []string{dup.Id, moved.Id}, ids, "the same name, here and at another company")
	var groups api.ContactDuplicateGroups
	a.request(http.MethodGet, "/v1/contacts/duplicates", token, nil).JSON(&groups)
	require.Len(t, groups.Items, 1, "only the two at the same company")
	assert.Len(t, groups.Items[0].Contacts, 2)
	assert.Equal(t, "Merge Foods Berhad", groups.Items[0].Contacts[0].CompanyName.MustGet())

	job := a.createJob(token, map[string]any{"companyId": co.Id, "contactId": dup.Id, "title": "Payroll Executive"})
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", token, map[string]any{"subjectType": "contact", "subjectId": dup.Id, "body": "Called back."}).Status)

	merge := func(into, from string, version int) *response {
		return a.request(http.MethodPost, "/v1/contacts/"+into+"/merge", token, map[string]any{"duplicateId": from, "version": version})
	}
	assert.Equal(t, []string{"duplicateId"}, fieldNames(t, merge(keep.Id, moved.Id, keep.Version).Error()), "another company's contact is not merged")
	res := merge(keep.Id, dup.Id, keep.Version)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var merged api.Contact
	res.JSON(&merged)
	assert.Equal(t, "Dato' Aisyah", merged.FirstName, "the name kept")
	assert.Equal(t, "HR Director", merged.Title.MustGet())
	assert.Equal(t, "aisyah.merge@example.com", merged.Email.MustGet(), "the duplicate's email moved")
	assert.Equal(t, "Prefers WhatsApp.", merged.Notes.MustGet())
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/contacts/"+dup.Id, token, nil).Status, "the duplicate is gone")

	var j api.JobOrder
	a.request(http.MethodGet, "/v1/jobs/"+job.Id, token, nil).JSON(&j)
	assert.Equal(t, keep.Id, j.ContactId.MustGet(), "the job's contact is the one kept")
	var notes api.NoteList
	a.request(http.MethodGet, "/v1/notes?subjectType=contact&subjectId="+keep.Id, token, nil).JSON(&notes)
	assert.Len(t, notes.Items, 1, "the note moved")
	a.request(http.MethodGet, "/v1/contacts/duplicates", token, nil).JSON(&groups)
	assert.Empty(t, groups.Items)
}
