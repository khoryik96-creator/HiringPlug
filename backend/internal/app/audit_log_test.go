package app_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) auditLog(token, query string) api.AuditLogPage {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/audit-log"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var page api.AuditLogPage
	res.JSON(&page)
	return page
}

func auditActions(page api.AuditLogPage) []string {
	out := make([]string, len(page.Items))
	for i, e := range page.Items {
		out[i] = e.Action
	}
	return out
}

func auditEntryOf(t *testing.T, page api.AuditLogPage, action, entityID string) api.AuditEntry {
	t.Helper()
	i := slices.IndexFunc(page.Items, func(e api.AuditEntry) bool { return e.Action == action && e.EntityId == entityID })
	require.GreaterOrEqual(t, i, 0, "no %s for %s in %v", action, entityID, auditActions(page))
	return page.Items[i]
}

// logged adds an audit entry as it was written at a time, by an actor (the
// system and API keys cannot be made to act at a chosen time).
func auditLogged(t *testing.T, tenantID, actor, action, entityType, entityID string, at time.Time) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO audit_log (tenant_id, actor_id, action, entity_type, entity_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, tenantID, actor, action, entityType, entityID, at)
	require.NoError(t, err)
}

func TestTheAuditLogNamesWhoDidWhatToWhich(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]

	company := a.createCompany(recruiter.Token, map[string]any{"name": "Maju Bhd", "website": "https://maju.example"})
	contact := a.createContact(recruiter.Token, map[string]any{"companyId": company.Id, "firstName": "Aminah", "lastName": "Yusof"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	cand := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	res := a.request(http.MethodPut, "/v1/candidates/"+cand.Id+"/compensation", recruiter.Token, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "9100", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	// Another company's changes are never in this one's log.
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.createCompany(other.Users[api.RoleOwner].Token, map[string]any{"name": "Other Sdn Bhd"})

	page := a.auditLog(owner.Token, "")
	require.NotEmpty(t, page.Items)
	for i := 1; i < len(page.Items); i++ {
		assert.False(t, page.Items[i].At.After(page.Items[i-1].At), "newest first")
	}
	created := auditEntryOf(t, page, "company.created", company.Id)
	assert.Equal(t, recruiter.ID, created.ActorId)
	assert.Equal(t, api.AuditActorKindUser, created.ActorKind)
	assert.Equal(t, "Test recruiter", created.ActorName)
	assert.Equal(t, "Maju Bhd", created.EntityName.MustGet())
	assert.Empty(t, created.Changes, "webhooks copy these: a new record's fields stay out")
	assert.Equal(t, "Aminah Yusof", auditEntryOf(t, page, "contact.created", contact.Id).EntityName.MustGet())
	assert.Equal(t, "Accountant", auditEntryOf(t, page, "job.created", job.Id).EntityName.MustGet())
	assert.Equal(t, "Siti Aisyah", auditEntryOf(t, page, "candidate.created", cand.Id).EntityName.MustGet())
	for _, e := range page.Items {
		if name, err := e.EntityName.Get(); err == nil {
			assert.NotContains(t, name, "Other", "another company's record")
		}
	}
	raw := a.request(http.MethodGet, "/v1/audit-log?limit=200", owner.Token, nil).Body
	assert.NotContains(t, string(raw), "9100", "a salary is never in the log")

	// Filters.
	assert.Equal(t, []string{"company.created"}, auditActions(a.auditLog(owner.Token, "?action=company.created")))
	byKind := auditActions(a.auditLog(owner.Token, "?action=candidate"))
	assert.Contains(t, byKind, "candidate.created")
	for _, act := range byKind {
		assert.True(t, strings.HasPrefix(act, "candidate."), act)
	}
	about := a.auditLog(owner.Token, "?entityType=company&entityId="+company.Id)
	require.NotEmpty(t, about.Items)
	for _, e := range about.Items {
		assert.Equal(t, company.Id, e.EntityId)
	}
	assert.Empty(t, a.auditLog(owner.Token, "?actor="+owner.ID).Items, "the owner did nothing")
	assert.Len(t, a.auditLog(owner.Token, "?actor="+recruiter.ID).Items, len(page.Items))
	for _, bad := range []string{"?action=Candidate", "?action=a.b.c", "?from=yesterday", "?limit=201", "?cursor=nope"} {
		assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/audit-log"+bad, owner.Token, nil).Status, bad)
	}

	// Pages: the cursor goes on where the last page stopped, nothing twice.
	var seen []string
	query := "?limit=2"
	for {
		p := a.auditLog(owner.Token, query)
		for _, e := range p.Items {
			assert.NotContains(t, seen, e.Id)
			seen = append(seen, e.Id)
		}
		if p.NextCursor.IsNull() {
			break
		}
		query = "?limit=2&cursor=" + p.NextCursor.MustGet()
	}
	assert.Len(t, seen, len(page.Items))
}

func TestTheAuditLogNamesTheSystemAndAPIKeys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	res := a.request(http.MethodPost, "/v1/api-keys", owner, map[string]any{"name": "Payroll sync", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var key api.ApiKeyCreated
	res.JSON(&key)
	viaKey := a.createCandidate(key.Key, map[string]any{"firstName": "Keyed"})
	auditLogged(t, tenant.ID, "system", "candidate.updated", "candidate", viaKey.Id, time.Now())
	auditLogged(t, tenant.ID, "akey_01jbz6w0m0k8q9x2r3s4t5v6w7", "candidate.updated", "candidate", viaKey.Id, time.Now())

	page := a.auditLog(owner, "?entityType=candidate&entityId="+viaKey.Id)
	byActor := map[string]api.AuditEntry{}
	for _, e := range page.Items {
		byActor[e.ActorId] = e
	}
	require.Len(t, byActor, 3)
	assert.Equal(t, "Automatic", byActor["system"].ActorName)
	assert.Equal(t, api.AuditActorKindSystem, byActor["system"].ActorKind)
	assert.Equal(t, "Payroll sync", byActor[key.ApiKey.Id].ActorName)
	assert.Equal(t, api.AuditActorKindApiKey, byActor[key.ApiKey.Id].ActorKind)
	assert.Equal(t, "API key", byActor["akey_01jbz6w0m0k8q9x2r3s4t5v6w7"].ActorName, "a key that cannot be found")
	assert.Len(t, a.auditLog(owner, "?actor=system&entityId="+viaKey.Id).Items, 1)
}

func TestTheAuditLogDaysAreTheAgencys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t) // Asia/Kuala_Lumpur, UTC+8
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	// 23:30 on 1 March and 00:30 on 2 March in Kuala Lumpur.
	auditLogged(t, tenant.ID, "system", "job.updated", "job", "job_late", time.Date(2026, 3, 1, 15, 30, 0, 0, time.UTC))
	auditLogged(t, tenant.ID, "system", "job.updated", "job", "job_early", time.Date(2026, 3, 1, 16, 30, 0, 0, time.UTC))

	ids := func(q string) []string {
		var out []string
		for _, e := range a.auditLog(owner, q).Items {
			out = append(out, e.EntityId)
		}
		return out
	}
	assert.Equal(t, []string{"job_late"}, ids("?from=2026-03-01&to=2026-03-01"))
	assert.Equal(t, []string{"job_early"}, ids("?from=2026-03-02&to=2026-03-02"))
	assert.Equal(t, []string{"job_early", "job_late"}, ids("?from=2026-03-01&to=2026-03-02"))
	assert.Equal(t, []string{"job_late"}, ids("?to=2026-03-01"))
}

func TestOnlyOwnersAndAdminsSeeTheAuditLog(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager, api.RoleViewer)
	export := map[string]any{"format": "csv"}
	for role, user := range tenant.Users {
		want := http.StatusForbidden
		if role == api.RoleOwner || role == api.RoleAdmin {
			want = http.StatusOK
		}
		assert.Equal(t, want, a.request(http.MethodGet, "/v1/audit-log", user.Token, nil).Status, role)
		assert.Equal(t, want, a.request(http.MethodPost, "/v1/audit-log/export", user.Token, export).Status, role)
	}
	// A role given the permission sees it.
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionAuditlogRead),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/audit-log", recruiter, nil).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/salary-views", recruiter, nil).Status, "salary views need their own permission")
}

func TestAScopedUserSeesOnlyTheirCandidatesInTheAuditLog(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner, recruiter, sourcer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	mine := a.createCandidate(recruiter, map[string]any{"firstName": "Mine"})
	theirs := a.createCandidate(sourcer, map[string]any{"firstName": "Theirs"})
	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": append(authz.PermissionsForRole(api.RoleRecruiter), api.PermissionAuditlogRead),
		"candidateScope": "own", "jobCandidates": false}, http.StatusOK)

	page := a.auditLog(recruiter, "?entityType=candidate")
	var about []string
	for _, e := range page.Items {
		about = append(about, e.EntityId)
	}
	assert.Contains(t, about, mine.Id)
	assert.NotContains(t, about, theirs.Id, "row-level security: not their candidate")
	assert.Len(t, a.auditLog(owner, "?entityType=candidate").Items, 2, "the owner sees both")
}

func TestTheAuditLogDownloads(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	owner, admin := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin]
	company := a.createCompany(owner.Token, map[string]any{"name": "Maju"})
	renamed := a.request(http.MethodPatch, "/v1/companies/"+company.Id, owner.Token, map[string]any{"version": company.Version, "name": "Maju Bhd"})
	require.Equal(t, http.StatusOK, renamed.Status, "%s", renamed.Body)

	res := a.request(http.MethodPost, "/v1/audit-log/export", admin.Token, map[string]any{"format": "csv", "action": "company.updated"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Contains(t, res.Header.Get("Content-Disposition"), "Audit log")
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(res.Body, []byte("\xef\xbb\xbf")))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"When", "Who", "Action", "Record type", "Record", "Record id", "Changes"}, rows[0])
	assert.Equal(t, []string{"Test owner", "company.updated", "company", "Maju Bhd", company.Id}, rows[1][1:6])
	assert.Contains(t, rows[1][6], `"name":{"from":"Maju","to":"Maju Bhd"}`)

	xlsx := a.request(http.MethodPost, "/v1/audit-log/export", admin.Token, map[string]any{"format": "xlsx"})
	require.Equal(t, http.StatusOK, xlsx.Status)
	assert.Equal(t, "PK", string(xlsx.Body[:2]), "an Excel workbook")

	// The download is in the log itself.
	exp := a.auditLog(owner.Token, "?action=auditlog.exported")
	require.Len(t, exp.Items, 2)
	assert.Equal(t, admin.ID, exp.Items[0].ActorId)
	assert.Equal(t, map[string]any{"list": "audit_log", "count": float64(1), "format": "csv"}, exp.Items[1].Changes)
}
