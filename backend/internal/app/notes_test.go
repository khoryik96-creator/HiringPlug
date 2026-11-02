package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestNotesOnRecordsWithSensitiveOnesProtected(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
	notes := "/v1/notes?subjectType=candidate&subjectId=" + siti.Id

	res := a.request(http.MethodPost, "/v1/notes", recruiter, map[string]any{
		"subjectType": "candidate", "subjectId": siti.Id, "kind": "screening", "body": "  Strong IFRS knowledge. ",
	})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var screening api.Note
	res.JSON(&screening)
	assert.Equal(t, "Strong IFRS knowledge.", screening.Body)
	assert.Equal(t, api.NoteKindScreening, screening.Kind)
	assert.NotEmpty(t, screening.AuthorName)

	res = a.request(http.MethodPost, "/v1/notes", recruiter, map[string]any{
		"subjectType": "candidate", "subjectId": siti.Id, "kind": "call", "body": "Wants 15k.", "sensitive": true,
	})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)

	// A hiring manager may write interview notes, but not sensitive ones.
	manager := tenant.Users[api.RoleHiringManager].Token
	res = a.request(http.MethodPost, "/v1/notes", manager, map[string]any{"subjectType": "candidate", "subjectId": siti.Id, "kind": "interview", "body": "Good fit."})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res = a.request(http.MethodPost, "/v1/notes", manager, map[string]any{"subjectType": "candidate", "subjectId": siti.Id, "body": "x", "sensitive": true})
	require.Equal(t, http.StatusBadRequest, res.Status)
	assert.Equal(t, []string{"sensitive"}, fieldNames(t, res.Error()))

	var list api.NoteList
	a.request(http.MethodGet, notes, recruiter, nil).JSON(&list)
	assert.Len(t, list.Items, 3)
	assert.Zero(t, list.HiddenSensitive)
	a.request(http.MethodGet, notes, tenant.Users[api.RoleSourcer].Token, nil).JSON(&list)
	assert.Len(t, list.Items, 2, "the sensitive note is left out")
	assert.Equal(t, 1, list.HiddenSensitive, "and the sourcer is told one is hidden")
	assert.Equal(t, "Good fit.", list.Items[0].Body, "newest first")

	// The database refuses sensitive notes to raw SQL without the permission.
	var n int
	require.NoError(t, database.InTx(context.Background(), testutil.Current.App, authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleSourcer), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "SELECT count(*) FROM note WHERE sensitive").Scan(&n)
	}))
	assert.Zero(t, n)

	// Viewers read but do not write; nobody deletes another person's note without notes:manage.
	res = a.request(http.MethodPost, "/v1/notes", tenant.Users[api.RoleViewer].Token, map[string]any{"subjectType": "candidate", "subjectId": siti.Id, "body": "x"})
	assert.Equal(t, http.StatusForbidden, res.Status)
	res = a.request(http.MethodDelete, "/v1/notes/"+screening.Id, manager, nil)
	assert.Equal(t, http.StatusForbidden, res.Status, "not your note")
	res = a.request(http.MethodDelete, "/v1/notes/"+screening.Id, recruiter, nil)
	assert.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)

	res = a.request(http.MethodPost, "/v1/notes", recruiter, map[string]any{"subjectType": "job", "subjectId": "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", "body": "x"})
	require.Equal(t, http.StatusBadRequest, res.Status)
	assert.Equal(t, []string{"subjectId"}, fieldNames(t, res.Error()), "the record must exist")
}

func TestActivityShowsWhoChangedWhat(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Siti", "currentTitle": "Accountant"})
	res := a.request(http.MethodPatch, "/v1/candidates/"+c.Id, token, map[string]any{"version": c.Version, "currentTitle": "Senior Accountant"})
	require.Equal(t, http.StatusOK, res.Status)
	res = a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8000", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	})
	require.Equal(t, http.StatusOK, res.Status)

	var activity api.ActivityList
	res = a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+c.Id, tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&activity)
	require.Len(t, activity.Items, 3)
	assert.Equal(t, "candidate.compensation_updated", activity.Items[0].Action, "newest first")
	assert.NotContains(t, res.Body, "8000", "salary amounts never appear")
	assert.Equal(t, "candidate.updated", activity.Items[1].Action)
	assert.Equal(t, map[string]any{"from": "Accountant", "to": "Senior Accountant"}, activity.Items[1].Changes["currentTitle"])
	assert.NotEqual(t, "System", activity.Items[1].ActorName)

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+c.Id, other.Users[api.RoleOwner].Token, nil).JSON(&activity)
	assert.Empty(t, activity.Items, "another company sees nothing")
}

func TestDashboardSummary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Maybank"})
	auditor := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	a.createJob(token, map[string]any{"companyId": company.Id, "title": "Old role", "status": "closed"})
	siti := a.addToJob(token, auditor.Id, a.createCandidate(token, map[string]any{"firstName": "Siti"}).Id)
	wei := a.addToJob(token, auditor.Id, a.createCandidate(token, map[string]any{"firstName": "Wei"}).Id)
	require.Equal(t, http.StatusOK, a.move(token, wei, map[string]any{"stage": "placed"}).Status)
	// Siti has waited 20 days in "applied".
	require.NoError(t, database.InTx(context.Background(), testutil.Current.App, authz.System(tenant.ID), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), "UPDATE application SET stage_changed_at = now() - interval '20 days' WHERE id = $1", siti.Id)
		return err
	}))

	var sum api.PipelineSummary
	res := a.request(http.MethodGet, "/v1/pipeline/summary", tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&sum)
	assert.Equal(t, 1, sum.OpenJobs, "closed jobs are left out")
	assert.Equal(t, 1, sum.StageCounts.Applied)
	assert.Equal(t, 1, sum.StageCounts.Placed)
	require.Len(t, sum.Jobs, 1)
	assert.Equal(t, "Auditor", sum.Jobs[0].Title)
	assert.Equal(t, "Maybank", sum.Jobs[0].CompanyName)
	require.Len(t, sum.Stuck, 1)
	assert.Equal(t, "Siti", sum.Stuck[0].CandidateName)
	assert.Equal(t, 1, sum.PlacedLast30Days)
	assert.Equal(t, 1, sum.MovedLast7Days)

	a.request(http.MethodGet, "/v1/pipeline/summary?stuckDays=30", token, nil).JSON(&sum)
	assert.Empty(t, sum.Stuck, "20 days is not stuck when the limit is 30")
}
