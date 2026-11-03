package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// careersSite is a company website for tests: /careers lists openings with
// JobPosting markup, /private is closed to robots, /plain has no markup.
func careersSite(t *testing.T, openings *atomic.Int32) *httptest.Server {
	posting := func(id, title string) string {
		return fmt.Sprintf(`{"@type":"JobPosting","title":%q,"identifier":{"value":%q},"hiringOrganization":{"name":"Contoh Berhad"},
			"jobLocation":{"address":{"addressLocality":"Kuala Lumpur","addressCountry":"MY"}}}`, title, id)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\n"))
		case "/careers":
			list := []string{posting("AE-1", "Account Executive"), posting("HR-2", "HR Executive")}[:openings.Load()]
			_, _ = fmt.Fprintf(w, `<html><script type="application/ld+json">{"@graph":[%s]}</script></html>`, strings.Join(list, ","))
		default:
			_, _ = w.Write([]byte("<html>Join us!</html>"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (a *testApp) leadSource(token string, body map[string]any) api.LeadSource {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/lead-sources", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var s api.LeadSource
	res.JSON(&s)
	return s
}

func (a *testApp) sourceByID(token, id string) api.LeadSource {
	a.t.Helper()
	var list api.LeadSourceList
	a.request(http.MethodGet, "/v1/lead-sources", token, nil).JSON(&list)
	for _, s := range list.Items {
		if s.Id == id {
			return s
		}
	}
	a.t.Fatalf("source %s not found", id)
	return api.LeadSource{}
}

func (a *testApp) jobLeads(token, query string) []api.JobLead {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/job-leads"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.JobLeadList
	res.JSON(&list)
	return list.Items
}

func TestJobLeadsFromACareersPage(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleSourcer, api.RoleViewer)
	token := tenant.Users[api.RoleSourcer].Token
	var openings atomic.Int32
	openings.Store(2)
	site := careersSite(t, &openings)

	src := a.leadSource(token, map[string]any{"kind": "careers_page", "identifier": site.URL + "/careers"})
	assert.Equal(t, "127.0.0.1", src.Name, "named after the site until its markup names the company")
	res := a.request(http.MethodPost, "/v1/lead-sources", token, map[string]any{"kind": "careers_page", "identifier": site.URL + "/careers"})
	assert.Equal(t, []string{"identifier"}, fieldNames(t, res.Error()), "once")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/job-leads", tenant.Users[api.RoleViewer].Token, nil).Status)

	runCoreJobsUntilIdle(t, a, tenant.ID)
	leads := a.jobLeads(token, "?sourceId="+src.Id)
	require.Len(t, leads, 2)
	assert.Equal(t, "Contoh Berhad", leads[0].CompanyName)
	location, _ := leads[0].Location.Get()
	assert.Equal(t, "Kuala Lumpur, MY", location)
	src = a.sourceByID(token, src.Id)
	assert.Equal(t, "Contoh Berhad", src.Name, "the markup names the company")
	assert.Equal(t, 2, src.OpenLeads)
	assert.True(t, src.LastError.IsNull())
	next, _ := src.NextCheckAt.Get()
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), next, time.Minute, "checked again tomorrow")

	t.Run("an opening no longer listed is closed", func(t *testing.T) {
		openings.Store(1)
		require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/lead-sources/"+src.Id+"/checks", token, nil).Status)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		closed := a.jobLeads(token, "?open=false")
		require.Len(t, closed, 1)
		assert.Equal(t, "HR Executive", closed[0].Title)
		assert.Len(t, a.jobLeads(token, "?open=true&q=account"), 1)
	})

	t.Run("robots.txt is respected, and a page without markup closes nothing", func(t *testing.T) {
		private := a.leadSource(token, map[string]any{"kind": "careers_page", "identifier": site.URL + "/private/jobs", "name": "Private"})
		plain := a.leadSource(token, map[string]any{"kind": "careers_page", "identifier": site.URL + "/plain", "name": "Plain"})
		runCoreJobsUntilIdle(t, a, tenant.ID)
		msg, _ := a.sourceByID(token, private.Id).LastError.Get()
		assert.Contains(t, msg, "robots.txt")
		msg, _ = a.sourceByID(token, plain.Id).LastError.Get()
		assert.Contains(t, msg, "no job markup")
	})

	t.Run("ads saved by hand, statuses and companies", func(t *testing.T) {
		company := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd"})
		res := a.request(http.MethodPost, "/v1/job-leads", token, map[string]any{"companyName": " Maju  Sdn Bhd ", "title": "Finance Manager", "url": "https://example.com/ad/9"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var lead api.JobLead
		res.JSON(&lead)
		assert.Equal(t, "Maju Sdn Bhd", lead.CompanyName)
		assert.True(t, lead.SourceId.IsNull())
		res = a.request(http.MethodPatch, "/v1/job-leads/"+lead.Id, token, map[string]any{"version": lead.Version, "status": "contacted", "companyId": company.Id, "notes": "Called HR"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res = a.request(http.MethodPatch, "/v1/job-leads/"+lead.Id, token, map[string]any{"version": lead.Version + 1, "companyId": "comp_01jbz6w0m0k8q9x2r3s4t5v6w7"})
		assert.Equal(t, []string{"companyId"}, fieldNames(t, res.Error()))
		assert.Len(t, a.jobLeads(token, "?status=contacted"), 1)
		res = a.request(http.MethodPost, "/v1/job-leads", token, map[string]any{"companyName": "x", "title": "y", "url": "javascript:alert(1)"})
		assert.Equal(t, []string{"url"}, fieldNames(t, res.Error()))
	})

	t.Run("turned off, a source is not checked; another company sees nothing", func(t *testing.T) {
		src = a.sourceByID(token, src.Id)
		res := a.request(http.MethodPatch, "/v1/lead-sources/"+src.Id, token, map[string]any{"version": src.Version, "active": false})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.True(t, a.sourceByID(token, src.Id).NextCheckAt.IsNull())
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Empty(t, a.jobLeads(stranger, ""))
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/lead-sources/"+src.Id, token, nil).Status)
		assert.Len(t, a.jobLeads(token, "?q=executive"), 2, "leads stay when their source goes")
	})
}
