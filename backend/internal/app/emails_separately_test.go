package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// One email to several people, each their own copy: to them alone, with
// their own fields, kept on their emails; those who cannot be sent to are
// left out and say why.
func TestSendingOneEmailToEachPersonSeparately(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	addr := func(who string) string { return who + "." + tenant.ID + "@example.test" }
	siti := a.createCandidate(token, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": addr("siti")})
	wei := a.createCandidate(token, map[string]any{"firstName": "Wei", "lastName": "Tan", "email": addr("wei")})
	noEmail := a.createCandidate(token, map[string]any{"firstName": "Raj", "lastName": "Kumar"})
	gone := a.createCandidate(token, map[string]any{"firstName": "Aina", "lastName": "Yusof", "email": addr("aina")})
	a.setConsent(token, gone.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	people := []map[string]any{{"candidateId": siti.Id}, {"candidateId": wei.Id}, {"candidateId": noEmail.Id}, {"candidateId": gone.Id}}
	var upload api.EmailAttachment
	a.upload("/v1/email-attachments", token, "", "Hub Operations Manager JD.pdf", minimalPDF).JSON(&upload)

	send := func(body map[string]any) api.EmailBatchResult {
		t.Helper()
		res := a.request(http.MethodPost, "/v1/email-batches", token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var out api.EmailBatchResult
		res.JSON(&out)
		return out
	}
	problems := func(r api.EmailBatchResult) []string {
		var out []string
		for _, x := range r.Recipients {
			problem := "ok"
			if !x.Problem.IsNull() {
				problem = x.Problem.MustGet()
			}
			out = append(out, x.Name+": "+problem)
		}
		return out
	}
	message := map[string]any{"subject": "A role for you, [First name]", "bodyHtml": "<p>Hi [First name],</p><p>A new role came in.</p>"}

	t.Run("a check says who would get it, and sends nothing", func(t *testing.T) {
		r := send(map[string]any{"recipients": people, "subject": message["subject"], "bodyHtml": message["bodyHtml"], "checkOnly": true})
		assert.Equal(t, 0, r.Sent)
		assert.Equal(t, []string{"Siti Aisyah: ok", "Wei Tan: ok", "Raj Kumar: has no email address", "Aina Yusof: withdrew consent to be contacted"}, problems(r))
		assert.Equal(t, addr("siti"), r.Recipients[0].To.MustGet())
		assert.Empty(t, a.emails(token, siti.Id))
	})

	t.Run("each gets their own, to them alone, with their own name and the file", func(t *testing.T) {
		r := send(map[string]any{"recipients": people, "subject": message["subject"], "bodyHtml": message["bodyHtml"], "attachmentIds": []string{upload.Id}})
		assert.Equal(t, 2, r.Sent)
		for _, c := range []struct {
			cand  api.Candidate
			first string
		}{{siti, "Siti"}, {wei, "Wei"}} {
			emails := a.emails(token, c.cand.Id)
			require.Len(t, emails, 1)
			e := emails[0]
			assert.Equal(t, []string{addr(map[string]string{"Siti": "siti", "Wei": "wei"}[c.first])}, e.To, "to them alone")
			assert.Empty(t, e.Cc)
			assert.Empty(t, e.Bcc)
			assert.Equal(t, "A role for you, "+c.first, e.Subject)
			assert.Contains(t, e.Body, "Hi "+c.first+",")
			require.Len(t, e.Attachments, 1)
			assert.Equal(t, "Hub Operations Manager JD.pdf", e.Attachments[0].FileName)
		}
		runCoreJobsUntilIdle(t, a, tenant.ID)
		for _, who := range []string{"siti", "wei"} {
			msgs := mailServer.To(addr(who))
			require.Len(t, msgs, 1, who)
			assert.NotContains(t, msgs[0].Data, map[string]string{"siti": addr("wei"), "wei": addr("siti")}[who], "nobody sees the others")
			assert.Contains(t, msgs[0].Data, "Hub Operations Manager JD.pdf")
		}
		assert.Empty(t, mailServer.To(addr("aina")))
	})

	t.Run("a field someone has no value for leaves them out, naming it", func(t *testing.T) {
		r := send(map[string]any{"recipients": people[:2], "subject": "Hi", "bodyHtml": "<p>[Current title]</p>", "checkOnly": true})
		assert.Equal(t, []string{"Siti Aisyah: no value for [Current title]", "Wei Tan: no value for [Current title]"}, problems(r))
	})

	t.Run("to client contacts too, each with their own name", func(t *testing.T) {
		company := a.createCompany(token, map[string]any{"name": "Maju Logistik Sdn Bhd"})
		mei := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Mei", "lastName": "Tan", "email": addr("mei")})
		ali := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Ali", "lastName": "Hassan", "email": addr("ali")})
		r := send(map[string]any{"recipients": []map[string]any{{"contactId": mei.Id}, {"contactId": ali.Id}}, "subject": "Hello [First name]", "body": "Season's greetings from us."})
		assert.Equal(t, 2, r.Sent)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		require.Len(t, mailServer.To(addr("mei")), 1)
		assert.Contains(t, mailServer.To(addr("ali"))[0].Data, "Hello Ali")
	})

	t.Run("refused: nobody, someone twice, neither or both, viewers", func(t *testing.T) {
		bad := func(recipients any) int {
			return a.request(http.MethodPost, "/v1/email-batches", token, map[string]any{"recipients": recipients, "subject": "Hi", "body": "x"}).Status
		}
		assert.Equal(t, http.StatusBadRequest, bad([]map[string]any{}))
		assert.Equal(t, http.StatusBadRequest, bad([]map[string]any{{"candidateId": siti.Id}, {"candidateId": siti.Id}}))
		assert.Equal(t, http.StatusBadRequest, bad([]map[string]any{{}}))
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/email-batches", tenant.Users[api.RoleViewer].Token,
			map[string]any{"recipients": people[:1], "subject": "Hi", "body": "x"}).Status)
	})
}
