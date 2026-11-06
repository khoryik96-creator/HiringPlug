package app_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The dashboard's quick search (like Kano's /find): every word must appear,
// phone numbers match however they are written, names can be looked up
// loosely, and jobs are found by their client's name.
func TestQuickSearch(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	a.createCandidate(token, map[string]any{
		"firstName": "Edran", "lastName": "Zainal", "email": "edran.z@example.com", "phone": "+60 16-766 3880",
		"currentTitle": "Database Administrator", "currentCompany": "Lenovo",
	})
	a.createCandidate(token, map[string]any{"firstName": "Edwin", "lastName": "Lim", "phone": "012 345 6789", "currentCompany": "Grab"})

	candidates := func(query string) []string {
		var page api.CandidatePage
		res := a.request(http.MethodGet, "/v1/candidates?"+query, token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&page)
		names := make([]string, len(page.Items))
		for i, c := range page.Items {
			names[i] = c.FullName
		}
		return names
	}
	q := func(s string) string { return "q=" + url.QueryEscape(s) }

	t.Run("candidates by email, phone in any format, and several words", func(t *testing.T) {
		assert.Equal(t, []string{"Edran Zainal"}, candidates(q("edran.z@example.com")))
		for _, phone := range []string{"0167663880", "016-766 3880", "+60167663880", "60 16 766 3880", "7663880"} {
			assert.Equal(t, []string{"Edran Zainal"}, candidates(q(phone)), phone)
		}
		assert.Equal(t, []string{"Edran Zainal"}, candidates(q("edran lenovo")), "name and current company")
		assert.Equal(t, []string{"Edran Zainal"}, candidates(q("lenovo - edran")), "in any order, punctuation ignored")
		assert.Equal(t, []string{"Edran Zainal"}, candidates(q("database administrator")), "current title")
		assert.Empty(t, candidates(q("edran grab")), "every word must match")
		assert.Empty(t, candidates(q("123")), "too few digits is not a phone search")
		assert.Empty(t, candidates(q("100%")), "wildcards are literal")
	})

	t.Run("names alike, for typos, closest first", func(t *testing.T) {
		assert.Empty(t, candidates(q("edran zainl")))
		similar := candidates(q("edran zainl") + "&similar=true")
		require.NotEmpty(t, similar)
		assert.Equal(t, "Edran Zainal", similar[0])
		assert.NotContains(t, candidates(q("edran zainl")+"&similar=true"), "Edwin Lim")
	})

	t.Run("contacts by phone, mobile, words and names alike", func(t *testing.T) {
		maybank := a.createCompany(token, map[string]any{"name": "Maybank", "phone": "+60 3-2070 8833"})
		a.createContact(token, map[string]any{
			"companyId": maybank.Id, "firstName": "Siti", "lastName": "Aminah", "title": "Head of Talent",
			"phone": "03-2070 1111", "mobile": "+65 9123 4567",
		})
		contacts := func(query string) []string {
			var page api.ContactPage
			res := a.request(http.MethodGet, "/v1/contacts?"+query, token, nil)
			require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
			res.JSON(&page)
			names := make([]string, len(page.Items))
			for i, c := range page.Items {
				names[i] = c.FullName
			}
			return names
		}
		assert.Equal(t, []string{"Siti Aminah"}, contacts(q("91234567")), "mobile")
		assert.Equal(t, []string{"Siti Aminah"}, contacts(q("(03) 2070 1111")), "phone")
		assert.Equal(t, []string{"Siti Aminah"}, contacts(q("siti talent")))
		assert.Equal(t, []string{"Siti Aminah"}, contacts(q("siti aminh")+"&similar=true"))

		var companies api.CompanyPage
		a.request(http.MethodGet, "/v1/companies?"+q("0320708833"), token, nil).JSON(&companies)
		require.Len(t, companies.Items, 1, "companies by phone")
		assert.Equal(t, "Maybank", companies.Items[0].Name)
	})

	t.Run("jobs by title, location and the client's name", func(t *testing.T) {
		maju := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
		kedai := a.createCompany(token, map[string]any{"name": "Kedai Online"})
		a.createJob(token, map[string]any{"companyId": maju.Id, "title": "Finance Manager", "location": "Kuala Lumpur"})
		a.createJob(token, map[string]any{"companyId": kedai.Id, "title": "Finance Executive", "location": "Penang"})
		jobs := func(query string) []string {
			var page api.JobOrderPage
			res := a.request(http.MethodGet, "/v1/jobs?"+query, token, nil)
			require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
			res.JSON(&page)
			titles := make([]string, len(page.Items))
			for i, j := range page.Items {
				titles[i] = j.Title
			}
			return titles
		}
		assert.Equal(t, []string{"Finance Manager"}, jobs(q("maju")))
		assert.Equal(t, []string{"Finance Manager"}, jobs(q("finance maju")), "a title word and a client word")
		assert.Equal(t, []string{"Finance Executive"}, jobs(q("finance penang")))
		assert.ElementsMatch(t, []string{"Finance Manager", "Finance Executive"}, jobs(q("finance")))
		assert.Empty(t, jobs(q("finance grab")))
	})
}
