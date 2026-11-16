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

func TestCompaniesAndContactsTakeABooleanSearch(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	bank := a.createCompany(token, map[string]any{"name": "Maju Bank", "industry": "Banking", "city": "Kuala Lumpur", "about": "Retail and Islamic banking across Malaysia."})
	insurer := a.createCompany(token, map[string]any{"name": "Takaful Prima", "industry": "Insurance", "city": "Penang", "about": "Family takaful and general insurance."})
	agency := a.createCompany(token, map[string]any{"name": "Prima Staffing", "industry": "Staffing", "city": "Kuala Lumpur", "about": "A recruitment agency for banks."})

	companies := func(boolean string) ([]string, *response) {
		res := a.request(http.MethodGet, "/v1/companies?boolean="+url.QueryEscape(boolean), token, nil)
		var page api.CompanyPage
		var ids []string
		if res.Status == http.StatusOK {
			res.JSON(&page)
			for _, c := range page.Items {
				ids = append(ids, c.Id)
			}
		}
		return ids, res
	}
	got, res := companies(`(bank* OR insurance) NOT agency`)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.ElementsMatch(t, []string{bank.Id, insurer.Id}, got, "the about counts too; the agency is left out")
	got, _ = companies(`"kuala lumpur" AND (staffing OR retail)`)
	assert.ElementsMatch(t, []string{bank.Id, agency.Id}, got)
	got, _ = companies(`islamic NEAR/1 banking`)
	assert.Equal(t, []string{bank.Id}, got)
	_, res = companies(`bank AND (insurance`)
	assert.Equal(t, http.StatusBadRequest, res.Status)
	assert.Contains(t, string(res.Body), "( without its )")

	hr := a.createContact(token, map[string]any{"companyId": bank.Id, "firstName": "Aisha", "lastName": "Rahman", "title": "HR Manager", "email": "aisha@majubank.example"})
	ta := a.createContact(token, map[string]any{"companyId": insurer.Id, "firstName": "Kumar", "title": "Talent Acquisition Lead"})
	a.createContact(token, map[string]any{"companyId": agency.Id, "firstName": "Lee", "title": "HR Intern"})
	contacts := func(boolean string) []string {
		res := a.request(http.MethodGet, "/v1/contacts?boolean="+url.QueryEscape(boolean), token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.ContactPage
		res.JSON(&page)
		var ids []string
		for _, c := range page.Items {
			ids = append(ids, c.Id)
		}
		return ids
	}
	assert.ElementsMatch(t, []string{hr.Id, ta.Id}, contacts(`(HR OR "talent acquisition") NOT intern*`))
	assert.Equal(t, []string{hr.Id}, contacts(`aisha AND manager`), "name and title together")
}
