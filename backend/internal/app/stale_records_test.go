package app_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Stale records: contacts and companies nobody touched for a year are listed
// apart (?stale=true); a note, an edit or a job touches them again, and a
// contact touched touches their company; automatic work does not.
func TestRecordsNobodyTouchedForAYearAreListedApart(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	maju := a.createCompany(token, map[string]any{"name": "Maju Teknologi"})
	fresh := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	aisyah := a.createContact(token, map[string]any{"companyId": maju.Id, "firstName": "Aisyah", "lastName": "Rahman"})
	age := func() {
		t.Helper()
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE contact SET touched_at = now() - interval '400 days' WHERE id = $1`, aisyah.Id)
		require.NoError(t, err)
		_, err = testutil.Current.Admin.Exec(ctx, `UPDATE company SET touched_at = now() - interval '400 days' WHERE id = $1`, maju.Id)
		require.NoError(t, err)
	}
	staleContacts := func() []string {
		var page api.ContactPage
		res := a.request(http.MethodGet, "/v1/contacts?stale=true", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&page)
		var ids []string
		for _, c := range page.Items {
			ids = append(ids, c.Id)
		}
		return ids
	}
	staleCompanies := func() []string {
		var page api.CompanyPage
		res := a.request(http.MethodGet, "/v1/companies?stale=true", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&page)
		var ids []string
		for _, c := range page.Items {
			ids = append(ids, c.Id)
		}
		return ids
	}

	assert.Empty(t, staleContacts(), "just created")
	age()
	assert.Equal(t, []string{aisyah.Id}, staleContacts())
	assert.Equal(t, []string{maju.Id}, staleCompanies(), "not the company touched lately")
	var all api.ContactPage
	a.request(http.MethodGet, "/v1/contacts", token, nil).JSON(&all)
	require.Len(t, all.Items, 1)
	assert.Less(t, all.Items[0].TouchedAt.Unix(), all.Items[0].UpdatedAt.Unix()+1, "each row says when it was last touched")

	// A note on the contact: they and their company are touched.
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", token, map[string]any{
		"subjectType": "contact", "subjectId": aisyah.Id, "body": "Called; hiring again in Q1.",
	}).Status)
	assert.Empty(t, staleContacts())
	assert.Empty(t, staleCompanies())

	// An edit of the contact.
	age()
	var got api.Contact
	a.request(http.MethodGet, "/v1/contacts/"+aisyah.Id, token, nil).JSON(&got)
	require.Equal(t, http.StatusOK, a.request(http.MethodPatch, "/v1/contacts/"+aisyah.Id, token, map[string]any{"version": got.Version, "title": "HR Manager"}).Status)
	assert.Empty(t, staleContacts())
	assert.Empty(t, staleCompanies())

	// A job for the company, naming the contact.
	age()
	a.createJob(token, map[string]any{"companyId": maju.Id, "contactId": aisyah.Id, "title": "Senior Accountant"})
	assert.Empty(t, staleContacts())
	assert.Empty(t, staleCompanies())

	// Automatic work (a monthly email check, a sync) changes the row but is not a touch.
	age()
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE contact SET updated_at = now() WHERE id = $1`, aisyah.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{aisyah.Id}, staleContacts())
	assert.False(t, slices.Contains(staleCompanies(), fresh.Id))
}
