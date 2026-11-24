package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
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
