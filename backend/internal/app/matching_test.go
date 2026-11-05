package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var matchingOn = map[string]string{"HP_AI_PROVIDER": "openai_compatible", "HP_AI_MONTHLY_TOKENS": "20000"}

func (a *testApp) matches(token, query string) api.MatchList {
	a.t.Helper()
	var list api.MatchList
	res := a.request(http.MethodGet, "/v1/matches?"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	return list
}

func (a *testApp) scoreFit(token, applicationID string) api.Match {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": applicationID})
	require.Equal(a.t, http.StatusAccepted, res.Status, "%s", res.Body)
	var m api.Match
	res.JSON(&m)
	return m
}

// withCV uploads a CV for the candidate and plays the AI worker reading its text.
func withCV(t *testing.T, a *testApp, aiConn *pgx.Conn, token, candidateID, text string) {
	t.Helper()
	res := a.upload("/v1/candidates/"+candidateID+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	answerAIJob(t, a, aiConn, claimAIJobFor(t, aiConn, doc.Id), api.JobKindDocumentTextExtracted, extractedText(doc.Id, text))
}

// aiScores plays the Python worker: it claims the match's question and answers it.
func aiScores(t *testing.T, a *testApp, conn *pgx.Conn, matchID string, answer api.MatchScoredJob) api.MatchScoreJob {
	t.Helper()
	job := claimAIJobWhere(t, conn, "matchId", matchID)
	require.Equal(t, string(api.JobKindMatchScore), job.Kind)
	var asked api.MatchScoreJob
	require.NoError(t, json.Unmarshal(job.Payload, &asked))
	answer.MatchId = matchID
	if answer.Claims == nil {
		answer.Claims = []api.MatchClaim{}
	}
	answerAIJob(t, a, conn, job, api.JobKindMatchScored, answer)
	return asked
}

func claim(req string, verdict api.MatchVerdict, evidence string) api.MatchClaim {
	c := api.MatchClaim{Requirement: req, Verdict: verdict, Evidence: nullable.NewNullNullable[string](), Note: nullable.NewNullNullable[string]()}
	if evidence != "" {
		c.Evidence = nullable.NewNullableWithValue(evidence)
	}
	return c
}

func dim(name api.MatchDimensionName, score int, note string) api.MatchDimension {
	return api.MatchDimension{Name: name, Score: score, Note: nullable.NewNullableWithValue(note)}
}

func scoredWith(claims ...api.MatchClaim) api.MatchScoredJob {
	return api.MatchScoredJob{
		Dimensions: []api.MatchDimension{
			dim("keywords", 55, "Most required tools appear."), dim("skills", 120, "Strong IFRS and SAP."), dim("skills", 10, "a second rating"),
			dim("charisma", 90, "not one of the six"), dim("experience", 60, "Fewer audit years than asked."),
			dim("industry", 70, "FMCG finance."), dim("education", 85, "Accounting degree."), dim("leadership", 40, "No team lead roles."),
		},
		Strengths: []string{"Hands-on SAP S/4HANA", "  hands-on  SAP S/4HANA ", "IFRS reporting", "", "a", "b", "c", "d"},
		Gaps:      []string{"Audit experience shorter than asked"},
		Claims:    claims, Summary: nullable.NewNullableWithValue("A partial fit: strong accounting, less audit."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("deepseek-chat"), InputTokens: 1000, OutputTokens: 200,
	}
}

func TestMatchingQuotesTheJobAndTheCVAndNeverSeesSalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token, viewer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()

	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti.aisyah@mail.com", "phone": "+60 12-345 6789"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8765", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	withCV(t, a, aiConn, token, c.Id, "SITI AISYAH\nsiti.aisyah@mail.com | 012-345 6789\nSenior Accountant, Contoh Sdn Bhd, 2019 – now\n"+
		"Auditor at KPMG, 2016-2019\nBachelor of Accounting, UiTM\nSkills: Excel, SAP FICO\nExpected salary: RM 9,500")
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance Manager",
		"description": "- Degree in Accounting or Finance\n- At least 5 years in audit\nSalary: RM 13,579 a month"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "24680", "currency": "MYR", "period": "month"},
	}).Status)
	a.adRequest(http.MethodPost, "/v1/jobs/"+job.Id+"/ads", token, map[string]any{
		"title": "Finance Manager (KL)", "description": "- Proficient in SAP\n- Malaysians only",
	}, http.StatusCreated)
	appl := a.addToJob(token, job.Id, c.Id)

	list := a.matches(token, "jobId="+job.Id)
	assert.True(t, list.Enabled)
	require.Len(t, list.Items, 1, "putting a candidate with a CV on a job scores them, without being asked")
	queued := list.Items[0]
	assert.Equal(t, api.MatchStatusQueued, queued.Status)
	assert.Equal(t, job.Id, queued.JobId)
	assert.Equal(t, queued.Id, a.scoreFit(token, appl.Id).Id, "a score already waiting is not asked for again")

	asked := aiScores(t, a, aiConn, queued.Id, scoredWith(
		claim("Degree in Accounting or Finance", api.MatchVerdictMet, "Bachelor of Accounting, UiTM"),
		claim("At least 5 years in audit", api.MatchVerdictPartly, "Auditor at KPMG, 2016-2019"),
		claim("Proficient in SAP", api.MatchVerdictMet, "SAP S/4HANA certified"),
		claim("CPA qualification", api.MatchVerdictMet, "Bachelor of Accounting"),
		claim("Malaysians only", api.MatchVerdictMet, "Senior Accountant"),
	))
	for _, leak := range []string{"Siti", "SITI", "Aisyah", "siti.aisyah@mail.com", "345 6789", "8765", "9,500", "13,579", "24680", "24,680"} {
		assert.NotContains(t, asked.Job+asked.Cv, leak, "the AI never sees the name, contact details or salary")
	}
	assert.Contains(t, asked.Job, "- At least 5 years in audit")
	assert.Contains(t, asked.Job, "Job ad (Finance Manager (KL)):\n- Proficient in SAP", "the job's ad too")
	assert.Contains(t, asked.Cv, "Auditor at KPMG, 2016-2019")

	got := a.matches(token, "applicationId="+appl.Id).Items
	require.Len(t, got, 1)
	m := got[0]
	assert.Equal(t, api.MatchStatusDone, m.Status)
	assert.Equal(t, 50, m.Score.MustGet(), "met + half of partly, of 3 requirements")
	assert.Equal(t, 1, m.Dropped, "CPA is not in the job's text")
	require.Len(t, m.Reasons, 3, "nothing about who a person is")
	assert.Equal(t, "Bachelor of Accounting, UiTM", m.Reasons[0].Evidence.MustGet())
	assert.Equal(t, api.MatchVerdictMissing, m.Reasons[2].Verdict, "evidence that is not in the CV does not count")
	assert.Equal(t, "A partial fit: strong accounting, less audit.", m.Summary.MustGet())
	assert.Equal(t, "deepseek-chat", m.Model.MustGet())
	assert.Equal(t, api.MatchRecommendationConsider, m.Recommendation.MustGet(), "50: consider, as CV Studio")
	var names []api.MatchDimensionName
	for _, d := range m.Dimensions {
		names = append(names, d.Name)
	}
	assert.Equal(t, []api.MatchDimensionName{"skills", "experience", "industry", "education", "leadership", "keywords"}, names,
		"CV Studio's six, once each, in order; unknown ones left out")
	assert.Equal(t, 100, m.Dimensions[0].Score, "held to 100")
	assert.Equal(t, "Strong IFRS and SAP.", m.Dimensions[0].Note.MustGet(), "the first rating of a dimension counts")
	assert.Equal(t, []string{"Hands-on SAP S/4HANA", "IFRS reporting", "a", "b", "c"}, m.Strengths, "distinct, at most five")
	assert.Equal(t, []string{"Audit experience shorter than asked"}, m.Gaps)
	assert.Equal(t, []api.Match{m}, a.matches(viewer, "jobId="+job.Id).Items, "everyone who may see the pipeline sees it")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/matches", viewer, map[string]any{"applicationId": appl.Id}).Status)

	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", token, nil).JSON(&status)
	assert.EqualValues(t, 1200, status.UsedTokens, "one AI allowance for matching and the assistant")

	t.Run("another company sees nothing", func(t *testing.T) {
		other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
		assert.Empty(t, a.matches(other, "jobId="+job.Id).Items)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/matches", other, map[string]any{"applicationId": appl.Id}).Status)
	})

	t.Run("every candidate on the job", func(t *testing.T) {
		wei := a.createCandidate(token, map[string]any{"firstName": "Wei"})
		withCV(t, a, aiConn, token, wei.Id, "Audit senior at EY, 2015-2022")
		weiAppl := a.addToJob(token, job.Id, wei.Id)
		a.addToJob(token, job.Id, a.createCandidate(token, map[string]any{"firstName": "Ali"}).Id)
		gone := a.addToJob(token, job.Id, a.createCandidate(token, map[string]any{"firstName": "Gone"}).Id)
		require.Equal(t, http.StatusOK, a.move(token, gone, map[string]any{"stage": "withdrawn", "reason": "candidate_declined"}).Status)

		var res api.MatchJobResult
		r := a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": job.Id})
		require.Equal(t, http.StatusAccepted, r.Status, "%s", r.Body)
		r.JSON(&res)
		assert.Equal(t, api.MatchJobResult{Queued: 0, AlreadyScored: 2, WithoutCv: 1}, res,
			"Siti is scored and Wei was queued when added; Ali has no CV; Gone withdrew")

		a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": job.Id, "rescore": true}).JSON(&res)
		assert.Equal(t, api.MatchJobResult{Queued: 1, AlreadyScored: 1, WithoutCv: 1}, res, "Siti again; Wei is still waiting")

		items := a.matches(token, "jobId="+job.Id).Items
		require.Len(t, items, 2)
		for _, it := range items {
			assert.Equal(t, api.MatchStatusQueued, it.Status)
			aiScores(t, a, aiConn, it.Id, scoredWith(claim("At least 5 years in audit", api.MatchVerdictMet, "2015-2022")))
		}
		w := a.matches(token, "applicationId="+weiAppl.Id).Items[0]
		assert.Equal(t, 100, w.Score.MustGet())
	})

	t.Run("a failed answer says why", func(t *testing.T) {
		q := a.scoreFit(token, appl.Id)
		aiScores(t, a, aiConn, q.Id, api.MatchScoredJob{
			Claims: []api.MatchClaim{}, Summary: nullable.NewNullNullable[string](), Model: nullable.NewNullNullable[string](),
			Error: nullable.NewNullableWithValue("the AI provider did not answer in time"),
		})
		f := a.matches(token, "applicationId="+appl.Id).Items[0]
		assert.Equal(t, api.MatchStatusFailed, f.Status)
		assert.Equal(t, "the AI provider did not answer in time", f.Error.MustGet())
		assert.True(t, f.Score.IsNull())
		assert.True(t, f.Recommendation.IsNull())
		assert.Empty(t, f.Dimensions)

		q = a.scoreFit(token, appl.Id)
		aiScores(t, a, aiConn, q.Id, scoredWith(claim("Ten years as CFO", api.MatchVerdictMet, "Senior Accountant")))
		f = a.matches(token, "applicationId="+appl.Id).Items[0]
		assert.Equal(t, api.MatchStatusFailed, f.Status, "no requirement quotes the job: no score")
		assert.Contains(t, f.Error.MustGet(), "could be found in the job's text")
	})

	t.Run("the shared monthly allowance", func(t *testing.T) {
		q := a.scoreFit(token, appl.Id)
		big := scoredWith(claim("At least 5 years in audit", api.MatchVerdictPartly, "Auditor at KPMG"))
		big.InputTokens = 20000
		aiScores(t, a, aiConn, q.Id, big)
		res := a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": appl.Id})
		assert.Equal(t, http.StatusTooManyRequests, res.Status)
		assert.Equal(t, api.ErrorCodeMATCHINGLIMITREACHED, res.Error().Error.Code)
		res = a.ask(token, map[string]any{"subjectType": "candidate", "subjectId": c.Id, "kind": "summary"})
		assert.Equal(t, api.ErrorCodeASSISTANTLIMITREACHED, res.Error().Error.Code, "matching used the assistant's allowance too")
	})
}

func TestMatchingIsOffUntilAProviderIsSetUp(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk", "description": "Filing."})
	appl := a.addToJob(token, job.Id, a.createCandidate(token, map[string]any{"firstName": "Ali"}).Id)

	assert.False(t, a.matches(token, "jobId="+job.Id).Enabled)
	res := a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": appl.Id})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMATCHINGOFF, res.Error().Error.Code)
	res = a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": job.Id})
	assert.Equal(t, api.ErrorCodeMATCHINGOFF, res.Error().Error.Code)
}

func TestMatchingNeedsACVAndAJobToCompare(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	ctx := context.Background()
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	company := a.createCompany(token, map[string]any{"name": "Maju Bhd"})
	bare := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk"})
	described := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk", "description": "Filing."})
	withText := a.createCandidate(token, map[string]any{"firstName": "Ali"})
	withCV(t, a, aiConn, token, withText.Id, "Clerk, 2020-2024")
	noCV := a.createCandidate(token, map[string]any{"firstName": "Abu"})

	res := a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": a.addToJob(token, bare.Id, withText.Id).Id})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeMATCHNEEDSTEXT, res.Error().Error.Code)
	assert.Contains(t, res.Error().Error.Message, "no description or ad text")
	res = a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": bare.Id})
	assert.Equal(t, api.ErrorCodeMATCHNEEDSTEXT, res.Error().Error.Code)

	res = a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": a.addToJob(token, described.Id, noCV.Id).Id})
	assert.Equal(t, api.ErrorCodeMATCHNEEDSTEXT, res.Error().Error.Code)
	assert.Contains(t, res.Error().Error.Message, "no CV with readable text")

	res = a.request(http.MethodPost, "/v1/matches", token, map[string]any{"applicationId": "app_01jbz6w0m0k8q9x2r3s4t5v6w7"})
	assert.Equal(t, http.StatusNotFound, res.Status)
	res = a.request(http.MethodPost, "/v1/matches/for-job", token, map[string]any{"jobId": "job_01jbz6w0m0k8q9x2r3s4t5v6w7"})
	assert.Equal(t, http.StatusNotFound, res.Status)
	assert.Equal(t, []string{"jobId"}, fieldNames(t, a.request(http.MethodGet, "/v1/matches", token, nil).Error()), "give a job or an application")
	assert.Equal(t, []string{"jobId"}, fieldNames(t, a.request(http.MethodGet, "/v1/matches?jobId=x&applicationId=y", token, nil).Error()))
}
