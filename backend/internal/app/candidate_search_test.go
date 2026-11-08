package app_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestAdvancedSearchFindsByFiltersAndABooleanOfTheirCVs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	token := tenant.Users[api.RoleRecruiter].Token
	add := func(body map[string]any, cv string) (api.Candidate, string) {
		c := a.createCandidate(token, body)
		doc := ""
		if cv != "" {
			doc = withCV(t, a, aiConn, token, c.Id, cv)
		}
		return c, doc
	}
	aina, ainaCV := add(map[string]any{"firstName": "Aina", "currentTitle": "SAP Consultant", "nationality": "MY", "residenceCountry": "MY",
		"workAuthorization": map[string]any{"status": "local"}, "availability": "looking", "noticePeriod": map[string]any{"value": 1, "unit": "month"},
		"skills": []string{"SAP", "ABAP"}, "industry": "Banking"},
		"Led SAP BW upgrades and BPC planning for a regional bank. Managed a team of five.")
	ben, _ := add(map[string]any{"firstName": "Ben", "currentTitle": "Data Engineer", "nationality": "IN", "residenceCountry": "MY",
		"workAuthorization": map[string]any{"status": "expat_eligible", "basis": "rp_t"}, "availability": "passive",
		"noticePeriod": map[string]any{"value": 3, "unit": "month"}, "skills": []string{"Python", "SAP BW"}},
		"SAP BW internship, then Python and Spark pipelines.")
	chris, _ := add(map[string]any{"firstName": "Chris", "currentTitle": "SAP BPC Lead", "nationality": "SG",
		"workAuthorization": map[string]any{"status": "requires_visa"}, "skills": []string{"SAP BPC"}}, "")
	res := a.request(http.MethodPut, "/v1/candidates/"+aina.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": nil, "expectedSalary": map[string]any{"min": "9000", "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+ben.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": nil, "expectedSalary": map[string]any{"min": "180000", "currency": "MYR", "period": "year"},
	}).Status)

	search := func(as string, body map[string]any) (api.CandidateSearchPage, *response) {
		var page api.CandidateSearchPage
		res := a.request(http.MethodPost, "/v1/candidates/search", as, body)
		if res.Status == http.StatusOK {
			res.JSON(&page)
		}
		return page, res
	}
	ids := func(p api.CandidateSearchPage) []string {
		var out []string
		for _, h := range p.Items {
			out = append(out, h.Candidate.Id)
		}
		return out
	}

	page, res := search(token, map[string]any{"boolean": "SAP AND (BW OR BPC) NOT internship"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, []string{chris.Id, aina.Id}, ids(page), "Ben's CV says internship (his profile matching does not count); Chris has no CV but his profile matches")
	assert.Equal(t, []string{"sap", "bw", "bpc"}, page.Terms)
	assert.Equal(t, []api.CandidateSearchHitMatchedIn{api.CandidateSearchHitMatchedInCv}, page.Items[1].MatchedIn)
	assert.Contains(t, page.Items[1].CvSnippet.MustGet(), "[[SAP]] [[BW]]")
	assert.Equal(t, []api.CandidateSearchHitMatchedIn{api.CandidateSearchHitMatchedInProfile}, page.Items[0].MatchedIn)
	assert.True(t, page.Items[0].CvSnippet.IsNull())

	page, _ = search(token, map[string]any{"boolean": `"team of five" OR manag*`})
	assert.Equal(t, []string{aina.Id}, ids(page), "a phrase, and managed for manag*")

	page, _ = search(token, map[string]any{"workAuthorization": []string{"expat", "needs_pass"}})
	assert.ElementsMatch(t, []string{ben.Id, chris.Id}, ids(page))
	page, _ = search(token, map[string]any{"residence": []string{"MY"}, "nationality": []string{"IN", "MY"}, "availability": []string{"looking"}})
	assert.Equal(t, []string{aina.Id}, ids(page))
	page, _ = search(token, map[string]any{"skills": []string{"sap bw", "python"}})
	assert.Equal(t, []string{ben.Id}, ids(page), "all the skills, ignoring case")
	page, _ = search(token, map[string]any{"skills": []string{"abap", "python"}, "skillsMatch": "any"})
	assert.ElementsMatch(t, []string{aina.Id, ben.Id}, ids(page))
	page, _ = search(token, map[string]any{"noticeMaxDays": 30})
	assert.Equal(t, []string{aina.Id}, ids(page), "Ben's 3 months is longer; Chris has none")
	page, _ = search(token, map[string]any{"industry": []string{"banking"}})
	assert.Equal(t, []string{aina.Id}, ids(page))
	page, _ = search(token, map[string]any{"expectedSalaryMax": map[string]any{"amount": "15000", "currency": "MYR"}})
	assert.Equal(t, []string{ben.Id, aina.Id}, ids(page), "RM 180,000 a year is RM 15,000 a month")
	page, _ = search(token, map[string]any{"expectedSalaryMax": map[string]any{"amount": "10000", "currency": "MYR"}})
	assert.Equal(t, []string{aina.Id}, ids(page))

	sourcer := tenant.Users[api.RoleSourcer].Token
	_, res = search(sourcer, map[string]any{"expectedSalaryMax": map[string]any{"amount": "10000", "currency": "MYR"}})
	assert.Equal(t, http.StatusBadRequest, res.Status, "salary is not searchable without seeing it")
	_, res = search(token, map[string]any{"boolean": "SAP AND (BW"})
	assert.Equal(t, http.StatusBadRequest, res.Status)
	assert.Contains(t, string(res.Body), "( without its )")

	t.Run("the CV shows where the words are", func(t *testing.T) {
		var hl api.DocumentHighlights
		res := a.request(http.MethodGet, "/v1/documents/"+ainaCV+"/highlights?boolean="+url.QueryEscape("SAP AND (BW OR BPC) manag*"), token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&hl)
		var marked []string
		var text strings.Builder
		for _, p := range hl.Parts {
			if p.Match {
				marked = append(marked, p.Text)
			}
			text.WriteString(p.Text)
		}
		assert.Equal(t, []string{"SAP", "BW", "BPC", "Managed"}, marked)
		assert.Equal(t, 4, hl.Matches)
		assert.Equal(t, "Led SAP BW upgrades and BPC planning for a regional bank. Managed a team of five.", text.String(), "the whole text, unchanged")
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+ainaCV+"/highlights?boolean=SAP", other, nil).Status)
	})
}
