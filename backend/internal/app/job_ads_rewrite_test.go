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

const longAd = "Maju Logistik Sdn Bhd runs 40 hubs across 12 countries.\n" +
	"- Run the Shah Alam hub with SAP and Power BI\n- Lead a team of 30\nSend your CV to Aisyah Rahman."

func rewriteAnswer(id string, jd *api.BlindJd, reason string) api.JdBlindedJob {
	out := api.JdBlindedJob{RewriteId: id, Jd: nullable.NewNullNullable[api.BlindJd](), Error: nullable.NewNullNullable[string](),
		Warnings: []string{}, Model: nullable.NewNullableWithValue("test-model"), InputTokens: 1200, OutputTokens: 400}
	if jd != nil {
		out.Jd = nullable.NewNullableWithValue(*jd)
		out.Warnings = []string{"Check the tech stack: Kafka from the ad may be missing."}
		out.Retried = true
	}
	if reason != "" {
		out.Error = nullable.NewNullableWithValue(reason)
	}
	return out
}

func TestABlindAdIsRewrittenByTheAICVStudiosWay(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd", "website": "https://majulogistik.my"})
	contact := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "contactId": contact.Id, "title": "Hub Manager"})
	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{
		"title": "Hub Manager", "description": longAd, "location": "Shah Alam", "workArrangement": "hybrid",
		"clientDescription": "a regional logistics company",
	}, http.StatusCreated)
	rewrites := "/v1/job-ads/" + ad.Id + "/blind-rewrites"

	short := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Clerk", "description": "Filing."}, http.StatusCreated)
	res := a.request(http.MethodPost, "/v1/job-ads/"+short.Id+"/blind-rewrites", token, map[string]any{"tone": "professional", "strength": "medium"})
	assert.Equal(t, api.ErrorCodeJOBADBLINDTOOSHORT, res.Error().Error.Code)
	viewer := tenant.Users[api.RoleViewer].Token
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, rewrites, viewer, map[string]any{"tone": "professional", "strength": "medium"}).Status)

	var run api.BlindRewrite
	res = a.request(http.MethodPost, rewrites, token, map[string]any{"tone": "concise", "strength": "high", "removeAlso": "Project Falcon"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	res.JSON(&run)
	assert.Equal(t, api.BlindRewriteStatusQueued, run.Status)

	claimed := claimAIJobWhere(t, aiConn, "rewriteId", run.Id)
	require.Equal(t, string(api.JobKindJdBlind), claimed.Kind)
	var asked api.JdBlindJob
	require.NoError(t, json.Unmarshal(claimed.Payload, &asked))
	assert.Equal(t, "Hub Manager\n\n"+longAd, asked.Raw, "the ad as written")
	assert.Equal(t, "Maju Logistik Sdn Bhd", asked.CompanyName)
	assert.Equal(t, "Hybrid", asked.WorkMode.MustGet())
	assert.Equal(t, "a regional logistics company", asked.ClientDescription.MustGet())
	assert.Equal(t, "Project Falcon, https://majulogistik.my, Aisyah Rahman", asked.RemoveAlso.MustGet(), "and the client's website and contact")
	assert.Equal(t, api.BlindRewriteStrengthHigh, asked.Strength)

	jd := api.BlindJd{
		JobTitle: "Hub Manager", AboutClient: "A regional logistics group with a broad footprint.", AboutRole: "Lead a busy hub.",
		TechStack: []string{"SAP", "Power BI"}, Responsibilities: []string{"Keep a large sorting hub running smoothly."},
		Requirements: []string{"Experience leading a team of 30."}, NiceToHave: []string{}, WhyJoin: []string{"Hybrid work."}, AltTitles: []string{},
	}
	answerAIJob(t, a, aiConn, claimed, api.JobKindJdBlinded, rewriteAnswer(run.Id, &jd, ""))

	got := a.adRequest(http.MethodGet, "/v1/job-ads/"+ad.Id, token, nil, http.StatusOK)
	assert.True(t, got.BlindEdited, "it is the ad's blind version now, as a person's edit")
	assert.False(t, got.BlindOutdated)
	assert.Equal(t, "Hub Manager", got.BlindTitle)
	assert.Equal(t, "About our client\nA regional logistics group with a broad footprint.\n\nLead a busy hub.\n\nTech stack: SAP, Power BI\n\n"+
		"What you will do\n• Keep a large sorting hub running smoothly.\n\nWhat you need to succeed\n• Experience leading a team of 30.\n\n"+
		"Why join us\n• Hybrid work.", got.BlindDescription.MustGet())

	var list api.BlindRewriteList
	a.request(http.MethodGet, rewrites, viewer, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	done := list.Items[0]
	assert.Equal(t, api.BlindRewriteStatusDone, done.Status)
	assert.Equal(t, 1600, done.Tokens)
	assert.True(t, done.Retried)
	assert.Equal(t, []string{"Check the tech stack: Kafka from the ad may be missing."}, done.Warnings)

	t.Run("a rewrite still naming the client is not used", func(t *testing.T) {
		var again api.BlindRewrite
		a.request(http.MethodPost, rewrites, token, map[string]any{"tone": "professional", "strength": "medium"}).JSON(&again)
		named := jd
		named.AboutClient = "Maju Logistik is a regional logistics group."
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "rewriteId", again.Id), api.JobKindJdBlinded, rewriteAnswer(again.Id, &named, ""))
		var list api.BlindRewriteList
		a.request(http.MethodGet, rewrites, token, nil).JSON(&list)
		assert.Equal(t, api.BlindRewriteStatusFailed, list.Items[0].Status)
		assert.True(t, strings.HasPrefix(list.Items[0].Error.MustGet(), "The AI's version still names the client"))
		after := a.adRequest(http.MethodGet, "/v1/job-ads/"+ad.Id, token, nil, http.StatusOK)
		assert.Equal(t, got.BlindDescription.MustGet(), after.BlindDescription.MustGet(), "the blind ad is unchanged")
	})

	t.Run("changes to the ad while the AI works still mark the blind version out of date", func(t *testing.T) {
		var again api.BlindRewrite
		a.request(http.MethodPost, rewrites, token, map[string]any{"tone": "professional", "strength": "medium"}).JSON(&again)
		claimed := claimAIJobWhere(t, aiConn, "rewriteId", again.Id)
		current := a.adRequest(http.MethodGet, "/v1/job-ads/"+ad.Id, token, nil, http.StatusOK)
		a.adRequest(http.MethodPatch, "/v1/job-ads/"+ad.Id, token, map[string]any{
			"version": current.Version, "description": longAd + "\n- Also run the Penang hub",
		}, http.StatusOK)
		answerAIJob(t, a, aiConn, claimed, api.JobKindJdBlinded, rewriteAnswer(again.Id, &jd, ""))
		after := a.adRequest(http.MethodGet, "/v1/job-ads/"+ad.Id, token, nil, http.StatusOK)
		assert.True(t, after.BlindOutdated, "the AI rewrote the ad as it was before the Penang hub was added")
	})

	t.Run("a failed rewrite says why, and its tokens count", func(t *testing.T) {
		var again api.BlindRewrite
		a.request(http.MethodPost, rewrites, token, map[string]any{"tone": "professional", "strength": "low"}).JSON(&again)
		answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "rewriteId", again.Id), api.JobKindJdBlinded,
			rewriteAnswer(again.Id, nil, "the AI's answer could not be read"))
		var list api.BlindRewriteList
		a.request(http.MethodGet, rewrites, token, nil).JSON(&list)
		assert.Equal(t, "It could not be rewritten: the AI's answer could not be read", list.Items[0].Error.MustGet())
		assert.Equal(t, 1600, list.Items[0].Tokens)
	})
}

func TestABlindAdIsNotRewrittenWhileTheAIIsOff(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Hub Manager"})
	ad := a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{"title": "Hub Manager", "description": longAd}, http.StatusCreated)
	res := a.request(http.MethodPost, "/v1/job-ads/"+ad.Id+"/blind-rewrites", token, map[string]any{"tone": "professional", "strength": "medium"})
	assert.Equal(t, api.ErrorCodeJOBADBLINDAIOFF, res.Error().Error.Code)
}
