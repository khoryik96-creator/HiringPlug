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
	hinted := a.createCompany(token, map[string]any{"name": "Lori Ekspres", "industryHint": "Truck Transportation"})
	got, _ = companies(`truck*`)
	assert.Equal(t, []string{hinted.Id}, got, "LinkedIn's industry counts")
	got, _ = companies(`lumpur NEAR/1 retail`)
	assert.Empty(t, got, "the city's last word is not next to the about's first: fields stay apart")
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
	assert.Equal(t, []string{hr.Id}, contacts(`majubank AND HR`), "an email's words count")
	assert.Equal(t, []string{hr.Id}, contacts(`"Aisha Rahman"`), "the full name as a phrase")
	assert.Equal(t, []string{hr.Id}, contacts(`"majubank.example"`), "and the address as typed")
	assert.Equal(t, []string{ta.Id}, contacts(`(talent OR HR) !aisha !intern`), "!word leaves out")
}

func TestABooleanFindsACandidatesCertifications(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	acca := a.createCandidate(token, map[string]any{"firstName": "Siti", "certifications": []string{"ACCA", "CIMA"}})
	a.createCandidate(token, map[string]any{"firstName": "Ben", "skills": []string{"Python"}})
	res := a.request(http.MethodPost, "/v1/candidates/search", token, map[string]any{"boolean": "ACCA OR CPA"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var page api.CandidateSearchPage
	res.JSON(&page)
	require.Len(t, page.Items, 1, "found by the certification on the profile (no CV)")
	assert.Equal(t, acca.Id, page.Items[0].Candidate.Id)
}

func TestNoAIPickIsShownWhenTheAIIsOff(t *testing.T) {
	t.Parallel()
	a := newTestApp(t) // no AI
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	c := a.createCompany(token, map[string]any{"name": "Kedai Runcit Ali"})
	assert.False(t, *c.IndustryPending, "nothing will pick it, so nothing is said to be picking it")
}

func TestABooleanFindsACompanyByAnyOfItsIndustries(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	c := industryCatalogue(t, a, owner)
	banking, education := industryNamed(t, c, "FSI – Banking"), industryNamed(t, c, "Education")
	co := a.createCompany(owner, map[string]any{"name": "Sekolah Wang", "industrySelections": map[string]any{
		"catalogueVersion": c.Version, "ids": []string{education.Id, banking.Id}, "primaryId": education.Id,
	}})
	find := func(boolean string) []string {
		res := a.request(http.MethodGet, "/v1/companies?boolean="+url.QueryEscape(boolean), owner, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var page api.CompanyPage
		res.JSON(&page)
		var ids []string
		for _, x := range page.Items {
			ids = append(ids, x.Id)
		}
		return ids
	}
	assert.Equal(t, []string{co.Id}, find(`banking AND "financial services"`), "a second industry's subindustry, and its industry")
	assert.Equal(t, []string{co.Id}, find(`education`), "the main one")

	// Renamed in the catalogue: the company is found by the new words.
	res := a.request(http.MethodPatch, "/v1/industries/"+banking.Id, owner, map[string]any{
		"catalogueVersion": c.Version, "label": "FSI – Islamic Banking", "aliases": banking.Aliases, "retired": false, "parentId": banking.ParentId,
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, []string{co.Id}, find(`islamic`))
}
