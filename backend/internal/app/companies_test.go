package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) createCompany(token string, body map[string]any) api.Company {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/companies", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var c api.Company
	res.JSON(&c)
	return c
}

func TestCompaniesCreateTidiesAndReturnsEveryField(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	c := a.createCompany(tenant.Users[api.RoleRecruiter].Token, map[string]any{
		"name": "  Maybank   Berhad ", "website": "Maybank2u.com.my", "industry": "Banking",
		"size": "enterprise", "city": "Kuala Lumpur", "country": "MY", "registrationNumber": "196001000142",
	})
	assert.Equal(t, "Maybank Berhad", c.Name)
	assert.Equal(t, api.CompanyTypeClient, c.Type, "default type")
	website, _ := c.Website.Get()
	assert.Equal(t, "https://maybank2u.com.my", website)
	assert.Equal(t, 1, c.Version)

	var got api.Company
	a.request(http.MethodGet, "/v1/companies/"+c.Id, tenant.Users[api.RoleRecruiter].Token, nil).JSON(&got)
	assert.Equal(t, c, got)
}

func TestCompaniesValidation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	tests := []struct {
		name   string
		body   map[string]any
		fields []string
	}{
		{"blank name", map[string]any{"name": "   "}, []string{"name"}},
		{"bad website", map[string]any{"name": "Acme", "website": "not a website"}, []string{"website"}},
		{"unknown type", map[string]any{"name": "Acme", "type": "partner"}, []string{"type"}},
		{"lower-case country", map[string]any{"name": "Acme", "country": "my"}, []string{"country"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := a.request(http.MethodPost, "/v1/companies", token, tc.body)
			require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
			assert.ElementsMatch(t, tc.fields, fieldNames(t, res.Error()))
		})
	}
}

func TestCompaniesRefuseDuplicateNames(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	first := a.createCompany(token, map[string]any{"name": "Nestlé Malaysia"})

	res := a.request(http.MethodPost, "/v1/companies", token, map[string]any{"name": "NESTLE  malaysia"})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	e := res.Error()
	assert.Equal(t, api.ErrorCodeCOMPANYDUPLICATENAME, e.Error.Code)
	assert.Equal(t, first.Id, (*e.Error.Details)["existingCompanyId"])

	// Renaming another company to the same name is refused too.
	second := a.createCompany(token, map[string]any{"name": "Nestle Products"})
	res = a.request(http.MethodPatch, "/v1/companies/"+second.Id, token, map[string]any{"version": second.Version, "name": "Nestle Malaysia"})
	assert.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
}

func TestCompaniesUpdate(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCompany(token, map[string]any{"name": "Grab", "city": "Singapore", "industry": "Tech"})

	res := a.request(http.MethodPatch, "/v1/companies/"+c.Id, token, map[string]any{"version": c.Version, "type": "prospect", "city": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var updated api.Company
	res.JSON(&updated)
	assert.Equal(t, api.CompanyTypeProspect, updated.Type)
	assert.True(t, updated.City.IsNull(), "null clears")
	industry, _ := updated.Industry.Get()
	assert.Equal(t, "Tech", industry, "fields not sent stay")
	assert.Equal(t, 2, updated.Version)

	// A save based on the old version is refused: no lost updates.
	res = a.request(http.MethodPatch, "/v1/companies/"+c.Id, token, map[string]any{"version": c.Version, "name": "Grab Holdings"})
	assert.Equal(t, http.StatusConflict, res.Status)
	assert.Equal(t, api.ErrorCodeCONFLICTSTALEVERSION, res.Error().Error.Code)

	// Saving without changes writes nothing (same version).
	res = a.request(http.MethodPatch, "/v1/companies/"+c.Id, token, map[string]any{"version": updated.Version, "type": "prospect"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var same api.Company
	res.JSON(&same)
	assert.Equal(t, updated.Version, same.Version)
}

func TestCompaniesSearchPermissionsAndIsolation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleViewer, api.RoleHiringManager)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	a.createCompany(recruiter, map[string]any{"name": "Société Générale", "website": "societegenerale.com"})
	a.createCompany(recruiter, map[string]any{"name": "Shopee", "type": "prospect"})
	a.createCompany(tenant.Users[api.RoleSourcer].Token, map[string]any{"name": "Lazada"})

	var page api.CompanyPage
	a.request(http.MethodGet, "/v1/companies?q=societe", tenant.Users[api.RoleViewer].Token, nil).JSON(&page)
	require.Len(t, page.Items, 1, "search ignores case and accents")
	assert.Equal(t, "Société Générale", page.Items[0].Name)

	a.request(http.MethodGet, "/v1/companies?type=prospect", recruiter, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Shopee", page.Items[0].Name)

	a.request(http.MethodGet, "/v1/companies?limit=2", recruiter, nil).JSON(&page)
	require.Len(t, page.Items, 2)
	cursor, err := page.NextCursor.Get()
	require.NoError(t, err)
	a.request(http.MethodGet, "/v1/companies?limit=2&cursor="+cursor, recruiter, nil).JSON(&page)
	assert.Len(t, page.Items, 1, "second page")

	for _, role := range []api.Role{api.RoleViewer, api.RoleHiringManager} {
		res := a.request(http.MethodPost, "/v1/companies", tenant.Users[role].Token, map[string]any{"name": "Nope"})
		assert.Equal(t, http.StatusForbidden, res.Status, "%s may only read", role)
	}

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.request(http.MethodGet, "/v1/companies", other.Users[api.RoleOwner].Token, nil).JSON(&page)
	assert.Empty(t, page.Items, "companies never see each other's records")
	a.createCompany(other.Users[api.RoleOwner].Token, map[string]any{"name": "Shopee"}) // names are unique per company only
}
