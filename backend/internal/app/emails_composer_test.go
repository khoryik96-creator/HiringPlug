package app_test

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	stdmail "net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

var bannerPNG = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

// partsOf lists a message's parts, depth first, as content type and file name.
func partsOf(t *testing.T, data string) []string {
	t.Helper()
	msg, err := stdmail.ReadMessage(strings.NewReader(data))
	require.NoError(t, err)
	var out []string
	var walk func(ctype string, body io.Reader)
	walk = func(ctype string, body io.Reader) {
		media, params, err := mime.ParseMediaType(ctype)
		require.NoError(t, err)
		if !strings.HasPrefix(media, "multipart/") {
			return
		}
		r := multipart.NewReader(body, params["boundary"])
		for {
			p, err := r.NextPart()
			if err != nil {
				return
			}
			out = append(out, strings.Split(p.Header.Get("Content-Type"), ";")[0]+" "+p.FileName())
			walk(p.Header.Get("Content-Type"), p)
		}
	}
	walk(msg.Header.Get("Content-Type"), msg.Body)
	return out
}

func TestSendingACandidateToTheClient(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	client := "hr." + tenant.ID + "@client.test" // tests share the mail server: their own addresses
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	hr := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Mei", "lastName": "Tan", "email": client})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "contactId": hr.Id, "title": "Finance Manager"})
	cand := a.createCandidate(recruiter, map[string]any{
		"firstName": "Siti", "lastName": "Aisyah", "nationality": "MY", "noticePeriod": map[string]any{"value": 1, "unit": "month"},
		"workAuthorization": map[string]any{"status": "expat_eligible", "basis": "rp_t"},
	})
	res := a.request(http.MethodPut, "/v1/candidates/"+cand.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0,
		"currentSalary":   map[string]any{"amount": "9500", "currency": "MYR", "period": "month"},
		"expectedSalary":  map[string]any{"min": "12000", "max": nil, "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	appl := a.addToJob(recruiter, job.Id, cand.Id)
	cv := a.upload("/v1/candidates/"+cand.Id+"/documents", recruiter, "original", "Siti Aisyah CV.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, cv.Status, "%s", cv.Body)
	var doc api.Document
	cv.JSON(&doc)

	var banner api.EmailBanner
	res = a.upload("/v1/email-banners", recruiter, "", "contoh-banner.png", bannerPNG)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&banner)
	assert.Equal(t, "contoh-banner", banner.Name)
	assert.Equal(t, http.StatusForbidden, a.upload("/v1/email-banners", sourcer, "", "b.png", bannerPNG).Status)
	assert.Equal(t, http.StatusBadRequest, a.upload("/v1/email-banners", recruiter, "", "b.png", []byte("not an image")).Status)

	var upload api.EmailAttachment
	res = a.upload("/v1/email-attachments", recruiter, "", "Job description.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&upload)

	body := `<img data-banner-id="` + banner.Id + `" alt="banner"><p>Dear [Client contact],</p>` +
		`<p>Please find <b>[Full name]</b> for [Job title]: current [Current salary], expecting [Expected salary], ` +
		`notice [Notice period], [Expat status], [Nationality].</p><script>alert(1)</script>`
	t.Run("a preview fills the fields as sending will", func(t *testing.T) {
		var p api.EmailPreview
		res := a.request(http.MethodPost, "/v1/email-previews", recruiter, map[string]any{
			"subject": "[Full name] for [Job title]", "bodyHtml": body, "applicationId": appl.Id, "timeZone": "Asia/Kuala_Lumpur",
		})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&p)
		assert.Equal(t, "Siti Aisyah for Finance Manager", p.Subject)
		assert.Contains(t, p.BodyHtml, "current RM 9,500 per month, expecting RM 12,000 per month, notice 1 month, Expat (RP-T), Malaysia.")
		assert.Contains(t, p.BodyHtml, "Dear Mei Tan,")
		assert.NotContains(t, p.BodyHtml, "script")
		assert.Empty(t, p.Missing)
		assert.True(t, p.Salary)
		assert.Equal(t, client, p.ClientEmail.MustGet(), "the job's hiring contact, to send to")

		res = a.request(http.MethodPost, "/v1/email-previews", sourcer, map[string]any{
			"subject": "x", "bodyHtml": "<p>[Expected salary]</p>", "applicationId": appl.Id, "timeZone": "Asia/Kuala_Lumpur",
		})
		res.JSON(&p)
		assert.Equal(t, []string{"[Expected salary]"}, p.Missing, "salary fills only for roles that may see salaries")
	})

	var sent api.Email
	t.Run("sending fills the fields, attaches the files and shows the banner", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "cc": []string{"lead." + tenant.ID + "@agency.test"}, "bcc": []string{"me." + tenant.ID + "@agency.test"},
			"subject": "[Full name] for [Job title]", "bodyHtml": body, "applicationId": appl.Id,
			"attachmentIds": []string{upload.Id}, "cvs": []map[string]any{{"documentId": doc.Id}},
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&sent)
		assert.Equal(t, "Siti Aisyah for Finance Manager", sent.Subject)
		assert.True(t, sent.Salary)
		assert.Equal(t, []string{"me." + tenant.ID + "@agency.test"}, sent.Bcc)
		require.Len(t, sent.Attachments, 2)
		assert.Contains(t, sent.Body, "Dear Mei Tan,", "the plain-text version")
		assert.NotContains(t, sent.BodyHtml.MustGet(), "script")

		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To(client)
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].To, "me."+tenant.ID+"@agency.test", "bcc gets it")
		assert.NotContains(t, msgs[0].Data, "me."+tenant.ID, "but is not listed in it")
		assert.Equal(t, []string{
			"multipart/related ", "multipart/alternative ", "text/plain ", "text/html ", "image/png banner.png",
			"application/pdf Job description.pdf", "application/pdf Siti Aisyah CV.pdf",
		}, partsOf(t, msgs[0].Data))
		assert.Contains(t, msgs[0].Data, "cid:"+banner.Id)
		assert.Equal(t, api.EmailStatusSent, a.emails(recruiter, cand.Id)[0].Status)
	})

	t.Run("an email that shows salary is hidden from roles that may not see salaries, unless the company says so", func(t *testing.T) {
		assert.Len(t, a.emails(recruiter, cand.Id), 1)
		assert.Empty(t, a.emails(tenant.Users[api.RoleHiringManager].Token, cand.Id))

		owner := tenant.Users[api.RoleOwner].Token
		res := a.request(http.MethodPut, "/v1/settings/policies", owner, map[string]any{"emailSalaryVisibility": "everyone"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/settings/policies", recruiter, map[string]any{"emailSalaryVisibility": "salary_roles"}).Status)
		res = a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "subject": "Update", "bodyHtml": "<p>[Full name] now expects [Expected salary].</p>", "applicationId": appl.Id,
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		seen := a.emails(tenant.Users[api.RoleHiringManager].Token, cand.Id)
		require.Len(t, seen, 1, "the new one, sent after the change")
		assert.Equal(t, "Update", seen[0].Subject)
	})

	t.Run("a field with no value, or salary without permission, refuses the email", func(t *testing.T) {
		bare := a.createCandidate(recruiter, map[string]any{"firstName": "Ali"})
		res := a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "subject": "x", "bodyHtml": "<p>[Notice period]</p>", "candidateId": bare.Id,
		})
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.Contains(t, string(res.Body), "[Notice period]")
		res = a.request(http.MethodPost, "/v1/emails", sourcer, map[string]any{
			"to": []string{client}, "subject": "x", "bodyHtml": "<p>[Current salary]</p>", "applicationId": appl.Id,
		})
		assert.Equal(t, http.StatusBadRequest, res.Status)
		res = a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "subject": "x", "bodyHtml": "<p>Hi</p>", "candidateId": bare.Id, "cvs": []map[string]any{{"documentId": doc.Id}},
		})
		assert.Equal(t, http.StatusBadRequest, res.Status, "only the candidate's own CVs")
		res = a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "subject": "x", "bodyHtml": "<p>Hi</p>", "attachmentIds": []string{upload.Id},
		})
		assert.Equal(t, http.StatusBadRequest, res.Status, "an upload goes with one email")
	})

	t.Run("formatted templates are saved and deleted", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/email-templates", recruiter, map[string]any{
			"name": "Client submission", "subject": "[Full name] for [Job title]", "bodyHtml": body + "<p>Regards,<br>[Your name]</p>",
		})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var tpl api.EmailTemplate
		res.JSON(&tpl)
		assert.Contains(t, tpl.BodyHtml.MustGet(), `data-banner-id="`+banner.Id+`"`, "with its banner")
		assert.NotContains(t, tpl.BodyHtml.MustGet(), "script")
		assert.Contains(t, tpl.Body, "Regards,\n[Your name]", "and a plain-text version")
		var list api.EmailTemplateList
		a.request(http.MethodGet, "/v1/email-templates", recruiter, nil).JSON(&list)
		assert.Equal(t, "[Expected salary]", list.Fields[4].Token)
		assert.True(t, list.Fields[4].Salary)
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/email-templates/"+tpl.Id, recruiter, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/email-templates/"+tpl.Id, recruiter, nil).Status)
	})
}
