package app_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestEmailCandidatePackagesUseRecipientGreetingAndCheckEverySource(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	first := a.createCandidate(token, map[string]any{"firstName": "Package One"})
	second := a.createCandidate(token, map[string]any{"firstName": "Package Two"})
	recipient := a.createCandidate(token, map[string]any{"firstName": "Recipient", "email": "recipient@example.test"})
	company := a.createCompany(token, map[string]any{"name": "Synthetic Package Client"})
	contact := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Client", "lastName": "Contact", "email": "client@example.test"})
	for i, c := range []api.Candidate{first, second} {
		r := a.request("PUT", "/v1/candidates/"+c.Id+"/compensation", token, map[string]any{"expectedVersion": 0, "currentSalary": nil, "expectedSalary": map[string]any{"min": fmt.Sprint((i + 1) * 11000), "currency": "MYR", "period": "month"}, "salaryNotes": fmt.Sprintf("Package %d notes", i+1)})
		require.Equal(t, 200, r.Status, "%s", r.Body)
	}
	list := []map[string]any{}
	for _, c := range []api.Candidate{first, second} {
		r := a.request("POST", "/v1/email-previews", token, map[string]any{"candidateId": c.Id, "subject": "", "bodyHtml": "", "timeZone": "Asia/Kuala_Lumpur"})
		require.Equal(t, 200, r.Status)
		var preview api.EmailPreview
		r.JSON(&preview)
		require.NotNil(t, preview.CandidateVersion)
		require.NotNil(t, preview.CompensationVersion)
		list = append(list, map[string]any{"candidateId": c.Id, "candidateVersion": *preview.CandidateVersion, "compensationVersion": *preview.CompensationVersion})
	}
	body := map[string]any{"contactId": contact.Id, "candidates": list, "to": []string{"client@example.test"}, "subject": "Selected candidates for [Client contact]", "bodyHtml": "<p>Dear [First name]</p><p>Package One and Package Two</p>", "timeZone": "Asia/Kuala_Lumpur"}
	r := a.request("POST", "/v1/emails", token, body)
	require.Equal(t, 202, r.Status, "%s", r.Body)
	var sent api.Email
	r.JSON(&sent)
	assert.Contains(t, sent.Body, "Dear Client")
	assert.Equal(t, contact.Id, sent.ContactId.MustGet())
	for _, c := range []api.Candidate{first, second} {
		r = a.request("GET", "/v1/emails?candidateId="+c.Id, token, nil)
		assert.Contains(t, string(r.Body), sent.Id)
	}
	r = a.request("GET", "/v1/emails?contactId="+contact.Id, token, nil)
	assert.Contains(t, string(r.Body), sent.Id)
	delete(body, "contactId")
	body["recipientCandidateId"] = recipient.Id
	body["subject"] = "Candidate packages"
	r = a.request("POST", "/v1/emails", token, body)
	require.Equal(t, 202, r.Status, "%s", r.Body)
	r.JSON(&sent)
	assert.Contains(t, sent.Body, "Dear Recipient")
	assert.Equal(t, recipient.Id, sent.CandidateId.MustGet())
	// A salary change on the second candidate cannot silently use the first candidate's version.
	r = a.request("PUT", "/v1/candidates/"+second.Id+"/compensation", token, map[string]any{"expectedVersion": 1, "currentSalary": nil, "expectedSalary": map[string]any{"min": "23000", "currency": "MYR", "period": "month"}})
	require.Equal(t, 200, r.Status)
	r = a.request("POST", "/v1/emails", token, body)
	assert.Equal(t, 409, r.Status, "%s", r.Body)
	// Each selected candidate must be visible, even when no CV is attached.
	foreignToken := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	foreign := a.createCandidate(foreignToken, map[string]any{"firstName": "Foreign package"})
	list[1] = map[string]any{"candidateId": foreign.Id}
	body["candidates"] = list
	r = a.request("POST", "/v1/emails", token, body)
	assert.Equal(t, 404, r.Status, "%s", r.Body)
}
