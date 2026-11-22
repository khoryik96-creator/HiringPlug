package app_test

import (
	"context"
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

	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 AND action LIKE 'company.hr_scan_%'`, tenant.ID, bank.Id).Scan(&audited))
	assert.Equal(t, 5, audited, "the start and each page read or failed, recorded")
	scan = a.hrScan(http.MethodPost, base+"/joined", token, map[string]any{"url": "https://www.linkedin.com/in/farah", "current": true, "joined": "2023-03"}, http.StatusOK)
	for _, p := range scan.People {
		if p.Name == "Farah Aziz" {
			assert.Equal(t, "2023-03", p.Joined.MustGet())
			assert.True(t, p.Current.MustGet())
		}
	}
}

// Find HR checks the people it finds against every contact in the agency:
// the same LinkedIn profile however it is written, the same name in another
// order or form (bin/binti, Mohd, titles), or one name inside the other, and
// shows the likeliest contact with their company and email.
func TestFindHRShowsWhoIsInHiringPlugAndTheirEmail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	bank := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad"})
	old := a.createCompany(token, map[string]any{"name": "Contoh Holdings"})
	a.createContact(token, map[string]any{"companyId": old.Id, "firstName": "Kumar", "lastName": "Selvam", "title": "HR Director",
		"email": "kumar@contoh.example", "linkedinUrl": "https://www.linkedin.com/in/Kumar-Selvam"})
	a.createContact(token, map[string]any{"companyId": bank.Id, "firstName": "Nur Aisyah", "lastName": "binti Ahmad", "email": "aisyah@maju.example"})
	a.createContact(token, map[string]any{"companyId": old.Id, "firstName": "Mohd Faizal", "lastName": "Abd Rahman", "email": "faizal.rahman@gmail.com"})
	a.createContact(token, map[string]any{"companyId": old.Id, "firstName": "Lee", "lastName": "Daniel", "status": "left"})
	a.createContact(token, map[string]any{"companyId": old.Id, "firstName": "Nur", "lastName": "Zainal"}) // one word in common only
	base := "/v1/companies/" + bank.Id + "/hr-scan"
	a.hrScan(http.MethodPut, base, token, map[string]any{"linkedinCompanyId": "123456", "linkedinCompanyName": "Maju Bank",
		"keywords": []string{"HR manager"}, "pages": 1}, http.StatusOK)
	scan := a.hrScan(http.MethodPost, base+"/pages", token, map[string]any{"query": 0, "page": 1, "hasNext": false, "people": []any{
		person("kumar-selvam", "Kumar Selvam", "HR Manager"), person("aisyah", "Aisyah Ahmad", "Talent Acquisition Lead"),
		person("faizal", "Muhammad Faizal Abdul Rahman (He/Him)", "HR Business Partner"), person("daniel", "Daniel Lee", "Recruiter"),
		person("nur-huda", "Nur Huda", "HR Executive"),
	}}, http.StatusOK)
	match := map[string]api.HrScanMatch{}
	for _, p := range scan.People {
		if m, err := p.Match.Get(); err == nil {
			match[p.Name] = m
		}
	}

	kumar := match["Kumar Selvam"]
	assert.Equal(t, api.HrScanMatchKindProfile, kumar.Kind, "the same profile, whatever its letter case")
	assert.Equal(t, "kumar@contoh.example", kumar.Email.MustGet())
	assert.Equal(t, "Contoh Holdings", kumar.CompanyName, "at another company in HiringPlug")
	assert.False(t, kumar.SameCompany)
	assert.Equal(t, "HR Director", kumar.Title.MustGet())

	aisyah := match["Aisyah Ahmad"]
	assert.Equal(t, api.HrScanMatchKindSimilarName, aisyah.Kind, "one name inside the other")
	assert.Equal(t, "aisyah@maju.example", aisyah.Email.MustGet())
	assert.True(t, aisyah.SameCompany)
	assert.False(t, aisyah.EmailPersonal)

	faizal := match["Muhammad Faizal Abdul Rahman (He/Him)"]
	assert.Equal(t, api.HrScanMatchKindSameName, faizal.Kind, "Mohd, Abd and (He/Him) aside, the same name")
	assert.True(t, faizal.EmailPersonal, "a gmail address is theirs, not work")

	daniel := match["Daniel Lee"]
	assert.Equal(t, api.HrScanMatchKindSameName, daniel.Kind, "in any order")
	assert.True(t, daniel.Left)
	assert.True(t, daniel.Email.IsNull())

	assert.NotContains(t, match, "Nur Huda", "one word in common is not a match")
	for _, p := range scan.People {
		switch p.Name {
		case "Kumar Selvam":
			assert.False(t, p.ContactId.IsNull(), "still named as a contact for older clients")
		case "Aisyah Ahmad":
			assert.False(t, p.DuplicateOf.IsNull(), "and as a possible duplicate")
		}
	}
}
