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

func (a *testApp) startLinkedinChecks(token string, body map[string]any) api.ContactLinkedinChecksStarted {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/contacts/linkedin-checks", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var out api.ContactLinkedinChecksStarted
	res.JSON(&out)
	return out
}

func TestCheckingOnLinkedInWhetherAContactHasLeft(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token

	bare := a.createCompany(token, map[string]any{"name": "No Page Bhd"})
	company := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd", "linkedinUrl": "linkedin.com/company/maju-logistik/about/"})
	ana := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Ana", "lastName": "Lim", "linkedinUrl": "https://my.linkedin.com/in/ana-lim-example/"})
	wei := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Wei", "linkedinUrl": "linkedin.com/in/wei-example"})
	a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Short", "linkedinUrl": "https://lnkd.in/abc"})
	a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Nobody"})

	res := a.request(http.MethodPost, "/v1/contacts/linkedin-checks", token, map[string]any{"companyId": bare.Id})
	assert.Equal(t, []string{"companyId"}, fieldNames(t, res.Error()), "the company's LinkedIn page is needed")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/contacts/linkedin-checks", tenant.Users[api.RoleViewer].Token,
		map[string]any{"companyId": company.Id}).Status)

	started := a.startLinkedinChecks(token, map[string]any{"companyId": company.Id})
	assert.Equal(t, "https://www.linkedin.com/company/maju-logistik/", started.CompanyUrl)
	require.Len(t, started.Checks, 2)
	assert.Len(t, started.Skipped, 2, "no profile, or a short link")
	checks := map[string]api.ContactLinkedinCheckRequest{}
	for _, c := range started.Checks {
		checks[c.ContactName] = c
	}
	assert.Equal(t, "https://www.linkedin.com/in/ana-lim-example", checks["Ana Lim"].ProfileUrl)

	answer := func(token, id string, body map[string]any) *response {
		return a.request(http.MethodPost, "/v1/contacts/linkedin-checks/"+id+"/answer", token, body)
	}
	former := map[string]any{"profileUrl": checks["Ana Lim"].ProfileUrl, "companyUrl": started.CompanyUrl, "linkedinCompanyId": "123456",
		"joined": "2019-04", "left": "2025-08", "current": false}

	// Only whoever started the check may answer it, about that profile and company.
	assert.Equal(t, http.StatusNotFound, answer(tenant.Users[api.RoleAdmin].Token, checks["Ana Lim"].Id, former).Status)
	wrong := map[string]any{"profileUrl": "https://www.linkedin.com/in/someone-else", "companyUrl": started.CompanyUrl, "linkedinCompanyId": "123456",
		"joined": "2019-04", "left": "2025-08", "current": false}
	assert.Equal(t, []string{"profileUrl"}, fieldNames(t, answer(token, checks["Ana Lim"].Id, wrong).Error()))
	nonsense := map[string]any{"profileUrl": checks["Ana Lim"].ProfileUrl, "companyUrl": started.CompanyUrl, "linkedinCompanyId": "123456",
		"joined": "2025-08", "left": "2019-04", "current": false}
	assert.Equal(t, []string{"joined"}, fieldNames(t, answer(token, checks["Ana Lim"].Id, nonsense).Error()), "left before joining")

	// Their time there ended, with no current role there: they have left.
	res = answer(token, checks["Ana Lim"].Id, former)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var check api.ContactLinkedinCheck
	res.JSON(&check)
	assert.True(t, check.MarkedLeft)
	got := a.contact(token, ana.Id)
	assert.Equal(t, api.ContactStatusLeft, got.Status)
	assert.Equal(t, ana.Version+1, got.Version)
	assert.Equal(t, "2025-08", got.LinkedinCheck.MustGet().Left.MustGet())
	assert.Equal(t, http.StatusConflict, answer(token, checks["Ana Lim"].Id, former).Status, "answered once")

	// Unclear dates change nothing.
	res = answer(token, checks["Wei"].Id, map[string]any{"profileUrl": checks["Wei"].ProfileUrl, "companyUrl": started.CompanyUrl,
		"linkedinCompanyId": "123456", "joined": "", "left": "", "current": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, api.ContactStatusActive, a.contact(token, wei.Id).Status)

	listed := a.companyContacts(token, company.Id)
	employment := listed["Ana Lim"].LinkedinEmployment.MustGet()
	assert.False(t, employment.Current.MustGet())
	assert.Equal(t, "2019-04", employment.Joined.MustGet())
	assert.True(t, listed["Wei"].LinkedinEmployment.MustGet().Current.IsNull())
	assert.True(t, listed["Nobody"].LinkedinEmployment.IsNull())

	// One contact; still at the company; a check left too long expires; a
	// profile that could not be read is kept as failed.
	one := a.startLinkedinChecks(token, map[string]any{"contactId": wei.Id})
	require.Len(t, one.Checks, 1)
	res = answer(token, one.Checks[0].Id, map[string]any{"profileUrl": one.Checks[0].ProfileUrl, "companyUrl": one.CompanyUrl,
		"linkedinCompanyId": "123456", "joined": "2021", "left": "", "current": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, api.ContactStatusActive, a.contact(token, wei.Id).Status)
	late := a.startLinkedinChecks(token, map[string]any{"contactId": wei.Id})
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE contact_linkedin_check SET created_at = now() - interval '20 minutes' WHERE id = $1`, late.Checks[0].Id)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, answer(token, late.Checks[0].Id, map[string]any{"error": "LinkedIn needs sign-in"}).Status)
	failed := a.startLinkedinChecks(token, map[string]any{"contactId": wei.Id})
	res = answer(token, failed.Checks[0].Id, map[string]any{"error": "LinkedIn needs sign-in, or the profile could not be confirmed."})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, api.ContactLinkedinCheckStatusFailed, a.contact(token, wei.Id).LinkedinCheck.MustGet().Status)

	// Another agency cannot start or answer these checks.
	stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/contacts/linkedin-checks", stranger, map[string]any{"contactId": wei.Id}).Status)
	assert.Equal(t, http.StatusNotFound, answer(stranger, checks["Wei"].Id, former).Status)
}
