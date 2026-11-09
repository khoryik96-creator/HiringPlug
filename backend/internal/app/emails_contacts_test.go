package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestEmailingAClientsContact(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	hr := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman", "email": "aisyah@contoh.test"})

	res := a.request(http.MethodPost, "/v1/email-templates", admin, map[string]any{
		"name": "Catch-up", "subject": "[Client company]: catching up", "bodyHtml": "<p>Dear [First name],</p><p>[Your name], [Agency]</p>",
	})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var tpl api.EmailTemplate
	res.JSON(&tpl)

	res = a.request(http.MethodPost, "/v1/email-drafts", recruiter, map[string]any{"templateId": tpl.Id, "contactId": hr.Id, "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var d api.EmailDraft
	res.JSON(&d)
	assert.Equal(t, []string{"aisyah@contoh.test"}, toStrings(d.To), "to the contact")
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/email-drafts", recruiter,
		map[string]any{"templateId": tpl.Id, "timeZone": "Asia/Kuala_Lumpur"}).Status, "a candidate or a contact")

	res = a.request(http.MethodPost, "/v1/email-previews", recruiter, map[string]any{
		"subject": "[Client company]: catching up", "bodyHtml": "<p>Dear [First name] [Last name] ([Client contact])</p>", "contactId": hr.Id, "timeZone": "Asia/Kuala_Lumpur",
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var pv api.EmailPreview
	res.JSON(&pv)
	assert.Equal(t, "Contoh Berhad: catching up", pv.Subject)
	assert.Contains(t, pv.BodyHtml, "Dear Aisyah Rahman (Aisyah Rahman)")
	assert.Equal(t, "Aisyah", pv.FieldValues["[First name]"])

	res = a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
		"to": []string{"aisyah@contoh.test"}, "subject": "[Client company]: catching up", "bodyHtml": "<p>Dear [First name],</p>",
		"contactId": hr.Id, "timeZone": "Asia/Kuala_Lumpur",
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var sent api.Email
	res.JSON(&sent)
	assert.Equal(t, hr.Id, sent.ContactId.MustGet())
	assert.Equal(t, "Contoh Berhad: catching up", sent.Subject)
	assert.True(t, sent.CandidateId.IsNull())

	res = a.request(http.MethodGet, "/v1/emails?contactId="+hr.Id, recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.EmailList
	res.JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, sent.Id, list.Items[0].Id)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/emails", recruiter, nil).Status, "whose emails")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/email-previews", recruiter, map[string]any{
		"subject": "x", "bodyHtml": "<p>x</p>", "contactId": "cont_01jbz6w0m0k8q9x2r3s4t5v6w7", "timeZone": "Asia/Kuala_Lumpur",
	}).Status)
}
