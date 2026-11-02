package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func job(title, company, start string, end any, current bool) map[string]any {
	return map[string]any{"title": title, "company": company, "startDate": start, "endDate": end, "current": current, "description": nil}
}

func TestCandidateWorkHistoryAndEducation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	assert.Empty(t, c.Experience)
	assert.Empty(t, c.Education)
	path := "/v1/candidates/" + c.Id + "/history"
	body := map[string]any{
		"version": c.Version,
		"experience": []any{
			job("  Senior   Accountant ", "Nestlé Malaysia Berhad", "2020-01", nil, true),
			job("Accountant", "ABC Sdn Bhd", "2016-03", "2019-12", false),
		},
		"education": []any{map[string]any{"qualification": "Bachelor of Accounting", "institution": "Universiti Malaya", "endYear": 2015}},
	}

	res := a.request(http.MethodPut, path, token, body)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.Candidate
	res.JSON(&saved)
	assert.Equal(t, c.Version+1, saved.Version, "the candidate's version moves on")
	require.Len(t, saved.Experience, 2)
	assert.Equal(t, "Senior Accountant", saved.Experience[0].Title.MustGet(), "spaces tidied")
	assert.True(t, saved.Experience[0].Current)
	assert.Equal(t, "2019-12", saved.Experience[1].EndDate.MustGet())
	assert.Equal(t, 2015, saved.Education[0].EndYear.MustGet())

	got := a.request(http.MethodGet, "/v1/candidates/"+c.Id, tenant.Users[api.RoleViewer].Token, nil)
	var read api.Candidate
	got.JSON(&read)
	assert.Equal(t, saved.Experience, read.Experience, "everyone who can see the candidate sees the history")

	t.Run("a save based on an older version is a conflict", func(t *testing.T) {
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, path, token, body).Status)
	})

	t.Run("entries are checked", func(t *testing.T) {
		bad := map[string]any{
			"version": saved.Version,
			"experience": []any{
				job("", "", "2020-01", nil, false),            // no title or company
				job("Clerk", "", "2021-01", "2020-12", false), // ends before it starts
				job("Clerk", "", "2021-13", nil, false),       // no such month
				job("Clerk", "", "2021-01", "2022-01", true),  // current, but ended
			},
			"education": []any{map[string]any{"qualification": " ", "institution": nil, "endYear": nil}},
		}
		res := a.request(http.MethodPut, path, token, bad)
		require.Equal(t, http.StatusBadRequest, res.Status)
		for _, field := range []string{"experience[0].title", "experience[1].endDate", "experience[2].startDate", "experience[3].endDate", "education[0].qualification"} {
			assert.Contains(t, string(res.Body), field)
		}
	})

	t.Run("viewers cannot change it", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, path, tenant.Users[api.RoleViewer].Token, body).Status)
	})

	t.Run("an empty list clears it", func(t *testing.T) {
		res := a.request(http.MethodPut, path, token, map[string]any{"version": saved.Version, "experience": []any{}, "education": []any{}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var cleared api.Candidate
		res.JSON(&cleared)
		assert.Empty(t, cleared.Experience)
		assert.Empty(t, cleared.Education)
	})
}

func TestMergingMovesHistoryOnlyWhenTheKeptCandidateHasNone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	keep := a.createCandidate(token, map[string]any{"firstName": "Wei"})
	dup := a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Tan"})
	res := a.request(http.MethodPut, "/v1/candidates/"+dup.Id+"/history", token, map[string]any{
		"version": dup.Version, "experience": []any{job("Engineer", "Grab", "2019", nil, true)}, "education": []any{},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res = a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var merged api.Candidate
	res.JSON(&merged)
	require.Len(t, merged.Experience, 1)
	assert.Equal(t, "Grab", merged.Experience[0].Company.MustGet())
}
