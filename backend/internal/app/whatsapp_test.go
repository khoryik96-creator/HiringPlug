package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestWhatsAppMessages(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token

	// The starters are there the first time; salary and interview fields cannot be used.
	var list api.WhatsAppTemplateList
	res := a.request(http.MethodGet, "/v1/whatsapp-templates", recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&list)
	require.Len(t, list.Items, 3)
	assert.Contains(t, list.Placeholders, "job.title")
	assert.NotContains(t, list.Placeholders, "candidate.expectedSalary")
	bad := a.request(http.MethodPost, "/v1/whatsapp-templates", admin, map[string]any{"name": "Pay", "body": "You asked for {{candidate.expectedSalary}}"})
	assert.Equal(t, http.StatusBadRequest, bad.Status, "%s", bad.Body)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, "/v1/whatsapp-templates/"+list.Items[0].Id, recruiter, nil).Status, "changing templates needs emails.templates:manage")

	res = a.request(http.MethodPost, "/v1/whatsapp-templates", admin, map[string]any{"name": "Shortlisted", "body": "Hi {{candidate.firstName}}, you are shortlisted for {{job.title}} at {{company.name}}. {{sender.name}}"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var tpl api.WhatsAppTemplate
	res.JSON(&tpl)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/whatsapp-templates", admin, map[string]any{"name": "shortlisted", "body": "x"}).Status, "names are unique")
	res = a.request(http.MethodPut, "/v1/whatsapp-templates/"+tpl.Id, admin, map[string]any{"version": tpl.Version, "name": "Shortlisted", "body": tpl.Body + " Thanks!"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, "/v1/whatsapp-templates/"+tpl.Id, admin, map[string]any{"version": tpl.Version, "name": "Shortlisted", "body": "x"}).Status, "stale version")

	// For a candidate on a job: filled, and their number without a country code as a chat number.
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter, map[string]any{"title": "Finance Manager", "companyId": company.Id})
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah", "phone": "012-345 6789"})
	app := a.addToJob(recruiter, job.Id, siti.Id)
	res = a.request(http.MethodPost, "/v1/whatsapp-drafts", recruiter, map[string]any{"templateId": tpl.Id, "candidateId": siti.Id, "applicationId": app.Id, "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var d api.WhatsAppDraft
	res.JSON(&d)
	assert.Contains(t, d.Text, "Hi Siti, you are shortlisted for Finance Manager at Contoh Berhad.")
	assert.Empty(t, d.Missing)
	require.Len(t, d.Numbers, 1)
	assert.Equal(t, "60123456789", *d.Numbers[0].ChatNumber)

	// Blank, for a contact: their mobile and office phone (one not a valid number).
	hr := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman", "mobile": "+65 9123 4567", "phone": "12"})
	res = a.request(http.MethodPost, "/v1/whatsapp-drafts", recruiter, map[string]any{"contactId": hr.Id, "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&d)
	assert.Empty(t, d.Text)
	require.Len(t, d.Numbers, 2)
	assert.Equal(t, "Mobile", d.Numbers[0].Label)
	assert.Equal(t, "6591234567", *d.Numbers[0].ChatNumber)
	assert.Nil(t, d.Numbers[1].ChatNumber, "not a valid number")
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/whatsapp-drafts", recruiter, map[string]any{"timeZone": "Asia/Kuala_Lumpur"}).Status, "a candidate or a contact")

	// The message sent is logged as a WhatsApp note.
	res = a.request(http.MethodPost, "/v1/notes", recruiter, map[string]any{"subjectType": "candidate", "subjectId": siti.Id, "kind": "whatsapp", "body": "WhatsApp to +60123456789:\n" + d.Text})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var note api.Note
	res.JSON(&note)
	assert.Equal(t, api.NoteKindWhatsapp, note.Kind)
}
