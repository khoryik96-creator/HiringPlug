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

func TestWhatsAppDraftListsThePersonsFields(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token

	// Settings shows the fields with their labels, salary aside.
	var list api.WhatsAppTemplateList
	a.request(http.MethodGet, "/v1/whatsapp-templates", recruiter, nil).JSON(&list)
	require.Len(t, list.Fields, len(list.Placeholders))
	assert.Equal(t, api.WhatsAppField{Key: "candidate.firstName", Label: "First name"}, list.Fields[0])
	assert.Contains(t, list.Fields, api.WhatsAppField{Key: "sender.name", Label: "Your name"})
	for _, f := range list.Fields {
		assert.NotContains(t, f.Key, "alary", "no salary field")
	}

	// A candidate with salary saved, on two jobs: the chosen application's job and company.
	contoh := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	other := a.createCompany(recruiter, map[string]any{"name": "Lain Sdn Bhd"})
	finance := a.createJob(recruiter, map[string]any{"title": "Finance Manager", "companyId": contoh.Id})
	audit := a.createJob(recruiter, map[string]any{"title": "Audit Lead", "companyId": other.Id})
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aminah", "phone": "012-345 6789"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+siti.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"},
		"expectedSalary": map[string]any{"min": "10000", "max": "12000", "currency": "MYR", "period": "month"},
	}).Status)
	a.addToJob(recruiter, finance.Id, siti.Id)
	onAudit := a.addToJob(recruiter, audit.Id, siti.Id)

	draft := func(body map[string]any) map[string]api.WhatsAppFieldValue {
		res := a.request(http.MethodPost, "/v1/whatsapp-drafts", recruiter, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		require.NotContains(t, string(res.Body), "8,500")
		require.NotContains(t, string(res.Body), "10,000")
		var d api.WhatsAppDraft
		res.JSON(&d)
		require.NotEmpty(t, d.Fields)
		out := map[string]api.WhatsAppFieldValue{}
		for _, f := range d.Fields {
			assert.NotContains(t, f.Key, "alary", "no salary field")
			out[f.Key] = f
		}
		assert.Equal(t, "candidate.firstName", d.Fields[0].Key, "in the order fields are offered")
		return out
	}
	fields := draft(map[string]any{"candidateId": siti.Id, "applicationId": onAudit.Id, "timeZone": "Asia/Kuala_Lumpur"})
	assert.Equal(t, api.WhatsAppFieldValue{Key: "candidate.firstName", Label: "First name", Value: "Siti"}, fields["candidate.firstName"])
	assert.Equal(t, "Siti Aminah", fields["candidate.fullName"].Value)
	assert.Equal(t, api.WhatsAppFieldValue{Key: "job.title", Label: "Job title", Value: "Audit Lead"}, fields["job.title"])
	assert.Equal(t, "Lain Sdn Bhd", fields["company.name"].Value)
	assert.NotEmpty(t, fields["sender.name"].Value)
	assert.Empty(t, fields["client.name"].Value, "unknown values are empty")

	// Without an application there is no job.
	fields = draft(map[string]any{"candidateId": siti.Id, "timeZone": "Asia/Kuala_Lumpur"})
	assert.Empty(t, fields["job.title"].Value)
	assert.Equal(t, "Job title", fields["job.title"].Label)

	// A contact's draft has the contact's values.
	hr := a.createContact(recruiter, map[string]any{"companyId": contoh.Id, "firstName": "Aisyah", "lastName": "Rahman", "mobile": "+65 9123 4567"})
	fields = draft(map[string]any{"contactId": hr.Id, "timeZone": "Asia/Kuala_Lumpur"})
	assert.Equal(t, "Aisyah", fields["candidate.firstName"].Value)
	assert.Equal(t, "Aisyah Rahman", fields["candidate.fullName"].Value)
	assert.Equal(t, "Contoh Berhad", fields["company.name"].Value)
	assert.Empty(t, fields["job.title"].Value)
}
