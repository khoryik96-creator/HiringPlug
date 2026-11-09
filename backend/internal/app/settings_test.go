package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) fieldSettings(token string) api.FieldSettings {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/settings/fields", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var s api.FieldSettings
	res.JSON(&s)
	return s
}

func optionKeys(s api.FieldSettings, name api.FieldListName) []string {
	var keys []string
	for _, l := range s.Lists {
		if l.Name == name {
			for _, o := range l.Options {
				keys = append(keys, o.Key)
			}
		}
	}
	return keys
}

func TestCompaniesChooseTheirSourcesAndMandatoryFields(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter, api.RoleViewer)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token

	s := a.fieldSettings(tenant.Users[api.RoleViewer].Token)
	assert.Equal(t, []string{"jobstreet", "linkedin", "referral", "careers_page", "direct", "other"}, optionKeys(s, api.FieldListNameCandidateSource))
	assert.Contains(t, optionKeys(s, api.FieldListNameIndustry), "Information Technology & Services")

	sources := map[string]any{"options": []any{
		map[string]any{"label": "Job fair"}, map[string]any{"key": "jobstreet", "label": "JobStreet"},
		map[string]any{"key": "linkedin", "label": "LinkedIn", "hidden": true}, map[string]any{"key": "referral", "label": "Referral"},
		map[string]any{"key": "careers_page", "label": "Careers page"}, map[string]any{"key": "direct", "label": "Direct"},
		map[string]any{"key": "other", "label": "Other"},
	}}
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/settings/fields/lists/candidate_source", recruiter, sources).Status)
	res := a.request(http.MethodPut, "/v1/settings/fields/lists/candidate_source", admin, sources)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.FieldList
	res.JSON(&list)
	assert.Equal(t, "job_fair", list.Options[0].Key)
	assert.Equal(t, "Job fair", list.Options[0].Label)
	assert.False(t, list.Options[0].Hidden)
	assert.False(t, list.Options[0].BuiltIn)
	assert.True(t, list.Options[0].Hint.IsNull(), "sources carry no hint")

	t.Run("a candidate's source comes from the company's list", func(t *testing.T) {
		c := a.createCandidate(recruiter, map[string]any{"firstName": "Aina", "source": "job_fair"})
		assert.Equal(t, api.CandidateSource("job_fair"), c.Source.MustGet())
		assert.Equal(t, api.CandidateSource("linkedin"), a.createCandidate(recruiter, map[string]any{"firstName": "Bala", "source": "linkedin"}).Source.MustGet(), "hidden options stay valid")
		res := a.request(http.MethodPost, "/v1/candidates", recruiter, map[string]any{"firstName": "Chong", "source": "roadshow"})
		assert.Equal(t, []string{"source"}, fieldNames(t, res.Error()))
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		res = a.request(http.MethodPost, "/v1/candidates", other, map[string]any{"firstName": "Dev", "source": "job_fair"})
		assert.Equal(t, []string{"source"}, fieldNames(t, res.Error()), "each company has its own list")
	})

	old := a.createCandidate(recruiter, map[string]any{"firstName": "Old", "phone": "+60 12-345 6789"})
	oldJob := a.createJob(recruiter, map[string]any{"companyId": a.createCompany(recruiter, map[string]any{"name": "Maju"}).Id, "title": "Clerk"})
	res = a.request(http.MethodPut, "/v1/settings/fields/mandatory", admin, map[string]any{"fields": []any{
		map[string]any{"form": "candidate", "field": "email", "mandatory": true},
		map[string]any{"form": "candidate", "field": "phone", "mandatory": true},
		map[string]any{"form": "job", "field": "location", "mandatory": true},
		map[string]any{"form": "job", "field": "country", "mandatory": true},
	}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	t.Run("mandatory fields are needed on new records and cannot be emptied", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates", recruiter, map[string]any{"firstName": "Eng"})
		assert.ElementsMatch(t, []string{"email", "phone"}, fieldNames(t, res.Error()))
		a.createCandidate(recruiter, map[string]any{"firstName": "Eng", "email": "eng@example.com", "phone": "+60 12-222 3333"})

		res = a.request(http.MethodPatch, "/v1/candidates/"+old.Id, recruiter, map[string]any{"version": old.Version, "currentTitle": "Clerk"})
		assert.Equal(t, http.StatusOK, res.Status, "an older record without an email can still be edited: %s", res.Body)
		var updated api.Candidate
		res.JSON(&updated)
		res = a.request(http.MethodPatch, "/v1/candidates/"+old.Id, recruiter, map[string]any{"version": updated.Version, "phone": nil})
		assert.Equal(t, []string{"phone"}, fieldNames(t, res.Error()))

		company := a.createCompany(recruiter, map[string]any{"name": "Contoh"})
		res = a.request(http.MethodPost, "/v1/jobs", recruiter, map[string]any{"companyId": company.Id, "title": "Analyst"})
		assert.ElementsMatch(t, []string{"location", "country"}, fieldNames(t, res.Error()))
		res = a.request(http.MethodPatch, "/v1/jobs/"+oldJob.Id, recruiter, map[string]any{"version": oldJob.Version, "openings": 2})
		assert.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	})

	t.Run("the app's own required fields stay on", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/settings/fields/mandatory", admin, map[string]any{"fields": []any{
			map[string]any{"form": "candidate", "field": "firstName", "mandatory": false},
		}})
		assert.Equal(t, []string{"fields[0].mandatory"}, fieldNames(t, res.Error()))
	})
}
