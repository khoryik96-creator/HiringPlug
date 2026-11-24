package app_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) salaryViews(token, query string) api.SalaryViewPage {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/salary-views"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var page api.SalaryViewPage
	res.JSON(&page)
	return page
}

// seen is what userID was shown, by kind and candidate ("" for a list or
// export), as an owner reads it.
func (a *testApp) seen(owner, userID string) map[string]api.SalaryView {
	a.t.Helper()
	out := map[string]api.SalaryView{}
	for _, v := range a.salaryViews(owner, "?limit=200&user="+userID).Items {
		key := string(v.Kind) + ":" + orEmpty(v.CandidateId)
		_, twice := out[key]
		require.False(a.t, twice, "one line a day for %s", key)
		out[key] = v
	}
	return out
}

// withSalary adds a candidate with a current and expected salary.
func (a *testApp) withSalary(token string, body map[string]any) api.Candidate {
	a.t.Helper()
	c := a.createCandidate(token, body)
	res := a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8517", "currency": "MYR", "period": "month"},
		"expectedSalary": map[string]any{"min": "10913", "currency": "MYR", "period": "month"},
	})
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	return c
}

// recordSeen counts a showing as a module would, in a transaction of p.
func (a *testApp) recordSeen(p authz.Principal, kind string, ids ...string) {
	a.t.Helper()
	require.NoError(a.t, database.InTx(context.Background(), a.deps.Pool, p, func(tx pgx.Tx) error {
		return a.modules.AuditLog.SalarySeen(context.Background(), tx, kind, ids...)
	}))
}

func orEmpty(n nullable.Nullable[string]) string {
	v, _ := n.Get()
	return v
}

func principalOf(t *testing.T, tenantID string, u testutil.User) authz.Principal {
	t.Helper()
	return authz.NewPrincipal(tenantID, u.ID, "", "", u.Role)
}

func TestSalaryViewsCollapseToOneLineADayWithACount(t *testing.T) {
	t.Parallel()
	a := newTestApp(t) // Asia/Kuala_Lumpur, UTC+8
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	other := a.withSalary(recruiter.Token, map[string]any{"firstName": "Ali"})

	for range 3 {
		require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter.Token, nil).Status)
	}
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+other.Id, recruiter.Token, nil).Status)
	for range 2 {
		require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates?includeSalary=true", recruiter.Token, nil).Status)
	}
	res := a.request(http.MethodPost, "/v1/candidates/search", recruiter.Token, map[string]any{"q": "Siti", "includeSalary": true})
	require.Equal(t, http.StatusOK, res.Status)

	seen := a.seen(owner, recruiter.ID)
	require.Len(t, seen, 3)
	page := seen["candidate:"+c.Id]
	assert.Equal(t, 3, page.Count, "the page three times: one line, counted")
	assert.Equal(t, 1, page.Candidates)
	assert.Equal(t, "Siti Aisyah", page.CandidateName.MustGet())
	assert.Equal(t, "Test recruiter", page.UserName)
	assert.Equal(t, recruiter.ID, page.UserId)
	assert.False(t, page.LastAt.Before(page.FirstAt))
	assert.Equal(t, 1, seen["candidate:"+other.Id].Count)
	list := seen["candidate_list:"]
	assert.Equal(t, 3, list.Count, "two list pages and a search")
	assert.Equal(t, 2, list.Candidates, "two different candidates' salaries that day")
	assert.True(t, list.CandidateId.IsNull())

	// The candidate filter finds the lists that showed them too.
	about := a.salaryViews(owner, "?candidateId="+other.Id)
	kinds := []api.SalaryViewKind{}
	for _, v := range about.Items {
		kinds = append(kinds, v.Kind)
	}
	assert.ElementsMatch(t, []api.SalaryViewKind{api.SalaryViewKindCandidate, api.SalaryViewKindCandidateList}, kinds)
	assert.Len(t, a.salaryViews(owner, "?kind=candidate").Items, 2)
	assert.Len(t, a.salaryViews(owner, "?kind=candidate_list&user="+recruiter.ID).Items, 1)

	// Pages, newest day first.
	first := a.salaryViews(owner, "?limit=2")
	require.Len(t, first.Items, 2)
	rest := a.salaryViews(owner, "?limit=2&cursor="+first.NextCursor.MustGet())
	require.Len(t, rest.Items, 1)
	assert.True(t, rest.NextCursor.IsNull())
	assert.NotContains(t, []string{first.Items[0].Id, first.Items[1].Id}, rest.Items[0].Id)
}

func TestSalaryViewDaysAreTheAgencys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t) // Asia/Kuala_Lumpur, UTC+8
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	p := principalOf(t, tenant.ID, recruiter)
	c := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti"})

	at := func(utc time.Time) {
		a.modules.AuditLog.UseClock(func() time.Time { return utc })
		a.recordSeen(p, salaryview.Candidate, c.Id)
	}
	at(time.Date(2026, 3, 1, 15, 58, 0, 0, time.UTC)) // 23:58 on 1 March in Kuala Lumpur
	at(time.Date(2026, 3, 1, 15, 59, 0, 0, time.UTC)) // 23:59
	at(time.Date(2026, 3, 1, 16, 1, 0, 0, time.UTC))  // 00:01 on 2 March

	days := map[string]api.SalaryView{}
	for _, v := range a.salaryViews(owner, "?user="+recruiter.ID).Items {
		days[v.Day.String()] = v
	}
	require.Len(t, days, 2)
	assert.Equal(t, 2, days["2026-03-01"].Count)
	assert.Equal(t, time.Date(2026, 3, 1, 15, 58, 0, 0, time.UTC), days["2026-03-01"].FirstAt.UTC())
	assert.Equal(t, time.Date(2026, 3, 1, 15, 59, 0, 0, time.UTC), days["2026-03-01"].LastAt.UTC())
	assert.Equal(t, 1, days["2026-03-02"].Count)
	assert.Len(t, a.salaryViews(owner, "?from=2026-03-02").Items, 1)
	assert.Len(t, a.salaryViews(owner, "?to=2026-03-01").Items, 1)
	assert.Len(t, a.salaryViews(owner, "?from=2026-03-01&to=2026-03-02").Items, 2)
}

func TestTheSystemAndTheAINeverCountAsSeeingASalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti"})

	// The system's own work, and a transaction with no user.
	a.recordSeen(authz.System(tenant.ID), salaryview.Candidate, c.Id)
	a.recordSeen(authz.TenantOnly(tenant.ID), salaryview.CandidateList, c.Id)
	// Other modules reading a candidate as the user (formatted CVs, emails,
	// the assistant, the AI's candidate search) count only where they show
	// the salary to a person, never when they read it.
	ctx := context.Background()
	p := principalOf(t, tenant.ID, recruiter)
	require.NoError(t, database.InTx(ctx, a.deps.Pool, p, func(tx pgx.Tx) error {
		view, err := a.modules.Candidates.ViewIn(ctx, tx, p, c.Id)
		if err != nil {
			return err
		}
		require.NotNil(t, view.Compensation, "read with the salary")
		page, err := a.modules.Candidates.SearchIn(ctx, tx, p, candidates.SearchFilter{IncludeSalary: true, AllSkills: true}, 10)
		require.NotEmpty(t, page.Items)
		return err
	}))

	assert.Empty(t, a.salaryViews(owner, "").Items)
	var n int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM salary_view WHERE tenant_id = $1`, tenant.ID).Scan(&n))
	assert.Zero(t, n)
}

func TestOnlyOwnersAndAdminsSeeWhoWasShownASalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti"})
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter.Token, nil).Status)
	export := map[string]any{"format": "csv"}
	for role, user := range tenant.Users {
		want := http.StatusForbidden
		if role == api.RoleOwner || role == api.RoleAdmin {
			want = http.StatusOK
		}
		assert.Equal(t, want, a.request(http.MethodGet, "/v1/salary-views", user.Token, nil).Status, role)
		assert.Equal(t, want, a.request(http.MethodPost, "/v1/salary-views/export", user.Token, export).Status, role)
	}
	assert.Len(t, a.salaryViews(tenant.Users[api.RoleAdmin].Token, "").Items, 1)

	// A role given the permission sees the lines; the database also refuses
	// them to a transaction without it.
	owner := tenant.Users[api.RoleOwner].Token
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionAuditlogSalaryViewsRead),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	assert.Len(t, a.salaryViews(recruiter.Token, "").Items, 1)
	var n int
	require.NoError(t, database.InTx(context.Background(), a.deps.Pool, principalOf(t, tenant.ID, tenant.Users[api.RoleViewer]), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM salary_view`).Scan(&n)
	}))
	assert.Zero(t, n, "row-level security: the permission is needed")
	require.Error(t, database.InTx(context.Background(), a.deps.Pool, principalOf(t, tenant.ID, tenant.Users[api.RoleOwner]), func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO salary_view (tenant_id, user_id, kind, candidate_id, day, first_at, last_at)
			VALUES (app_current_tenant(), 'usr_x', 'candidate', $1, current_date, now(), now())`, c.Id)
		return err
	}), "only counted through app_record_salary_view")
}

func TestSalaryViewsStayInTheirCompanyAndScope(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner, recruiter, sourcer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	mine := a.withSalary(recruiter.Token, map[string]any{"firstName": "Mine"})
	theirs := a.createCandidate(sourcer.Token, map[string]any{"firstName": "Theirs"})
	res := a.request(http.MethodPut, "/v1/candidates/"+theirs.Id+"/compensation", owner, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "7000", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	for _, c := range []api.Candidate{mine, theirs} {
		require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter.Token, nil).Status)
	}
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates?includeSalary=true", recruiter.Token, nil).Status)
	// The owner's search showed only the sourcer's candidate.
	res = a.request(http.MethodPost, "/v1/candidates/search", owner, map[string]any{"q": "Theirs", "includeSalary": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	// Another company's views are never shown.
	other := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	elsewhere := a.withSalary(other.Users[api.RoleRecruiter].Token, map[string]any{"firstName": "Elsewhere"})
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+elsewhere.Id, other.Users[api.RoleRecruiter].Token, nil).Status)
	assert.Len(t, a.salaryViews(other.Users[api.RoleOwner].Token, "").Items, 1)
	assert.Len(t, a.salaryViews(owner, "").Items, 4)
	assert.Empty(t, a.salaryViews(owner, "?candidateId="+elsewhere.Id).Items)
	assert.Empty(t, a.salaryViews(owner, "?user="+other.Users[api.RoleRecruiter].ID).Items)

	assert.Len(t, a.salaryViews(owner, "?candidateId="+theirs.Id).Items, 3, "their page, the recruiter's list, the owner's search")

	// A user who sees only their own candidates sees only lines about them,
	// and the lists that showed one of them, counting only those: never that
	// a list showed someone else's candidate.
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionAuditlogSalaryViewsRead),
		"candidateScope": "own", "jobCandidates": false}, http.StatusOK)
	scoped := a.salaryViews(recruiter.Token, "").Items
	require.Len(t, scoped, 2)
	var about []string
	for _, v := range scoped {
		about = append(about, orEmpty(v.CandidateId))
		assert.Equal(t, recruiter.ID, v.UserId, "not the owner's search of someone else's candidate")
		assert.Equal(t, 1, v.Candidates, "their own candidate only")
	}
	assert.ElementsMatch(t, []string{mine.Id, ""}, about)
	assert.Empty(t, a.salaryViews(recruiter.Token, "?candidateId="+theirs.Id).Items, "not even through the list")
	assert.Len(t, a.salaryViews(recruiter.Token, "?candidateId="+mine.Id).Items, 2)
	file := a.request(http.MethodPost, "/v1/salary-views/export", recruiter.Token, map[string]any{"format": "csv"})
	require.Equal(t, http.StatusOK, file.Status, "%s", file.Body)
	assert.NotContains(t, string(file.Body), "Test owner", "nor in the download")
}

func TestSalaryViewsDownload(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	for range 2 {
		require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter.Token, nil).Status)
	}
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates?includeSalary=true", recruiter.Token, nil).Status)

	res := a.request(http.MethodPost, "/v1/salary-views/export", admin, map[string]any{"format": "csv", "kind": "candidate", "user": recruiter.ID})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Contains(t, res.Header.Get("Content-Disposition"), "Salary views")
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(res.Body, []byte("\xef\xbb\xbf")))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2, "the filter: candidate pages only")
	assert.Equal(t, []string{"Day", "Who", "Where", "Candidate", "Candidate id", "Candidates", "Times", "First", "Last"}, rows[0])
	assert.Equal(t, []string{"Test recruiter", "Candidate page", "Siti Aisyah", c.Id, "1", "2"}, rows[1][1:7])
	assert.Equal(t, time.Now().In(kualaLumpur(t)).Format(time.DateOnly), rows[1][0], "the agency's day")

	all := a.request(http.MethodPost, "/v1/salary-views/export", admin, map[string]any{"format": "xlsx"})
	require.Equal(t, http.StatusOK, all.Status)
	assert.Equal(t, "PK", string(all.Body[:2]))
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/salary-views/export", admin, map[string]any{"format": "csv", "kind": "salary"}).Status)

	// Each download is in the audit log, with no amount.
	exp := a.auditLog(owner.Token, "?action=auditlog.exported")
	require.Len(t, exp.Items, 2)
	assert.Equal(t, map[string]any{"list": "salary_views", "count": float64(1), "format": "csv"}, exp.Items[1].Changes)
	assert.Equal(t, map[string]any{"list": "salary_views", "count": float64(2), "format": "xlsx"}, exp.Items[0].Changes)
}
