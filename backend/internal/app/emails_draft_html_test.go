package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A formatted template (written with the toolbar) comes into the composer
// formatted and filled: its message used to come out empty, as only the
// plain-text version was filled.
func TestADraftOfAFormattedTemplateIsFormatted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	res := a.request(http.MethodPost, "/v1/email-templates", token, map[string]any{
		"name": "Formatted invite", "subject": "About {{job.title}}",
		"bodyHtml": "<p>Dear {{candidate.firstName}},</p><p>We would like to talk about <b>{{job.title}}</b>.</p>",
	})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var tpl api.EmailTemplate
	res.JSON(&tpl)

	company := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "R&D Accountant"})
	cand := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.test"})
	appl := a.addToJob(token, job.Id, cand.Id)

	res = a.request(http.MethodPost, "/v1/email-drafts", token, map[string]any{
		"templateId": tpl.Id, "candidateId": cand.Id, "applicationId": appl.Id, "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var d api.EmailDraft
	res.JSON(&d)
	html := d.BodyHtml.MustGet()
	assert.Contains(t, html, "<p>Dear Siti,</p>")
	assert.Contains(t, html, "<b>R&amp;D Accountant</b>", "values go in as text, never as markup")
	assert.Equal(t, "About R&D Accountant", d.Subject)
	assert.Empty(t, d.Missing)

	// The starter templates are formatted too: the composer used to show
	// them with an empty message.
	starter := a.emailTemplates(token)["Interview invitation"]
	res = a.request(http.MethodPost, "/v1/email-drafts", token, map[string]any{"templateId": starter.Id, "candidateId": cand.Id, "timeZone": "UTC"})
	res.JSON(&d)
	assert.Contains(t, d.BodyHtml.MustGet(), "Dear Siti")
}
