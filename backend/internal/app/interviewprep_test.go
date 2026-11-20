package app_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) interviewQuestion(token, jobID string, body map[string]any) api.InterviewQuestion {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/jobs/"+jobID+"/interview-questions", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var q api.InterviewQuestion
	res.JSON(&q)
	return q
}

func (a *testApp) interviewQuestions(token, jobID string, archived bool) []api.InterviewQuestion {
	a.t.Helper()
	path := "/v1/jobs/" + jobID + "/interview-questions"
	if archived {
		path += "?archived=true"
	}
	res := a.request(http.MethodGet, path, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.InterviewQuestionList
	res.JSON(&list)
	return list.Items
}

func TestQuestionsAskedAtAJobsInterviewsAreRecordedAndArchived(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer"})

	q := a.interviewQuestion(token, job.Id, map[string]any{"question": "  How would you   design a late-arriving data fix? ",
		"kind": "technical", "round": "technical", "askedOn": "2026-11-18", "source": "Candidate after round 2", "notes": "Panel pushed on idempotency"})
	assert.Equal(t, "How would you design a late-arriving data fix?", q.Question)
	assert.Equal(t, api.InterviewRoundTechnical, q.Round.MustGet())
	assert.Equal(t, "2026-11-18", q.AskedOn.MustGet().String())
	assert.False(t, q.Archived)
	a.interviewQuestion(token, job.Id, map[string]any{"question": "Tell me about a time you disagreed with your lead.", "kind": "behavioural"})

	res := a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-questions", token, map[string]any{"question": " ", "kind": "other"})
	assert.Equal(t, []string{"question"}, fieldNames(t, res.Error()))
	viewer := tenant.Users[api.RoleViewer].Token
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-questions", viewer, nil).Status,
		"interview insights are the agency's: not for viewers")

	// Archived: out of the list (and out of the AI's prep), kept and brought back.
	res = a.request(http.MethodPatch, "/v1/interview-questions/"+q.Id, token, map[string]any{"version": q.Version, "archived": true, "notes": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var archived api.InterviewQuestion
	res.JSON(&archived)
	assert.True(t, archived.Archived)
	assert.True(t, archived.Notes.IsNull(), "a note taken out")
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPatch, "/v1/interview-questions/"+q.Id, token,
		map[string]any{"version": q.Version, "archived": false}).Status, "an old version is refused")
	assert.Len(t, a.interviewQuestions(token, job.Id, false), 1)
	assert.Len(t, a.interviewQuestions(token, job.Id, true), 1)
	res = a.request(http.MethodPatch, "/v1/interview-questions/"+q.Id, token, map[string]any{"version": archived.Version, "archived": false})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Len(t, a.interviewQuestions(token, job.Id, false), 2)

	// Recorded by mistake: deleted. Another company sees none of it.
	assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/interview-questions/"+q.Id, token, nil).Status)
	assert.Len(t, a.interviewQuestions(token, job.Id, false), 1)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-questions", other.Users[api.RoleRecruiter].Token, nil).Status)
}

func prepWritten(id string, content *api.InterviewPrepContent, reason string) api.InterviewPrepWrittenJob {
	out := api.InterviewPrepWrittenJob{PrepId: id, Content: nullable.NewNullNullable[api.InterviewPrepContent](), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("test-model"), InputTokens: 3000, OutputTokens: 1500}
	if content != nil {
		out.Content.Set(*content)
	}
	if reason != "" {
		out.Error.Set(reason)
	}
	return out
}

func TestTheAIWritesInterviewPrepFromTheJobTheClientAndRecordedQuestions(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad", "industry": "Financial Services", "about": "A digital bank in Malaysia."})
	bare := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk"})
	res := a.request(http.MethodPost, "/v1/jobs/"+bare.Id+"/interview-preps", token, map[string]any{})
	assert.Equal(t, api.ErrorCodeINTERVIEWPREPTOOLITTLE, res.Error().Error.Code, "nothing to prepare from")

	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer", "location": "Kuala Lumpur",
		"description": "Build and run the bank's data platform on Kafka and Spark.\nSalary: RM 12,000 - 15,000 monthly"})
	a.interviewQuestion(token, job.Id, map[string]any{"question": "What if the nightly batch is late on month end?", "kind": "situational", "round": "hiring_manager"})
	gone := a.interviewQuestion(token, job.Id, map[string]any{"question": "An archived question", "kind": "other"})
	a.request(http.MethodPatch, "/v1/interview-questions/"+gone.Id, token, map[string]any{"version": gone.Version, "archived": true})

	res = a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{"focus": " Strong on Spark, new to banking "})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var prep api.InterviewPrep
	res.JSON(&prep)
	assert.Equal(t, api.InterviewPrepStatusQueued, prep.Status)
	assert.Equal(t, 1, prep.QuestionCount, "archived questions are left out")

	claimed := claimAIJobWhere(t, aiConn, "prepId", prep.Id)
	require.Equal(t, string(api.JobKindInterviewPrepWrite), claimed.Kind)
	var asked api.InterviewPrepWriteJob
	require.NoError(t, json.Unmarshal(claimed.Payload, &asked))
	assert.Equal(t, "Data Engineer", asked.JobTitle)
	assert.Equal(t, "Maju Bank Berhad", asked.CompanyName)
	assert.Equal(t, "Financial Services", asked.CompanyIndustry.MustGet())
	assert.Equal(t, "A digital bank in Malaysia.", asked.CompanyAbout.MustGet())
	assert.Contains(t, asked.JobDescription, "Kafka and Spark")
	assert.NotContains(t, asked.JobDescription, "12,000", "salary never goes to the AI")
	assert.NotContains(t, string(claimed.Payload), "15,000")
	require.Len(t, asked.Questions, 1)
	assert.Equal(t, api.InterviewQuestionKindSituational, asked.Questions[0].Kind)
	assert.Equal(t, "Strong on Spark, new to banking", asked.Focus.MustGet())

	content := api.InterviewPrepContent{Summary: "Expect depth on streaming reliability.", FocusAreas: []string{"Kafka", "Data quality"},
		SoftSkills: []api.InterviewPrepItem{{Question: "What if the nightly batch is late on month end?", Kind: "What-if",
			WhyAsked: "Tests calm prioritisation.", Approach: []string{"Say who is affected first"}, Phrasing: []string{"\"My first call would be…\""},
			TradeOffs: []string{"Speed vs completeness"}, Pitfalls: []string{"Jumping to the fix"}, FromInterviews: true}},
		Technical: []api.InterviewPrepItem{}, QuestionsToAsk: []string{"How is data quality owned today?"}}
	answerAIJob(t, a, aiConn, claimed, api.JobKindInterviewPrepWritten, prepWritten(prep.Id, &content, ""))

	var list api.InterviewPrepList
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-preps", token, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	done := list.Items[0]
	assert.Equal(t, api.InterviewPrepStatusDone, done.Status)
	assert.Equal(t, "test-model", done.Model.MustGet())
	got := done.Content.MustGet()
	assert.Equal(t, []string{"\"My first call would be…\""}, got.SoftSkills[0].Phrasing)
	assert.True(t, got.SoftSkills[0].FromInterviews)

	// Archived: kept apart, and brought back.
	res = a.request(http.MethodPatch, "/v1/interview-preps/"+done.Id, token, map[string]any{"archived": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-preps", token, nil).JSON(&list)
	assert.Empty(t, list.Items)
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-preps?archived=true", token, nil).JSON(&list)
	assert.Len(t, list.Items, 1)

	// The AI could not write it: failed, with why.
	res = a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	res.JSON(&prep)
	claimed = claimAIJobWhere(t, aiConn, "prepId", prep.Id)
	answerAIJob(t, a, aiConn, claimed, api.JobKindInterviewPrepWritten, prepWritten(prep.Id, nil, "the AI provider is down"))
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/interview-preps", token, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, api.InterviewPrepStatusFailed, list.Items[0].Status)
	assert.Equal(t, "the AI provider is down", list.Items[0].Error.MustGet())
}

func TestInterviewPrepNeedsTheAI(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer", "description": "Build and run the bank's data platform on Kafka and Spark."})
	res := a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{})
	assert.Equal(t, api.ErrorCodeINTERVIEWPREPAIOFF, res.Error().Error.Code)
}

func TestInterviewPrepNeverSendsPayAndCoversTheNewestQuestions(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, matchingOn)
	aiConn := aiConnFor(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maju Bank Berhad", "about": "A digital bank paying RM 9,000 to juniors."})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer",
		"description": "Build and run the bank's data platform on Kafka and Spark.\nPackage: 12,000 - 15,000 monthly"})
	for i := range interviewprep.MaxRecordedQuestions {
		a.interviewQuestion(token, job.Id, map[string]any{"question": "Technical question " + string(rune('A'+i)), "kind": "technical"})
	}
	a.interviewQuestion(token, job.Id, map[string]any{"question": "What do you expect? (she said RM 14k)", "kind": "culture",
		"notes": "Client budget is 15,000 a month"})

	res := a.request(http.MethodPost, "/v1/jobs/"+job.Id+"/interview-preps", token, map[string]any{"focus": "Expects RM 13,500, strong on Spark"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var prep api.InterviewPrep
	res.JSON(&prep)
	assert.Equal(t, interviewprep.MaxRecordedQuestions, prep.QuestionCount, "the newest questions, as many as one answer covers")

	claimed := claimAIJobWhere(t, aiConn, "prepId", prep.Id)
	payload := string(claimed.Payload)
	for _, pay := range []string{"9,000", "12,000", "15,000", "14k", "13,500"} {
		assert.NotContains(t, payload, pay, "pay never goes to the AI")
	}
	assert.Contains(t, payload, "strong on Spark")
	assert.Contains(t, payload, "What do you expect? (she said", "the question is kept, its pay taken out")
	assert.NotContains(t, payload, "Technical question A", "the oldest question is past the newest twelve")
}
