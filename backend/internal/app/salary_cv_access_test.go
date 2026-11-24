package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A formatted CV made with the salary is opened, downloaded, turned into a
// PDF, searched in or attached to an email only by those who may see
// salaries (candidates.compensation:read); the others see it listed but
// cannot open it. A formatted CV without the salary is theirs to open.
func TestOnlyThoseWhoMaySeeSalariesOpenAFormattedCVWithIt(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	c := a.withSalary(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com"})
	var withPay, noPay api.FormattedCv
	res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", recruiter, map[string]any{"includeSalary": true})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&withPay)
	res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", recruiter, map[string]any{})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&noPay)
	// Both have a PDF made (the worker's copy stands in as the Word file).
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE document SET pdf_key = storage_key, pdf_status = 'ready' WHERE id = ANY ($1)`, []string{withPay.DocumentId, noPay.DocumentId})
	require.NoError(t, err)

	opens := func(token, doc string) map[string]int {
		status := map[string]int{
			"open":       a.request(http.MethodGet, "/v1/documents/"+doc, token, nil).Status,
			"file":       a.request(http.MethodGet, "/v1/documents/"+doc+"/file", token, nil).Status,
			"pdf":        a.request(http.MethodGet, "/v1/documents/"+doc+"/pdf", token, nil).Status,
			"make pdf":   a.request(http.MethodPost, "/v1/documents/"+doc+"/pdf", token, nil).Status,
			"highlights": a.request(http.MethodGet, "/v1/documents/"+doc+"/highlights?boolean=Siti", token, nil).Status,
		}
		return status
	}
	allowed := map[string]int{"open": 200, "file": 200, "pdf": 200, "make pdf": 202, "highlights": 200}
	refused := map[string]int{"open": 403, "file": 403, "pdf": 403, "make pdf": 403, "highlights": 403}
	assert.Equal(t, allowed, opens(recruiter, withPay.DocumentId), "a recruiter may see salaries")
	for _, role := range []api.Role{api.RoleSourcer, api.RoleHiringManager, api.RoleViewer} {
		token := tenant.Users[role].Token
		assert.Equal(t, refused, opens(token, withPay.DocumentId), role)
		assert.Equal(t, allowed, opens(token, noPay.DocumentId), "%s: a CV without the salary", role)
		var docs api.DocumentList
		a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/documents", token, nil).JSON(&docs)
		assert.Len(t, docs.Items, 2, "%s: still listed", role)
	}
	res = a.request(http.MethodGet, "/v1/documents/"+withPay.DocumentId+"/file", tenant.Users[api.RoleSourcer].Token, nil)
	assert.Equal(t, api.ErrorCodeFORBIDDEN, res.Error().Error.Code)
	assert.Contains(t, res.Error().Error.Message, "salary")

	// Attached to an email: only by someone who may see salaries.
	send := map[string]any{"candidateId": c.Id, "to": []string{"hr@client.example"}, "subject": "Siti", "bodyHtml": "<p>CV attached</p>",
		"timeZone": "Asia/Kuala_Lumpur", "cvs": []map[string]any{{"documentId": withPay.DocumentId}}}
	res = a.request(http.MethodPost, "/v1/emails", tenant.Users[api.RoleSourcer].Token, send)
	assert.Equal(t, http.StatusForbidden, res.Status, "%s", res.Body)
	res = a.request(http.MethodPost, "/v1/emails", recruiter, send)
	assert.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	send["cvs"] = []map[string]any{{"documentId": noPay.DocumentId}}
	res = a.request(http.MethodPost, "/v1/emails", tenant.Users[api.RoleSourcer].Token, send)
	assert.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
}

// A copy of a candidate's data (an access request) holds the text of a
// formatted CV made with the salary only for someone who may see salaries,
// and then counts it as a salary shown; for anyone else it says the text
// was left out. The salary section and an email with the salary in the
// copy count too.
func TestACopyOfTheirDataHoldsASalaryCVsTextOnlyForThoseWhoMaySeeSalaries(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	a.sameSalaryDay()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleSourcer)
	owner, sourcer := tenant.Users[api.RoleOwner], tenant.Users[api.RoleSourcer]
	c := a.withSalary(owner.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.com"})
	var cv api.FormattedCv
	res := a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/formatted-cvs", owner.Token, map[string]any{"includeSalary": true})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&cv)
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE document SET extracted_text = 'Siti Aisyah. Expected salary: MYR 10,913 a month' WHERE id = $1`, cv.DocumentId)
	require.NoError(t, err)
	res = a.request(http.MethodPost, "/v1/emails", owner.Token, map[string]any{"candidateId": c.Id, "to": []string{"hr@client.example"},
		"subject": "Siti", "bodyHtml": "<p>Expecting [Expected salary]</p>", "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	a.setRole(owner.Token, "sourcer", map[string]any{"version": 0, "permissions": append(authz.PermissionsForRole(api.RoleSourcer), api.PermissionPrivacyManage),
		"candidateScope": "all", "jobCandidates": true}, http.StatusOK)

	export := func(token string) (string, []string) {
		t.Helper()
		res := a.request(http.MethodGet, "/v1/candidates/"+c.Id+"/data-export", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var e api.CandidateDataExport
		res.JSON(&e)
		return string(res.Body), e.NotIncluded
	}
	body, left := export(sourcer.Token)
	assert.NotContains(t, body, "10,913", "no salary in the CV's text")
	assert.Contains(t, body, `"formatted"`, "the CV is still listed")
	assert.Contains(t, left, "The text of 1 formatted CV(s) made with the salary: you may not see it.")
	assert.Empty(t, a.seen(owner.Token, sourcer.ID), "no salary in the copy: nothing counted")

	body, left = export(owner.Token)
	assert.Contains(t, body, "MYR 10,913", "the owner may see salaries")
	for _, l := range left {
		assert.NotContains(t, l, "formatted CV")
	}
	seen := a.seen(owner.Token, owner.ID)
	assert.Equal(t, 2, seen["cv:"+c.Id].Count, "made, then in the copy")
	assert.Equal(t, 1, seen["candidate:"+c.Id].Count, "the salary section")
	assert.Equal(t, 1, seen["email_read:"+c.Id].Count, "the email with the salary")
}
