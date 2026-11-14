package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Following a contact who left: the open jobs they were the hiring contact
// for, and whether their new company is already a client.
func TestFindingWhatAContactWhoLeftLeavesBehindAndWhereTheyWent(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	maju := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd"})
	acme := a.createCompany(token, map[string]any{"name": "Acme Bhd", "linkedinUrl": "linkedin.com/company/Acme-Bhd/about/"})
	a.createCompany(token, map[string]any{"name": "Acme Holdings", "linkedinUrl": "linkedin.com/company/acme-bhd-holdings"})
	ana := a.createContact(token, map[string]any{"companyId": maju.Id, "firstName": "Ana"})
	other := a.createContact(token, map[string]any{"companyId": maju.Id, "firstName": "Wei"})
	for _, job := range []map[string]any{
		{"companyId": maju.Id, "contactId": ana.Id, "title": "Finance Manager"},
		{"companyId": maju.Id, "contactId": ana.Id, "title": "Auditor"},
		{"companyId": maju.Id, "contactId": other.Id, "title": "Clerk"},
	} {
		require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/jobs", token, job).Status)
	}

	var jobs api.JobOrderPage
	res := a.request(http.MethodGet, "/v1/jobs?status=open&contactId="+ana.Id, token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&jobs)
	var titles []string
	for _, j := range jobs.Items {
		titles = append(titles, j.Title)
	}
	assert.ElementsMatch(t, []string{"Finance Manager", "Auditor"}, titles)

	var companies api.CompanyPage
	res = a.request(http.MethodGet, "/v1/companies?linkedinCompany=acme-bhd", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&companies)
	require.Len(t, companies.Items, 1, "the exact page, in any letter case; not acme-bhd-holdings")
	assert.Equal(t, acme.Id, companies.Items[0].Id)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/companies?linkedinCompany=acme.*", token, nil).Status)
}

func TestSuggestingAnEmailFromHowColleaguesAddressesAreSpelled(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Acme Bhd"})
	suggest := func(id string) api.ContactEmailSuggestion {
		var out api.ContactEmailSuggestion
		res := a.request(http.MethodGet, "/v1/contacts/"+id+"/email-suggestion", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&out)
		return out
	}
	ana := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Ana", "lastName": "Lim"})
	assert.True(t, suggest(ana.Id).Suggestion.IsNull(), "no colleagues yet")
	a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Siti", "lastName": "Aisyah", "email": "siti.aisyah@acme.test"})
	a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Wei", "lastName": "Chen", "email": "wei.chen@acme.test"})
	got := suggest(ana.Id).Suggestion.MustGet()
	assert.Equal(t, api.EmailGuess{Email: "ana.lim@acme.test", Pattern: "first.last", Matches: 2, Of: 2}, got)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/contacts/"+ana.Id+"/email-suggestion",
		testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token, nil).Status)
}
