package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Emails that would not reach a client contact (they left, or their mailbox
// does not exist): the composer is told, and automations do not send them.
func TestEmailsToAContactWhoLeftOrWhoseMailboxIsGone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	hr := "ana." + tenant.ID + "@maju.test"
	ana := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Ana", "lastName": "Lim", "email": hr})
	gone := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Gone", "email": "gone@maju.test"})
	fine := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Wei", "email": "wei@maju.test"})
	runCoreJobsUntilIdle(t, a, tenant.ID) // gone@ bounces (the fake mail servers)
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "contactId": ana.Id, "title": "Accountant"})
	res := a.request(http.MethodPatch, "/v1/contacts/"+ana.Id, recruiter, map[string]any{"version": ana.Version, "status": "left"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	// The composer asks before sending.
	res = a.request(http.MethodPost, "/v1/contacts/address-check", recruiter, map[string]any{"emails": []string{strings.ToUpper(hr), "gone@maju.test", fine.Email.MustGet(), "x@y.test"}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var problems api.ContactAddressProblems
	res.JSON(&problems)
	byContact := map[string]api.ContactAddressProblemProblem{}
	for _, p := range problems.Items {
		byContact[p.ContactId] = p.Problem
		assert.Equal(t, "Maju Foods Berhad", p.CompanyName)
	}
	assert.Equal(t, map[string]api.ContactAddressProblemProblem{ana.Id: api.ContactAddressProblemProblemLeft, gone.Id: api.ContactAddressProblemProblemBounces}, byContact)

	// An automation emailing the client: not sent; the recruiter gets a task.
	res = a.request(http.MethodPost, "/v1/automations", owner, map[string]any{"name": "Tell the client", "enabled": true,
		"trigger": "stage_changed", "stage": "submitted", "offsetDays": 0, "sendMode": "send",
		"actions": []any{map[string]any{"kind": "email_client", "subject": "A candidate for {{job.title}}", "body": "Please see the CV."}}})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "email": "siti." + tenant.ID + "@candidates.test"}).Id)
	require.Equal(t, http.StatusOK, a.move(recruiter, appl, map[string]any{"stage": "submitted"}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	assert.Empty(t, mailServer.To(hr), "not sent to someone who left")
	runs := a.automationRuns(owner, "?status=skipped")
	require.Len(t, runs, 1)
	assert.Contains(t, runs[0].Detail.MustGet(), "has left Maju Foods Berhad; added a task")
	var tasks api.TaskList
	res = a.request(http.MethodGet, "/v1/tasks", recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&tasks)
	require.Len(t, tasks.Items, 1)
	assert.Contains(t, tasks.Items[0].Title, "Not emailed: Ana Lim ("+hr+") has left Maju Foods Berhad. Update Maju Foods Berhad's contact for Accountant")
}
