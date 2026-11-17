package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) hrScan(method, path, token string, body any, want int) api.HrScan {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var s api.HrScan
	res.JSON(&s)
	return s
}

func person(slug, name, title string) map[string]any {
	return map[string]any{"url": "https://www.linkedin.com/in/" + slug, "name": name, "title": title, "location": "Kuala Lumpur"}
}

func TestFindHRReadsPageByPageResumesWhereAPageFailedAndKnowsWhoIsAContact(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	bank := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad"})
	a.createContact(token, map[string]any{"companyId": bank.Id, "firstName": "Kumar", "lastName": "Selvam", "linkedinUrl": "https://www.linkedin.com/in/kumar"})
	a.createContact(token, map[string]any{"companyId": bank.Id, "firstName": "Aisha", "lastName": "Rahman"})
	base := "/v1/companies/" + bank.Id + "/hr-scan"

	var none api.HrScanOrNone
	a.request(http.MethodGet, base, token, nil).JSON(&none)
	assert.True(t, none.Scan.IsNull(), "no scan yet")
	start := map[string]any{"linkedinCompanyId": "123456", "linkedinCompanyName": "Maju Bank", "country": "MY",
		"keywords": []string{"talent acquisition", "recruiter", "HR manager", "people and culture"}, "pages": 2}
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, base, tenant.Users[api.RoleViewer].Token, start).Status)
	scan := a.hrScan(http.MethodPut, base, token, start, http.StatusOK)
	assert.Equal(t, []string{`"talent acquisition" OR "recruiter" OR "HR manager"`, `"people and culture"`}, scan.Queries)
	assert.Equal(t, "106808692", scan.GeoId.MustGet())
	assert.Equal(t, "Malaysia", scan.CountryName.MustGet())
	assert.Equal(t, api.HrScanNext{Query: 0, Page: 1}, scan.Next.MustGet())

	scan = a.hrScan(http.MethodPost, base+"/pages", token, map[string]any{"query": 0, "page": 1, "hasNext": true, "people": []any{
		person("farah", "Farah Aziz", "Head of Talent Acquisition"), person("kumar", "Kumar Selvam", "People & Culture Manager"),
		person("aisha-r", "Aisha Rahman", "Senior Recruiter"), person("nurul", "Nurul Huda", "HR Intern"),
	}}, http.StatusOK)
	assert.Equal(t, api.HrScanNext{Query: 0, Page: 2}, scan.Next.MustGet())
	// Page 2 fails: the scan stops there, with what it found, to resume there.
	scan = a.hrScan(http.MethodPost, base+"/pages", token, map[string]any{"query": 0, "page": 2, "error": "LinkedIn did not finish loading."}, http.StatusOK)
	assert.Equal(t, api.HrScanStatusStopped, scan.Status)
	assert.Equal(t, api.HrScanNext{Query: 0, Page: 2}, scan.Next.MustGet())
	assert.Len(t, scan.People, 4)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPost, base+"/pages", token, map[string]any{"query": 0, "page": 1, "people": []any{}}).Status, "page 1 is read already")

	scan = a.hrScan(http.MethodPost, base+"/pages", token, map[string]any{"query": 0, "page": 2, "hasNext": true, "people": []any{person("farah", "Farah Aziz", "Head of Talent Acquisition")}}, http.StatusOK)
	assert.Equal(t, api.HrScanNext{Query: 1, Page: 1}, scan.Next.MustGet(), "two pages read: the next search")
	scan = a.hrScan(http.MethodPost, base+"/pages", token, map[string]any{"query": 1, "page": 1, "hasNext": false,
		"people": []any{person("daniel", "Daniel Lee", "HR Business Partner")}}, http.StatusOK)
	assert.Equal(t, api.HrScanStatusDone, scan.Status)
	assert.True(t, scan.Next.IsNull())
	assert.Equal(t, 3, scan.PagesRead, "the failed page counts once, when read")

	byName := map[string]api.HrScanPerson{}
	for _, p := range scan.People {
		byName[p.Name] = p
	}
	require.Len(t, byName, 5, "each profile once")
	assert.Equal(t, api.HrScanPersonTierA, scan.People[0].Tier, "best first")
	assert.False(t, byName["Kumar Selvam"].ContactId.IsNull(), "a contact already: same LinkedIn profile")
	assert.Equal(t, "Aisha Rahman", byName["Aisha Rahman"].DuplicateName.MustGet(), "a possible duplicate: same name")
	assert.True(t, byName["Farah Aziz"].ContactId.IsNull())
	assert.False(t, byName["Nurul Huda"].Hidden.IsNull(), "interns are left out")

	scan = a.hrScan(http.MethodPost, base+"/joined", token, map[string]any{"url": "https://www.linkedin.com/in/farah", "current": true, "joined": "2023-03"}, http.StatusOK)
	for _, p := range scan.People {
		if p.Name == "Farah Aziz" {
			assert.Equal(t, "2023-03", p.Joined.MustGet())
			assert.True(t, p.Current.MustGet())
		}
	}
}
