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

func TestJobLeadsWorkspacePagesAndKeepsFavoritesPersonal(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	token, colleague := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	var ids []string
	for _, title := range []string{"First", "Second", "Third"} {
		res := a.request(http.MethodPost, "/v1/job-leads", token, map[string]any{"title": title, "companyName": "Fixture company"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var lead api.JobLead
		res.JSON(&lead)
		ids = append(ids, lead.Id)
	}

	var first api.JobLeadList
	res := a.request(http.MethodGet, "/v1/job-leads?limit=2", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&first)
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.NextCursor)
	second := a.jobLeads(token, "?limit=2&cursor="+*first.NextCursor)
	require.Len(t, second, 1)
	assert.Equal(t, ids[0], second[0].Id)
	assert.Empty(t, a.jobLeads(token, "?favorites=true"))

	path := "/v1/job-leads/" + ids[1]
	for range 2 {
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodPut, path+"/favorite", token, map[string]any{"favorite": true}).Status)
	}
	require.Len(t, a.jobLeads(token, "?favorites=true"), 1)
	assert.Empty(t, a.jobLeads(colleague, "?favorites=true"))
	var view api.JobLead
	res = a.request(http.MethodGet, path, token, nil)
	require.Equal(t, http.StatusOK, res.Status)
	res.JSON(&view)
	require.NotNil(t, view.Favorite)
	assert.True(t, *view.Favorite)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path, other, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPut, path+"/favorite", other, map[string]any{"favorite": false}).Status)
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodPut, path+"/favorite", token, map[string]any{"favorite": false}).Status)
	assert.Empty(t, a.jobLeads(token, "?favorites=true"))
}

func TestLeadDiscoveryUsesTheSelectedCVAndVerifiedFeedLinks(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter]
	docCandidate := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Aina"})
	other := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Another"})
	res := a.upload("/v1/candidates/"+docCandidate.Id+"/documents", recruiter.Token, "original", "fixture.txt",
		[]byte("Senior Database Administrator. Oracle RAC and Exadata specialist. I collaborated with HR business partners."))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var cv api.Document
	res.JSON(&cv)
	src := a.leadSource(recruiter.Token, map[string]any{"kind": "greenhouse", "identifier": "fixture", "name": "Fixture company"})
	_, err := testutil.Current.Admin.Exec(context.Background(), `
		INSERT INTO job_lead (tenant_id, source_id, source_kind, external_id, company_name, title, location, url, closed_at, created_by)
		SELECT $1, $2, 'greenhouse', id, 'Fixture company', title, location, url,
		  CASE WHEN id='6' THEN now() END, $3
		FROM (VALUES
		  ('1','Database Engineer','Kuala Lumpur, MY','https://boards.greenhouse.io/fixture/jobs/1?gh_src=track'),
		  ('2','HR Business Partner','Kuala Lumpur, MY','https://boards.greenhouse.io/fixture/jobs/2'),
		  ('3','Senior DBA','Singapore','https://boards.greenhouse.io/fixture/jobs/3'),
		  ('4','Database Administrator','Remote APAC','https://boards.greenhouse.io/fixture/jobs/4'),
		  ('5','Database Administrator','Penang','https://www.google.com/search?q=jobs'),
		  ('6','Database Administrator','Johor','https://boards.greenhouse.io/fixture/jobs/6'),
		  ('7','Database Administrator','Johor','https://boards.greenhouse.io/fixture/jobs/99')
		) AS ads(id,title,location,url)`, tenant.ID, src.Id, recruiter.ID)
	require.NoError(t, err)

	query := map[string]any{"candidateId": docCandidate.Id, "documentId": cv.Id, "countries": []string{"MY"}}
	res = a.request(http.MethodPost, "/v1/job-leads/discover", recruiter.Token, query)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var found api.LeadDiscoveryPage
	res.JSON(&found)
	require.Len(t, found.Items, 1)
	link, _ := found.Items[0].Url.Get()
	assert.Equal(t, "https://job-boards.greenhouse.io/fixture/jobs/1", link)
	assert.Equal(t, "Database Administrator", found.Role)

	query["targetRole"] = "HR Business Partner"
	res = a.request(http.MethodPost, "/v1/job-leads/discover", recruiter.Token, query)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&found)
	require.Len(t, found.Items, 1)
	assert.Equal(t, "HR Business Partner", found.Items[0].Title, "an explicit target can be a career change")

	query["candidateId"] = other.Id
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/job-leads/discover", recruiter.Token, query).Status)
}
