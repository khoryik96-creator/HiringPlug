package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Every place a salary leaves the server for a person counts it once per
// showing (one line a day, counted); reading without a salary, or a list
// that does not show it, counts nothing. No amount is in either log.
func TestEveryPlaceThatShowsASalaryCountsItOncePerView(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	rt := recruiter.Token
	c := a.withSalary(rt, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com", "currentTitle": "Accountant"})
	bare := a.createCandidate(rt, map[string]any{"firstName": "Bare", "email": "bare@example.com"})
	counted := func(user, kind, candidateID string) int {
		t.Helper()
		return a.seen(owner, user)[kind+":"+candidateID].Count
	}
	ok := func(res *response, want int) *response {
		t.Helper()
		require.Equal(t, want, res.Status, "%s", res.Body)
		return res
	}

	// A candidate's page, with their salary; one without a salary is not counted.
	ok(a.request(http.MethodGet, "/v1/candidates/"+c.Id, rt, nil), http.StatusOK)
	ok(a.request(http.MethodGet, "/v1/candidates/"+bare.Id, rt, nil), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "candidate", c.Id))
	assert.Zero(t, counted(recruiter.ID, "candidate", bare.Id))

	// Lists and searches: only when salaries are asked for and returned.
	ok(a.request(http.MethodGet, "/v1/candidates?q=Siti", rt, nil), http.StatusOK)
	ok(a.request(http.MethodPost, "/v1/candidates/search", rt, map[string]any{"q": "Siti"}), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "candidate_list", ""), "a typeahead gets no salary")
	ok(a.request(http.MethodGet, "/v1/candidates?q=Bare&includeSalary=true", rt, nil), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "candidate_list", ""), "no salary on the page")
	ok(a.request(http.MethodGet, "/v1/candidates?includeSalary=true", rt, nil), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "candidate_list", ""))
	ok(a.request(http.MethodPost, "/v1/candidates/search", rt, map[string]any{"q": "Siti", "includeSalary": true}), http.StatusOK)
	assert.Equal(t, 2, counted(recruiter.ID, "candidate_list", ""))

	// A screening call's salary.
	screened := a.createCandidate(rt, map[string]any{"firstName": "Screened"})
	ok(a.request(http.MethodPost, "/v1/candidates/"+screened.Id+"/screening-calls", rt, map[string]any{
		"version": screened.Version, "compensation": map[string]any{"expectedVersion": 0,
			"currentSalary": map[string]any{"amount": "6211", "currency": "MYR", "period": "month"}, "expectedSalary": nil},
	}), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "screening", screened.Id), "what they just typed in")
	ok(a.request(http.MethodGet, "/v1/candidates/"+screened.Id+"/screening-calls", rt, nil), http.StatusOK)
	ok(a.request(http.MethodGet, "/v1/candidates/"+bare.Id+"/screening-calls", rt, nil), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "screening", screened.Id))
	assert.Zero(t, counted(recruiter.ID, "screening", bare.Id))

	// An email being written: the composer's field values hold the salary.
	preview := map[string]any{"subject": "", "bodyHtml": "", "timeZone": "Asia/Kuala_Lumpur"}
	ok(a.request(http.MethodPost, "/v1/email-previews", rt, with(preview, "candidateId", c.Id)), http.StatusOK)
	ok(a.request(http.MethodPost, "/v1/email-previews", rt, with(preview, "candidateId", bare.Id)), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "email_fields", c.Id))
	assert.Zero(t, counted(recruiter.ID, "email_fields", bare.Id))
	// A draft from a template that fills a salary field; one that fills none is not counted.
	var withPay, plain api.EmailTemplate
	ok(a.request(http.MethodPost, "/v1/email-templates", rt, map[string]any{"name": "Pay", "subject": "[Full name]", "body": "Expecting [Expected salary]"}), http.StatusCreated).JSON(&withPay)
	ok(a.request(http.MethodPost, "/v1/email-templates", rt, map[string]any{"name": "Hello", "subject": "Hi [First name]", "body": "Hello"}), http.StatusCreated).JSON(&plain)
	draft := map[string]any{"candidateId": c.Id, "timeZone": "Asia/Kuala_Lumpur"}
	ok(a.request(http.MethodPost, "/v1/email-drafts", rt, with(draft, "templateId", withPay.Id)), http.StatusOK)
	ok(a.request(http.MethodPost, "/v1/email-drafts", rt, with(draft, "templateId", plain.Id)), http.StatusOK)
	assert.Equal(t, 2, counted(recruiter.ID, "email_fields", c.Id))

	// An email sent with the salary; one without it is not counted.
	send := map[string]any{"candidateId": c.Id, "to": []string{"hr@client.example"}, "subject": "Siti", "timeZone": "Asia/Kuala_Lumpur"}
	ok(a.request(http.MethodPost, "/v1/emails", rt, with(send, "bodyHtml", "<p>Hello</p>")), http.StatusAccepted)
	assert.Zero(t, counted(recruiter.ID, "email_sent", c.Id))
	ok(a.request(http.MethodPost, "/v1/emails", rt, with(send, "bodyHtml", "<p>Expecting [Expected salary]</p>")), http.StatusAccepted)
	assert.Equal(t, 1, counted(recruiter.ID, "email_sent", c.Id))

	// A formatted CV made with the salary, opened, downloaded and attached.
	var cv, noPay api.FormattedCv
	ok(a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", rt, map[string]any{"includeSalary": true}), http.StatusCreated).JSON(&cv)
	assert.Equal(t, 1, counted(recruiter.ID, "cv", c.Id))
	ok(a.request(http.MethodGet, "/v1/documents/"+cv.DocumentId+"/file", rt, nil), http.StatusOK)
	ok(a.request(http.MethodGet, "/v1/documents/"+cv.DocumentId, rt, nil), http.StatusOK)
	assert.Equal(t, 3, counted(recruiter.ID, "cv", c.Id))
	ok(a.request(http.MethodPost, "/v1/emails", rt, with(with(send, "bodyHtml", "<p>CV attached</p>"), "cvs", []map[string]any{{"documentId": cv.DocumentId}})), http.StatusAccepted)
	assert.Equal(t, 4, counted(recruiter.ID, "cv", c.Id))
	ok(a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", rt, map[string]any{}), http.StatusCreated).JSON(&noPay)
	ok(a.request(http.MethodGet, "/v1/documents/"+noPay.DocumentId+"/file", rt, nil), http.StatusOK)
	assert.Equal(t, 4, counted(recruiter.ID, "cv", c.Id), "a CV without the salary")

	// Offers: where the app shows the salary (an application's offers, those waiting for approval).
	company := a.createCompany(rt, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(rt, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(rt, job.Id, c.Id)
	a.offer(http.MethodPost, "/v1/applications/"+app.Id+"/offers", rt, map[string]any{"salary": map[string]any{"amount": "9123", "currency": "MYR", "period": "month"}}, http.StatusCreated)
	assert.Zero(t, counted(recruiter.ID, "offer", c.Id), "what they just typed in")
	ok(a.request(http.MethodGet, "/v1/offers?status=sent", rt, nil), http.StatusOK)
	ok(a.request(http.MethodGet, "/v1/offers", rt, nil), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "offer", c.Id), "lists that do not show it")
	ok(a.request(http.MethodGet, "/v1/offers?applicationId="+app.Id, rt, nil), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "offer", c.Id))

	// Placements: an application's placement.
	job2 := a.createJob(rt, map[string]any{"companyId": company.Id, "title": "Auditor"})
	app2 := a.addToJob(rt, job2.Id, c.Id)
	a.placement(http.MethodPost, "/v1/applications/"+app2.Id+"/placement", rt, map[string]any{
		"startDate": "2026-12-01", "money": map[string]any{"salary": map[string]any{"amount": "7345", "currency": "MYR", "period": "month"}, "feePercent": "20"},
	}, http.StatusCreated)
	ok(a.request(http.MethodGet, "/v1/placements", rt, nil), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "placement", c.Id), "the dashboard does not show it")
	ok(a.request(http.MethodGet, "/v1/placements?applicationId="+app2.Id, rt, nil), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "placement", c.Id))

	// Exports with salaries: one line a day for all of them.
	ok(a.request(http.MethodPost, "/v1/candidates/export", admin.Token, map[string]any{"format": "csv"}), http.StatusOK)
	exp := a.seen(owner, admin.ID)["export:"]
	assert.Equal(t, 1, exp.Count)
	assert.Equal(t, 2, exp.Candidates, "Siti and the screened candidate; not Bare")
	ok(a.request(http.MethodPost, "/v1/placements/export", admin.Token, map[string]any{"format": "csv"}), http.StatusOK)
	assert.Equal(t, 2, a.seen(owner, admin.ID)["export:"].Count)

	// No amount anywhere in either log, nor in their downloads.
	views := ok(a.request(http.MethodGet, "/v1/salary-views?limit=200", owner, nil), http.StatusOK).Body
	log := ok(a.request(http.MethodGet, "/v1/audit-log?limit=200", owner, nil), http.StatusOK).Body
	viewsFile := ok(a.request(http.MethodPost, "/v1/salary-views/export", owner, map[string]any{"format": "csv"}), http.StatusOK).Body
	logFile := ok(a.request(http.MethodPost, "/v1/audit-log/export", owner, map[string]any{"format": "csv"}), http.StatusOK).Body
	var rows string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT coalesce(string_agg(row_to_json(s)::text, ' '), '') FROM salary_view s WHERE tenant_id = $1`, tenant.ID).Scan(&rows))
	for _, amount := range []string{"8517", "8,517", "10913", "10,913", "6211", "6,211", "9123", "9,123", "7345", "7,345"} {
		for name, text := range map[string]string{"salary views": string(views), "audit log": string(log), "salary views file": string(viewsFile),
			"audit log file": string(logFile), "salary_view rows": rows} {
			assert.False(t, strings.Contains(text, amount), "%s holds %s", name, amount)
		}
	}
}

// with is body with one more field.
func with(body map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(body)+1)
	for k, v := range body {
		out[k] = v
	}
	out[key] = value
	return out
}

// An email to a candidate about shortlisted candidates fills its own fields
// with the recipient's details: their salary is the one counted, and a
// shortlisted candidate's salary never fills them (not even when the
// recipient has none).
func TestAShortlistEmailToACandidateCountsTheRecipientsOwnSalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	rt := recruiter.Token
	shortlisted := a.withSalary(rt, map[string]any{"firstName": "Siti", "email": "siti@example.com"})
	recipient := a.createCandidate(rt, map[string]any{"firstName": "Ravi", "email": "ravi@example.com"})
	res := a.request(http.MethodPut, "/v1/candidates/"+recipient.Id+"/compensation", rt, map[string]any{
		"expectedVersion": 0, "currentSalary": nil, "expectedSalary": map[string]any{"min": "12345", "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	noSalary := a.createCandidate(rt, map[string]any{"firstName": "Mei", "email": "mei@example.com"})
	email := func(path string, to api.Candidate) *response {
		body := map[string]any{
			"recipientCandidateId": to.Id, "candidates": []map[string]any{{"candidateId": shortlisted.Id}},
			"subject": "Roles for you", "bodyHtml": "<p>Your expected salary: [Expected salary]</p>", "timeZone": "Asia/Kuala_Lumpur",
		}
		if path == "/v1/emails" {
			body["to"] = []string{string(to.Email.MustGet())}
		}
		return a.request(http.MethodPost, path, rt, body)
	}

	res = email("/v1/email-previews", recipient)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var preview api.EmailPreview
	res.JSON(&preview)
	assert.Contains(t, preview.BodyHtml, "12,345")
	res = email("/v1/emails", recipient)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	seen := a.seen(owner, recruiter.ID)
	assert.Equal(t, 1, seen["email_fields:"+recipient.Id].Count, "the composer showed the recipient's salary")
	assert.Equal(t, 1, seen["email_sent:"+recipient.Id].Count, "the recipient's salary went to them")
	assert.NotContains(t, seen, "email_fields:"+shortlisted.Id)
	assert.NotContains(t, seen, "email_sent:"+shortlisted.Id)

	// A recipient with no salary: the field stays empty, never the shortlisted candidate's.
	res = email("/v1/email-previews", noSalary)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&preview)
	assert.Contains(t, preview.Missing, "[Expected salary]")
	assert.NotContains(t, preview.BodyHtml, "10,913")
	for _, v := range preview.FieldValues {
		assert.NotContains(t, v, "10,913")
	}
	assert.Equal(t, http.StatusBadRequest, email("/v1/emails", noSalary).Status, "no value for the recipient")
	assert.NotContains(t, a.seen(owner, recruiter.ID), "email_fields:"+shortlisted.Id)
}
