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
	a.sameSalaryDay()
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
	// A draft from a template counts only when it comes back with the salary
	// filled in ({{placeholders}}); [Field] tokens are left for the composer.
	var withPay, token, plain api.EmailTemplate
	ok(a.request(http.MethodPost, "/v1/email-templates", rt, map[string]any{"name": "Pay", "subject": "{{candidate.fullName}}", "body": "Expecting {{candidate.expectedSalary}}"}), http.StatusCreated).JSON(&withPay)
	ok(a.request(http.MethodPost, "/v1/email-templates", rt, map[string]any{"name": "Pay later", "subject": "[Full name]", "body": "Expecting [Expected salary]"}), http.StatusCreated).JSON(&token)
	ok(a.request(http.MethodPost, "/v1/email-templates", rt, map[string]any{"name": "Hello", "subject": "Hi [First name]", "body": "Hello"}), http.StatusCreated).JSON(&plain)
	draft := map[string]any{"candidateId": c.Id, "timeZone": "Asia/Kuala_Lumpur"}
	var got api.EmailDraft
	ok(a.request(http.MethodPost, "/v1/email-drafts", rt, with(draft, "templateId", token.Id)), http.StatusOK).JSON(&got)
	assert.Contains(t, got.Body, "[Expected salary]", "no salary in this draft")
	ok(a.request(http.MethodPost, "/v1/email-drafts", rt, with(draft, "templateId", plain.Id)), http.StatusOK)
	assert.Equal(t, 1, counted(recruiter.ID, "email_fields", c.Id))
	ok(a.request(http.MethodPost, "/v1/email-drafts", rt, with(draft, "templateId", withPay.Id)), http.StatusOK).JSON(&got)
	assert.Contains(t, got.Body, "10,913", "the draft shows the salary")
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

	// Offers: only lists that ask for the salary get it, and count it.
	company := a.createCompany(rt, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(rt, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(rt, job.Id, c.Id)
	a.offer(http.MethodPost, "/v1/applications/"+app.Id+"/offers", rt, map[string]any{"salary": map[string]any{"amount": "9123", "currency": "MYR", "period": "month"}}, http.StatusCreated)
	assert.Zero(t, counted(recruiter.ID, "offer", c.Id), "what they just typed in")
	ok(a.request(http.MethodGet, "/v1/offers?status=sent", rt, nil), http.StatusOK)
	ok(a.request(http.MethodGet, "/v1/offers", rt, nil), http.StatusOK)
	unasked := ok(a.request(http.MethodGet, "/v1/offers?applicationId="+app.Id, rt, nil), http.StatusOK)
	assert.NotContains(t, string(unasked.Body), "9123", "checking whether an offer is open gets no salary")
	assert.Zero(t, counted(recruiter.ID, "offer", c.Id), "lists that do not ask for it")
	asked := ok(a.request(http.MethodGet, "/v1/offers?applicationId="+app.Id+"&includeSalary=true", rt, nil), http.StatusOK)
	assert.Contains(t, string(asked.Body), "9123")
	assert.Equal(t, 1, counted(recruiter.ID, "offer", c.Id))
	ok(a.request(http.MethodGet, "/v1/offers?includeSalary=true", rt, nil), http.StatusOK)
	assert.Equal(t, 2, counted(recruiter.ID, "offer", c.Id), "any list that asks")

	// Placements: an application's placement.
	job2 := a.createJob(rt, map[string]any{"companyId": company.Id, "title": "Auditor"})
	app2 := a.addToJob(rt, job2.Id, c.Id)
	a.placement(http.MethodPost, "/v1/applications/"+app2.Id+"/placement", rt, map[string]any{
		"startDate": "2026-12-01", "money": map[string]any{"salary": map[string]any{"amount": "7345", "currency": "MYR", "period": "month"}, "feePercent": "20"},
	}, http.StatusCreated)
	dashboard := ok(a.request(http.MethodGet, "/v1/placements", rt, nil), http.StatusOK)
	assert.NotContains(t, string(dashboard.Body), "7345", "the dashboard does not show it")
	assert.Contains(t, string(dashboard.Body), "feeAmount", "the fee is still there")
	ok(a.request(http.MethodGet, "/v1/placements?applicationId="+app2.Id, rt, nil), http.StatusOK)
	assert.Zero(t, counted(recruiter.ID, "placement", c.Id))
	shown := ok(a.request(http.MethodGet, "/v1/placements?applicationId="+app2.Id+"&includeSalary=true", rt, nil), http.StatusOK)
	assert.Contains(t, string(shown.Body), "7345")
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
	a.sameSalaryDay()
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

// A change that sends the candidate back with their salary shows it as much
// as their page does: a profile change, their history, a merge and a
// screening call that did not set the salary are counted; a call that set
// it is not (what they just typed in). Salary notes alone, or a formatted
// CV "with the salary" of a candidate who has none, show no salary.
func TestACandidateSentBackWithTheirSalaryIsCounted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	rt := recruiter.Token
	c := a.withSalary(rt, map[string]any{"firstName": "Siti", "email": "siti@example.com"})
	dup := a.createCandidate(rt, map[string]any{"firstName": "Siti", "lastName": "Dup"})
	pages := func() int {
		t.Helper()
		return a.seen(owner, recruiter.ID)["candidate:"+c.Id].Count
	}
	var got api.Candidate
	send := func(method, path string, body map[string]any) {
		t.Helper()
		res := a.request(method, path, rt, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		if method == http.MethodPost && strings.HasSuffix(path, "/screening-calls") {
			var out api.ScreeningCallResult
			res.JSON(&out)
			got = out.Candidate
		} else {
			res.JSON(&got)
		}
		require.True(t, got.Compensation.IsSpecified() && !got.Compensation.IsNull(), "sent back with the salary")
	}

	send(http.MethodPatch, "/v1/candidates/"+c.Id, map[string]any{"version": c.Version, "currentTitle": "Auditor"})
	assert.Equal(t, 1, pages(), "a profile change")
	send(http.MethodPut, "/v1/candidates/"+c.Id+"/history", map[string]any{"version": got.Version, "experience": []any{}, "education": []any{}})
	assert.Equal(t, 2, pages(), "their history")
	send(http.MethodPost, "/v1/candidates/"+c.Id+"/merge", map[string]any{"duplicateId": dup.Id, "version": got.Version})
	assert.Equal(t, 3, pages(), "a merge")
	send(http.MethodPost, "/v1/candidates/"+c.Id+"/screening-calls", map[string]any{"version": got.Version, "notes": "Called"})
	assert.Equal(t, 4, pages(), "a call that left the salary as it was")
	send(http.MethodPost, "/v1/candidates/"+c.Id+"/screening-calls", map[string]any{"version": got.Version, "compensation": map[string]any{
		"expectedVersion": 1, "currentSalary": map[string]any{"amount": "8600", "currency": "MYR", "period": "month"}, "expectedSalary": nil}})
	assert.Equal(t, 4, pages(), "a call that set it: what they just typed in")

	// Salary notes alone are not a salary; nor is a CV "with the salary" of someone who has none.
	notes := a.createCandidate(rt, map[string]any{"firstName": "Notes"})
	res := a.request(http.MethodPut, "/v1/candidates/"+notes.Id+"/compensation", rt, map[string]any{
		"expectedVersion": 0, "currentSalary": nil, "expectedSalary": nil, "salaryNotes": "Ask again in March",
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+notes.Id, rt, nil).Status)
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/candidates/"+notes.Id+"/formatted-cvs", rt, map[string]any{"includeSalary": true}).Status)
	seen := a.seen(owner, recruiter.ID)
	assert.NotContains(t, seen, "candidate:"+notes.Id)
	assert.NotContains(t, seen, "cv:"+notes.Id)
}

// Merging a duplicate moves who was shown its salary to the kept candidate:
// a line of theirs for the same person, kind and day takes the duplicate's
// count; lists name the kept candidate.
func TestAMergeMovesWhoWasShownTheDuplicatesSalary(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	keep := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	dup := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Duplicate"})
	for _, view := range []struct {
		token, id string
	}{{recruiter.Token, keep.Id}, {recruiter.Token, keep.Id}, {recruiter.Token, dup.Id}, {admin.Token, dup.Id}} {
		require.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+view.id, view.token, nil).Status)
	}
	res := a.request(http.MethodPost, "/v1/candidates/search", admin.Token, map[string]any{"q": "Duplicate", "includeSalary": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	res = a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", owner, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	byRecruiter := a.seen(owner, recruiter.ID)
	assert.Equal(t, 3, byRecruiter["candidate:"+keep.Id].Count, "their two views and the duplicate's")
	assert.NotContains(t, byRecruiter, "candidate:"+dup.Id)
	byAdmin := a.seen(owner, admin.ID)
	assert.Equal(t, 1, byAdmin["candidate:"+keep.Id].Count, "the duplicate's line, now about the kept candidate")
	assert.Equal(t, "Siti Aisyah", byAdmin["candidate:"+keep.Id].CandidateName.MustGet())
	assert.Empty(t, a.salaryViews(owner, "?candidateId="+dup.Id).Items)
	kinds := []api.SalaryViewKind{}
	for _, v := range a.salaryViews(owner, "?user="+admin.ID+"&candidateId="+keep.Id).Items {
		kinds = append(kinds, v.Kind)
	}
	assert.ElementsMatch(t, []api.SalaryViewKind{api.SalaryViewKindCandidate, api.SalaryViewKindCandidateList}, kinds, "the search that showed the duplicate")
}

// A formatted CV with the salary whose file (or PDF) cannot be opened shows
// nothing, so it is not counted.
func TestASalaryCVThatCannotBeOpenedIsNotCounted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]
	c := a.withSalary(recruiter.Token, map[string]any{"firstName": "Siti"})
	var cv api.FormattedCv
	res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", recruiter.Token, map[string]any{"includeSalary": true})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&cv)
	require.Equal(t, 1, a.seen(owner, recruiter.ID)["cv:"+c.Id].Count, "made")
	_, err := testutil.Current.Admin.Exec(t.Context(),
		`UPDATE document SET storage_key = left(storage_key, -4) || '0000', pdf_key = left(storage_key, -4) || '0001', pdf_status = 'ready' WHERE id = $1`, cv.DocumentId)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+cv.DocumentId+"/file", recruiter.Token, nil).Status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+cv.DocumentId+"/pdf", recruiter.Token, nil).Status)
	assert.Equal(t, 1, a.seen(owner, recruiter.ID)["cv:"+c.Id].Count, "nothing was shown")
}
