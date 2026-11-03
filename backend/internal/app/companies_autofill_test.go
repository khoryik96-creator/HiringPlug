package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// companySite is a company website for tests: the home page has schema.org
// organization data and links to a contact page with the registration
// number. With closed set, robots.txt asks robots to stay out.
func companySite(t *testing.T, closed bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			if closed {
				_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
			}
		case "/":
			_, _ = w.Write([]byte(`<html><head><script type="application/ld+json">{"@type":"Organization","legalName":"Contoh Teknologi Sdn Bhd",
				"telephone":"+60 3-1234 5678","numberOfEmployees":{"value":120},
				"address":{"streetAddress":"Level 5, Menara Contoh","postalCode":"50450","addressLocality":"Kuala Lumpur","addressCountry":"Malaysia"}}</script>
				<meta name="description" content="Payroll software for Malaysian companies."></head>
				<body><a href="/hubungi-kami">Hubungi kami</a></body></html>`))
		case "/hubungi-kami":
			_, _ = w.Write([]byte(`<html><body>Contoh Teknologi Sdn Bhd (1234567-X)</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (a *testApp) autofill(token, companyID string) api.CompanyAutofill {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/companies/"+companyID+"/autofill", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.CompanyAutofill
	res.JSON(&out)
	return out
}

func suggestionFor(t *testing.T, a api.CompanyAutofill, field api.CompanySuggestionField) api.CompanySuggestion {
	t.Helper()
	for _, s := range a.Suggestions {
		if s.Field == field {
			return s
		}
	}
	t.Fatalf("no suggestion for %s in %+v", field, a.Suggestions)
	return api.CompanySuggestion{}
}

func TestCompanyAutofillFromItsWebsite(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	site := companySite(t, false)
	c := a.createCompany(token, map[string]any{"name": "Contoh", "website": site.URL, "city": "Kuala Lumpur"})

	assert.Equal(t, api.CompanyAutofillStatusIdle, a.autofill(token, c.Id).Status, "never read")
	assert.Equal(t, http.StatusForbidden,
		a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill", tenant.Users[api.RoleViewer].Token, nil).Status, "viewers only look")

	res := a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.CompanyAutofill
	res.JSON(&queued)
	assert.Equal(t, api.CompanyAutofillStatusQueued, queued.Status)

	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.autofill(token, c.Id)
	require.Equal(t, api.CompanyAutofillStatusDone, got.Status, "%+v", got.Error)
	assert.False(t, got.CheckedAt.IsNull())

	name := suggestionFor(t, got, api.CompanySuggestionFieldName)
	assert.Equal(t, "Contoh Teknologi Sdn Bhd", name.Value)
	current, _ := name.Current.Get()
	assert.Equal(t, "Contoh", current, "shown next to what is recorded")
	assert.Contains(t, name.Source, "schema.org data on "+site.URL)
	assert.Equal(t, "MY", suggestionFor(t, got, api.CompanySuggestionFieldCountry).Value)
	assert.Equal(t, "medium", suggestionFor(t, got, api.CompanySuggestionFieldSize).Value)
	assert.Equal(t, "Level 5, Menara Contoh, 50450", suggestionFor(t, got, api.CompanySuggestionFieldAddress).Value)
	assert.Equal(t, "Payroll software for Malaysian companies.", suggestionFor(t, got, api.CompanySuggestionFieldAbout).Value)
	reg := suggestionFor(t, got, api.CompanySuggestionFieldRegistrationNumber)
	assert.Equal(t, "1234567-X", reg.Value)
	assert.Contains(t, reg.Source, "/hubungi-kami", "from the contact page")
	for _, s := range got.Suggestions {
		assert.NotEqual(t, api.CompanySuggestionFieldCity, s.Field, "the city is already recorded")
	}

	t.Run("accepting sets the detail and is audited with its source", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill/"+name.Id, token, map[string]any{"accept": true})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var after api.CompanyAutofill
		res.JSON(&after)
		for _, s := range after.Suggestions {
			assert.NotEqual(t, name.Id, s.Id, "no longer waiting")
		}
		var company api.Company
		a.request(http.MethodGet, "/v1/companies/"+c.Id, token, nil).JSON(&company)
		assert.Equal(t, "Contoh Teknologi Sdn Bhd", company.Name)
		assert.Equal(t, c.Version+1, company.Version)

		var changes map[string]any
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT changes FROM audit_log WHERE entity_id = $1 AND action = 'company.updated'`, c.Id).Scan(&changes))
		assert.Contains(t, changes, "name")
		assert.Equal(t, name.Source, changes["from"])

		res = a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill/"+name.Id, token, map[string]any{"accept": true})
		assert.Equal(t, http.StatusNotFound, res.Status, "decided once")
	})

	t.Run("dismissing leaves the company as it was", func(t *testing.T) {
		phone := suggestionFor(t, got, api.CompanySuggestionFieldPhone)
		res := a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill/"+phone.Id, token, map[string]any{"accept": false})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var company api.Company
		a.request(http.MethodGet, "/v1/companies/"+c.Id, token, nil).JSON(&company)
		assert.True(t, company.Phone.IsNull())
	})

	t.Run("another company cannot see or decide them", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/companies/"+c.Id+"/autofill", other, nil).Status)
		reg := suggestionFor(t, got, api.CompanySuggestionFieldRegistrationNumber)
		res := a.request(http.MethodPost, "/v1/companies/"+c.Id+"/autofill/"+reg.Id, other, map[string]any{"accept": true})
		assert.Equal(t, http.StatusNotFound, res.Status)
	})
}

func TestCompanyAutofillNeedsAWebsiteRobotsAllow(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token

	none := a.createCompany(token, map[string]any{"name": "No Website"})
	res := a.request(http.MethodPost, "/v1/companies/"+none.Id+"/autofill", token, nil)
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"website"}, fieldNames(t, res.Error()))

	closed := a.createCompany(token, map[string]any{"name": "Private", "website": companySite(t, true).URL})
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/companies/"+closed.Id+"/autofill", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	got := a.autofill(token, closed.Id)
	assert.Equal(t, api.CompanyAutofillStatusFailed, got.Status)
	reason, _ := got.Error.Get()
	assert.Contains(t, reason, "robots.txt")
	assert.Empty(t, got.Suggestions)
}
