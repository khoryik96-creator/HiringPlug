package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestAssistantHistoryRespectsCurrentJobPermission(t *testing.T) {
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	recruiter := tenant.Users[api.RoleRecruiter].Token
	c := a.createCompany(owner, map[string]any{"name": "Synthetic Assistant Client"})
	j := a.createJob(owner, map[string]any{"companyId": c.Id, "title": "Synthetic private job"})
	ask := map[string]any{"subjectType": "job", "subjectId": j.Id, "kind": "question", "question": "Summarize this job."}
	res := a.ask(recruiter, ask)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.AssistantAnswer
	res.JSON(&queued)
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	aiAnswers(t, a, conn, queued.Id, api.AssistantAnsweredJob{
		Answer: nullable.NewNullableWithValue("Synthetic prior job details."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("fixture"), InputTokens: 1, OutputTokens: 1,
	})
	a.setRole(owner, "recruiter", map[string]any{
		"version": 0, "permissions": without(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionJobsRead),
		"candidateScope": "all", "jobCandidates": true,
	}, http.StatusOK)
	require.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/jobs/"+j.Id, recruiter, nil).Status)
	require.Equal(t, http.StatusNotFound, a.ask(recruiter, ask).Status)
	answers := a.answers(recruiter, "job", j.Id)
	require.Empty(t, answers)
}

func TestAssistantAllowanceIncludesAllAgencyRecords(t *testing.T) {
	a := newTestApp(t, assistantOn)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	recruiter := tenant.Users[api.RoleRecruiter].Token
	private := a.createCandidate(owner, map[string]any{"firstName": "Synthetic Owner Record"})
	own := a.createCandidate(recruiter, map[string]any{"firstName": "Synthetic Recruiter Record"})
	res := a.ask(owner, map[string]any{"subjectType": "candidate", "subjectId": private.Id, "kind": "summary"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.AssistantAnswer
	res.JSON(&queued)
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	aiAnswers(t, a, conn, queued.Id, api.AssistantAnsweredJob{
		Answer: nullable.NewNullableWithValue("Synthetic answer."), Error: nullable.NewNullNullable[string](),
		Model: nullable.NewNullableWithValue("fixture"), InputTokens: 6000, OutputTokens: 1,
	})
	a.setRole(owner, "recruiter", map[string]any{
		"version": 0, "permissions": authz.PermissionsForRole(api.RoleRecruiter),
		"candidateScope": "own", "jobCandidates": false,
	}, http.StatusOK)
	var status api.AssistantStatus
	a.request(http.MethodGet, "/v1/assistant", owner, nil).JSON(&status)
	require.GreaterOrEqual(t, status.UsedTokens, status.MonthlyTokens)
	a.request(http.MethodGet, "/v1/assistant", recruiter, nil).JSON(&status)
	require.Equal(t, int64(6001), status.UsedTokens)
	res = a.ask(recruiter, map[string]any{"subjectType": "candidate", "subjectId": own.Id, "kind": "summary"})
	require.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
	company := a.createCompany(owner, map[string]any{"name": "Budget Client"})
	job := a.createJob(owner, map[string]any{"companyId": company.Id, "title": "Analyst"})
	appl := a.addToJob(recruiter, job.Id, own.Id)
	res = a.request(http.MethodPost, "/v1/matches", recruiter, map[string]any{"applicationId": appl.Id})
	require.Equal(t, http.StatusTooManyRequests, res.Status, "%s", res.Body)
}
