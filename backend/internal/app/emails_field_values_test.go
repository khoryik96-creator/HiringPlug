package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The composer puts a field's value in the message at once ("[Expected
// salary]" -> "RM 12,000 per month"), where it can be edited; the edited
// value is sent as typed, the candidate's record is untouched, and the email
// still counts as showing salary.
func TestComposerFieldValues(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	client := "hr." + tenant.ID + "@client.test"
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	res := a.request(http.MethodPut, "/v1/candidates/"+cand.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 0, "currentSalary": nil, "expectedSalary": map[string]any{"min": "12000", "max": nil, "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	preview := func(token string) api.EmailPreview {
		var p api.EmailPreview
		res := a.request(http.MethodPost, "/v1/email-previews", token, map[string]any{"subject": "", "bodyHtml": "", "candidateId": cand.Id, "timeZone": "Asia/Kuala_Lumpur"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&p)
		return p
	}
	t.Run("the values come with a preview, salary only for roles that may see it", func(t *testing.T) {
		values := preview(recruiter).FieldValues
		assert.Equal(t, "RM 12,000 per month", values["[Expected salary]"])
		assert.Equal(t, "Siti Aisyah", values["[Full name]"])
		assert.NotContains(t, values, "[Current salary]", "no value, not offered")
		assert.NotContains(t, preview(sourcer).FieldValues, "[Expected salary]")
	})

	t.Run("an edited value is sent as typed, counts as salary, and leaves the record alone", func(t *testing.T) {
		body := `<p>[Full name] expects <span data-field="candidate.expectedSalary">RM 12,500 per month (negotiable)</span>.</p>`
		res := a.request(http.MethodPost, "/v1/emails", recruiter, map[string]any{
			"to": []string{client}, "subject": "Siti", "bodyHtml": body, "candidateId": cand.Id,
		})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var sent api.Email
		res.JSON(&sent)
		assert.Equal(t, `<p>Siti Aisyah expects <span>RM 12,500 per month (negotiable)</span>.</p>`, sent.BodyHtml.MustGet(), "the mark is not sent")
		assert.True(t, sent.Salary)
		assert.Empty(t, a.emails(tenant.Users[api.RoleHiringManager].Token, cand.Id), "hidden from roles that may not see salaries")
		assert.Equal(t, "RM 12,000 per month", preview(recruiter).FieldValues["[Expected salary]"], "the record is unchanged")
	})

	t.Run("a template keeps the field, never the candidate's value", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/email-templates", recruiter, map[string]any{
			"name": "Submission", "subject": "[Full name]",
			"bodyHtml": `<p>Expecting <b><span data-field="candidate.expectedSalary">RM 12,000 per month</span></b>; <span data-field="candidate.fullName">Siti</span>.</p>`,
		})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var tpl api.EmailTemplate
		res.JSON(&tpl)
		assert.Equal(t, `<p>Expecting <b>[Expected salary]</b>; [Full name].</p>`, tpl.BodyHtml.MustGet())
	})
}
