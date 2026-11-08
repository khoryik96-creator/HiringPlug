package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestTheAIRanksTheCandidatesASearchFindsForAJD(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	add := func(first, title, cv string) api.Candidate {
		c := a.createCandidate(token, map[string]any{"firstName": first, "lastName": "Tan", "email": strings.ToLower(first) + "@example.com",
			"currentTitle": title, "residenceCountry": "MY", "noticePeriod": map[string]any{"value": 1, "unit": "month"}})
		withCV(t, a, aiConn, token, c.Id, cv)
		return c
	}
	best := add("Aina", "SAP Consultant", "Aina Tan, aina@example.com. Led SAP BW and BPC planning for a bank; ABAP; team of five. Expected salary RM 12,000.")
	other := add("Ben", "Data Engineer", "Ben Tan. Python pipelines; some SAP BW extracts.")
	add("Chris", "Barista", "Chris Tan. Coffee and latte art.")
	away := a.createCandidate(token, map[string]any{"firstName": "Dina", "residenceCountry": "SG"})
	withCV(t, a, aiConn, token, away.Id, "SAP BW and BPC lead in Singapore")
	noConsent := add("Eva", "SAP Lead", "Eva Tan. SAP BW, BPC, ABAP.")
	withdrawConsent(t, a, token, noConsent.Id)

	var search api.TalentSearch
	res := a.request(http.MethodPost, "/v1/talent-searches", token, map[string]any{
		"jd":     "About us: a great bank with free coffee.\nRequirements: SAP BW, BPC planning, ABAP. Salary RM 15,000.",
		"search": map[string]any{"residence": []string{"MY"}, "boolean": "SAP OR barista OR coffee"},
		"rank":   3,
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	res.JSON(&search)
	assert.Equal(t, api.TalentSearchStatusQueued, search.Status)
	assert.Equal(t, 4, search.Considered, "Dina lives in Singapore")
	assert.Equal(t, 3, search.Sent, "not Eva, who withdrew consent")

	claimed := claimAIJobWhere(t, aiConn, "searchId", search.Id)
	require.Equal(t, string(api.JobKindTalentRank), claimed.Kind)
	var asked api.TalentRankJob
	require.NoError(t, json.Unmarshal(claimed.Payload, &asked))
	require.Len(t, asked.Candidates, 3)
	assert.Equal(t, "C1", asked.Candidates[0].Ref)
	assert.Contains(t, asked.Candidates[0].Cv, "SAP BW and BPC", "the CV sharing most words with the JD first")
	sentText, _ := json.Marshal(asked)
	for _, secret := range []string{"Aina", "aina@example.com", "12,000", "15,000"} {
		assert.NotContains(t, string(sentText), secret, "no names, contact details or salary go to the AI")
	}
	assert.Contains(t, asked.Candidates[0].Profile, "Current title: SAP Consultant")

	answerAIJob(t, a, aiConn, claimed, api.JobKindTalentRanked, api.TalentRankedJob{
		SearchId: search.Id, Summary: nullable.NewNullableWithValue("Two SAP profiles; BPC is rare."),
		MustHaves: []string{"SAP BW", "BPC"}, NiceToHaves: []string{"ABAP"}, SuggestedBoolean: nullable.NewNullableWithValue("SAP AND BW AND BPC"),
		Results: []api.TalentRanked{
			{Ref: "C2", Fit: 45, Verdict: "weak", Reasons: []string{"Some SAP BW"}, Gaps: []string{"No BPC"}},
			{Ref: "C1", Fit: 88, Verdict: "strong", Reasons: []string{"SAP BW and BPC"}, Gaps: []string{}},
		},
		Error: nullable.NewNullNullable[string](), Model: nullable.NewNullableWithValue("test-model"), InputTokens: 3000, OutputTokens: 500,
	})

	a.request(http.MethodGet, "/v1/talent-searches/"+search.Id, token, nil).JSON(&search)
	assert.Equal(t, api.TalentSearchStatusDone, search.Status)
	require.Len(t, search.Results, 2)
	assert.Equal(t, best.Id, search.Results[0].CandidateId, "best fit first")
	assert.Equal(t, "Aina Tan", search.Results[0].Name)
	assert.Equal(t, other.Id, search.Results[1].CandidateId)
	assert.Equal(t, []string{"No BPC"}, search.Results[1].Gaps)
	assert.Equal(t, "SAP AND BW AND BPC", search.SuggestedBoolean.MustGet())
	assert.Equal(t, []string{"SAP BW", "BPC"}, search.MustHaves)

	var list api.TalentSearchList
	a.request(http.MethodGet, "/v1/talent-searches", token, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	other2 := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/talent-searches/"+search.Id, other2, nil).Status)

	t.Run("nobody matching asks no AI", func(t *testing.T) {
		var none api.TalentSearch
		res := a.request(http.MethodPost, "/v1/talent-searches", token, map[string]any{"search": map[string]any{"boolean": "astronaut"}})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&none)
		assert.Equal(t, api.TalentSearchStatusDone, none.Status)
		assert.Equal(t, 0, none.Sent)
		assert.Contains(t, none.Summary.MustGet(), "No candidates match")
	})
	t.Run("a JD or a boolean is needed", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/talent-searches", token, map[string]any{"search": map[string]any{}})
		assert.Equal(t, http.StatusBadRequest, res.Status)
	})
}
