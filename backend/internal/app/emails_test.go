package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) emailTemplates(token string) map[string]api.EmailTemplate {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/email-templates", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.EmailTemplateList
	res.JSON(&list)
	out := map[string]api.EmailTemplate{}
	for _, t := range list.Items {
		out[t.Name] = t
	}
	return out
}

func (a *testApp) emails(token, candidateID string) []api.Email {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/emails?candidateId="+candidateID, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.EmailList
	res.JSON(&list)
	return list.Items
}

func TestEmailTemplatesAndDrafts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleSourcer, api.RoleAdmin)
	recruiter, sourcer, admin := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer], tenant.Users[api.RoleAdmin]

	templates := a.emailTemplates(recruiter.Token)
	require.Len(t, templates, 4, "every company starts with the starter templates")
	assert.Len(t, a.emailTemplates(sourcer.Token), 4, "added once")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/email-templates", tenant.Users[api.RoleHiringManager].Token, nil).Status)

	t.Run("templates are checked and changed with a version", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/email-templates", recruiter.Token, map[string]any{"name": "Offer", "subject": "Your salary: {{candidate.salary}}", "body": "x"})
		assert.Equal(t, []string{"subject"}, fieldNames(t, res.Error()), "only known placeholders")
		res = a.request(http.MethodPost, "/v1/email-templates", recruiter.Token, map[string]any{"name": "interview INVITATION", "subject": "x", "body": "x"})
		assert.Equal(t, []string{"name"}, fieldNames(t, res.Error()), "names are unique")
		assert.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/email-templates", sourcer.Token, map[string]any{"name": "Mine", "subject": "x", "body": "x"}).Status,
			"anyone who writes emails may add a template")

		res = a.request(http.MethodPost, "/v1/email-templates", recruiter.Token, map[string]any{"name": " Follow  up ", "subject": "Following up on {{job.title}}", "body": "Hi {{candidate.firstName}}"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var created api.EmailTemplate
		res.JSON(&created)
		assert.Equal(t, "Follow up", created.Name)
		path := "/v1/email-templates/" + created.Id
		update := map[string]any{"version": created.Version, "name": "Follow up", "subject": "Still keen on {{job.title}}?", "body": "Hi {{candidate.firstName}}"}
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, path, recruiter.Token, update).Status, "changing the company's templates is for admins")
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodDelete, path, recruiter.Token, nil).Status)
		assert.Equal(t, http.StatusOK, a.request(http.MethodPut, path, admin.Token, update).Status)
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, path, admin.Token, update).Status, "stale version")
	})

	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	hr := a.createContact(recruiter.Token, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "email": "aisyah@contoh.test"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "contactId": hr.Id, "title": "Accountant"})
	cand := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti@example.test"})
	application := a.addToJob(recruiter.Token, job.Id, cand.Id)
	start := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC) // 10:00 in Kuala Lumpur
	iv := a.interview(http.MethodPost, "/v1/applications/"+application.Id+"/interviews", recruiter.Token, map[string]any{
		"title": "First interview", "startsAt": start, "durationMinutes": 90, "mode": "video",
		"location": "https://meet.example/abc", "interviewerIds": []string{tenant.Users[api.RoleHiringManager].ID},
	}, http.StatusCreated)

	draft := func(body map[string]any) api.EmailDraft {
		t.Helper()
		res := a.request(http.MethodPost, "/v1/email-drafts", recruiter.Token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var d api.EmailDraft
		res.JSON(&d)
		return d
	}

	t.Run("an interview invitation is filled from the interview, in the sender's time zone", func(t *testing.T) {
		d := draft(map[string]any{"templateId": templates["Interview invitation"].Id, "candidateId": cand.Id, "interviewId": iv.Id, "timeZone": "Asia/Kuala_Lumpur"})
		assert.Equal(t, []string{"siti@example.test"}, toStrings(d.To))
		assert.Equal(t, "Interview for Accountant on Tuesday, 10 November 2026, 10:00 AM (GMT+8)", d.Subject)
		assert.Contains(t, d.Body, "Dear Siti,")
		assert.Contains(t, d.Body, "(1 hour 30 minutes)")
		assert.Contains(t, d.Body, "How: Video call\nWhere: https://meet.example/abc")
		assert.Contains(t, d.Body, "Test recruiter\nTest tenant TestEmailTemplatesAndDrafts")
		assert.NotContains(t, d.Body, "\n\n\n", "no instructions leave no gap")
		assert.Empty(t, d.Missing)
	})

	t.Run("templates for the client go to the job's hiring contact", func(t *testing.T) {
		d := draft(map[string]any{"templateId": templates["Submit candidate to client"].Id, "candidateId": cand.Id, "applicationId": application.Id, "timeZone": "UTC"})
		assert.Equal(t, []string{"aisyah@contoh.test"}, toStrings(d.To))
		assert.Equal(t, "Candidate for Accountant: Siti Aisyah", d.Subject)
		assert.Contains(t, d.Body, "the Accountant role at Contoh Berhad")
	})

	t.Run("what has no value is left in brackets and listed", func(t *testing.T) {
		d := draft(map[string]any{"templateId": templates["Interview invitation"].Id, "candidateId": cand.Id, "timeZone": "UTC"})
		assert.Contains(t, d.Subject, "[job.title]")
		assert.Contains(t, d.Missing, "interview.when")
	})

	t.Run("checks the records belong together", func(t *testing.T) {
		other := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"})
		res := a.request(http.MethodPost, "/v1/email-drafts", recruiter.Token, map[string]any{"templateId": templates["Interview invitation"].Id, "candidateId": other.Id, "interviewId": iv.Id, "timeZone": "UTC"})
		assert.Equal(t, []string{"applicationId"}, fieldNames(t, res.Error()))
		res = a.request(http.MethodPost, "/v1/email-drafts", recruiter.Token, map[string]any{"templateId": templates["Interview invitation"].Id, "candidateId": cand.Id, "timeZone": "Mars/Base"})
		assert.Equal(t, []string{"timeZone"}, fieldNames(t, res.Error()))
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter]
		res = a.request(http.MethodPost, "/v1/email-drafts", stranger.Token, map[string]any{"templateId": templates["Interview invitation"].Id, "candidateId": cand.Id, "timeZone": "UTC"})
		assert.Equal(t, http.StatusNotFound, res.Status, "another company's template")
	})
}

func toStrings[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func TestSendingEmail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter]
	siti := "siti." + tenant.ID + "@example.test" // tests share the mail server: their own addresses
	cand := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "email": siti})
	send := map[string]any{
		"to": []string{strings.ToUpper(siti)}, "cc": []string{"boss." + tenant.ID + "@agency.test"}, "subject": "Your interview",
		"body": "Dear Siti,\n\nSee you on Tuesday — selamat datang.\n", "candidateId": cand.Id,
	}

	res := a.request(http.MethodPost, "/v1/emails", recruiter.Token, send)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var queued api.Email
	res.JSON(&queued)
	assert.Equal(t, api.EmailStatusQueued, queued.Status)
	assert.Equal(t, []string{siti}, queued.To)
	assert.Equal(t, "Test recruiter", queued.SentByName)

	runCoreJobsUntilIdle(t, a, tenant.ID)
	msgs := mailServer.To(siti)
	require.Len(t, msgs, 1)
	assert.Equal(t, "no-reply@agency.test", msgs[0].From)
	assert.ElementsMatch(t, []string{siti, "boss." + tenant.ID + "@agency.test"}, msgs[0].To)
	assert.Contains(t, msgs[0].Data, "Reply-To: <recruiter."+tenant.ID+"@test.local>")
	assert.Contains(t, msgs[0].Data, "Test recruiter (Test tenant TestSendingEmail)")

	sent := a.emails(tenant.Users[api.RoleHiringManager].Token, cand.Id)
	require.Len(t, sent, 1, "hiring managers see what was sent")
	assert.Equal(t, api.EmailStatusSent, sent[0].Status)
	assert.True(t, sent[0].SentAt.IsSpecified())
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/emails", tenant.Users[api.RoleHiringManager].Token, send).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/emails?candidateId="+cand.Id, tenant.Users[api.RoleViewer].Token, nil).Status)

	t.Run("a refused address fails the email for good, with the reason", func(t *testing.T) {
		refused := map[string]any{"to": []string{"refuse." + tenant.ID + "@example.test"}, "subject": "x", "body": "x", "candidateId": cand.Id}
		require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/emails", recruiter.Token, refused).Status)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		latest := a.emails(recruiter.Token, cand.Id)[0]
		assert.Equal(t, api.EmailStatusFailed, latest.Status)
		reason, _ := latest.Error.Get()
		assert.Contains(t, reason, "550")
	})

	t.Run("checks what is sent", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/emails", recruiter.Token, map[string]any{"to": []string{"a@b.test"}, "subject": "x", "body": "x", "candidateId": "cand_01jbz6w0m0k8q9x2r3s4t5v6w7"})
		assert.Equal(t, http.StatusNotFound, res.Status)
		res = a.request(http.MethodPost, "/v1/emails", recruiter.Token, map[string]any{"to": []string{"a@b.test"}, "subject": "Hi\r\nBcc: x@evil.test", "body": "x"})
		require.Equal(t, http.StatusAccepted, res.Status, "line breaks in a subject are squeezed out")
		var e api.Email
		res.JSON(&e)
		assert.Equal(t, "Hi Bcc: x@evil.test", e.Subject)
	})

	t.Run("a merged duplicate's emails follow it", func(t *testing.T) {
		keep := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"})
		dup := a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei", "lastName": "Dup"})
		email := map[string]any{"to": []string{"wei." + tenant.ID + "@example.test"}, "subject": "x", "body": "x", "candidateId": dup.Id}
		require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/emails", recruiter.Token, email).Status)
		res := a.request(http.MethodPost, "/v1/candidates/"+keep.Id+"/merge", recruiter.Token, map[string]any{"duplicateId": dup.Id, "version": keep.Version})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Len(t, a.emails(recruiter.Token, keep.Id), 1)
	})
}

func TestEmailIsRetriedWhileTheMailServerIsBusy(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	cand := a.createCandidate(token, map[string]any{"firstName": "Siti"})
	busy := map[string]any{"to": []string{"busy." + tenant.ID + "@example.test"}, "subject": "x", "body": "x", "candidateId": cand.Id}
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/emails", token, busy).Status)

	ctx := context.Background()
	// attempt runs the tenant's due jobs now, allowing two attempts in all.
	attempt := func() {
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now(), max_attempts = 2 WHERE tenant_id = $1 AND status = 'pending'`, tenant.ID)
		require.NoError(t, err)
		w, err := app.NewWorker(a.deps, a.modules)
		require.NoError(t, err)
		w.Tenant = tenant.ID
		for range 1000 {
			_, err := w.RunOnce(ctx)
			require.NoError(t, err)
			var due int
			require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1
				AND (status = 'running' OR (status = 'pending' AND run_after <= now()))`, tenant.ID).Scan(&due))
			if due == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("the job did not run")
	}
	attempt()
	assert.Equal(t, api.EmailStatusQueued, a.emails(token, cand.Id)[0].Status, "a busy server is tried again later")
	attempt()
	e := a.emails(token, cand.Id)[0]
	assert.Equal(t, api.EmailStatusFailed, e.Status, "the last attempt gives up, with the reason")
	reason, _ := e.Error.Get()
	assert.Contains(t, reason, "451")
}
