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

// One email to the client about several candidates: each one's details are
// in the message (put there by the composer from their own values), each
// one's profile is attached, and the email is kept on each candidate.
func TestEmailingTheClientAboutSeveralCandidates(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	client := "hr." + tenant.ID + "@client.test"
	company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd"})
	hr := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman", "email": client})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "contactId": hr.Id, "title": "Hub Operations Manager"})
	farid := a.createCandidate(token, map[string]any{"firstName": "Farid", "lastName": "Hassan"})
	nurul := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Huda"})
	other := a.createCandidate(token, map[string]any{"firstName": "Lim", "lastName": "Wei Jie"})
	faridApp, nurulApp := a.addToJob(token, job.Id, farid.Id), a.addToJob(token, job.Id, nurul.Id)
	cvOf := func(c api.Candidate, name string) api.Document {
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", name, minimalPDF)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var d api.Document
		res.JSON(&d)
		return d
	}
	faridCV, nurulCV, otherCV := cvOf(farid, "Farid Hassan CV.pdf"), cvOf(nurul, "Nurul Huda CV.pdf"), cvOf(other, "Lim CV.pdf")
	both := []map[string]any{{"candidateId": farid.Id, "applicationId": faridApp.Id}, {"candidateId": nurul.Id, "applicationId": nurulApp.Id}}
	// As the composer writes each candidate's block: their values, marked with their field.
	body := `<p>Dear [Client contact],</p><p>Please find 2 candidates for [Job title]:</p>` +
		`<p><b>1. Farid Hassan</b><br>Expected: <span data-field="candidate.expectedSalary">RM 9,000 per month</span></p>` +
		`<p><b>2. Nurul Huda</b><br>Expected: <span data-field="candidate.expectedSalary">RM 10,000 per month</span></p>`

	t.Run("fields left in the message are the job's and the client's, not one candidate's", func(t *testing.T) {
		var p api.EmailPreview
		res := a.request(http.MethodPost, "/v1/email-previews", token, map[string]any{
			"subject": "Candidates for [Job title]", "bodyHtml": body + "<p>[First name]</p>", "candidates": both, "timeZone": "Asia/Kuala_Lumpur",
		})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&p)
		assert.Equal(t, "Candidates for Hub Operations Manager", p.Subject)
		assert.Contains(t, p.BodyHtml, "Dear Aisyah Rahman,")
		assert.Equal(t, []string{"[First name]"}, p.Missing, "whose first name? Each candidate's goes in their block")
		assert.NotContains(t, p.FieldValues, "[Full name]")
		assert.Equal(t, "Hub Operations Manager", p.FieldValues["[Job title]"])
		assert.Equal(t, client, p.ClientEmail.MustGet())
		assert.True(t, p.Salary)
	})

	var sent api.Email
	t.Run("sent with each one's profile, and kept on each candidate and the job", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/emails", token, map[string]any{
			"to": []string{client}, "subject": "Candidates for [Job title]", "bodyHtml": body, "candidates": both,
			"cvs": []map[string]any{{"documentId": faridCV.Id}, {"documentId": nurulCV.Id}},
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&sent)
		assert.True(t, sent.Salary)
		require.Len(t, sent.Attachments, 2)
		assert.Equal(t, "Farid Hassan CV.pdf", sent.Attachments[0].FileName)
		for _, c := range []api.Candidate{farid, nurul} {
			emails := a.emails(token, c.Id)
			require.Len(t, emails, 1, c.FirstName)
			assert.Equal(t, sent.Id, emails[0].Id)
		}
		assert.Empty(t, a.emails(token, other.Id))
		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT changes::text FROM audit_log WHERE entity_type = 'job' AND entity_id = $1 AND action = 'email.shortlist_sent'`, job.Id).Scan(&changes))
		assert.Contains(t, changes, `"candidates": 2`)
		assert.NotContains(t, changes, "9,000", "never the salary in the audit log")

		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To(client)
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].Data, "Farid Hassan CV.pdf")
		assert.Contains(t, msgs[0].Data, "Nurul Huda CV.pdf")
	})

	t.Run("refused: a candidate twice, one candidate and several, another's CV or application", func(t *testing.T) {
		send := func(extra map[string]any) *response {
			b := map[string]any{"to": []string{client}, "subject": "Candidates", "bodyHtml": "<p>Hi</p>", "candidates": both}
			for k, v := range extra {
				b[k] = v
			}
			return a.request(http.MethodPost, "/v1/emails", token, b)
		}
		assert.Equal(t, http.StatusBadRequest, send(map[string]any{"candidates": []map[string]any{{"candidateId": farid.Id}, {"candidateId": farid.Id}}}).Status)
		assert.Equal(t, http.StatusBadRequest, send(map[string]any{"applicationId": faridApp.Id}).Status)
		assert.Equal(t, http.StatusBadRequest, send(map[string]any{"cvs": []map[string]any{{"documentId": otherCV.Id}}}).Status, "not one of theirs")
		assert.Equal(t, http.StatusBadRequest, send(map[string]any{"candidates": []map[string]any{{"candidateId": other.Id, "applicationId": faridApp.Id}}}).Status)
	})

	t.Run("a merged duplicate's shortlists follow the candidate kept, once", func(t *testing.T) {
		keep := a.createCandidate(token, map[string]any{"firstName": "Nurul", "lastName": "Huda Binti Ali"})
		res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", token, map[string]any{"duplicateId": nurul.Id, "version": keep.Version})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		emails := a.emails(token, keep.Id)
		require.Len(t, emails, 1)
		assert.Equal(t, sent.Id, emails[0].Id)
	})
}
