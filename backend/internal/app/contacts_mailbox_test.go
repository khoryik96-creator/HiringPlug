package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) companyContacts(token, companyID string) map[string]api.ContactSummary {
	a.t.Helper()
	var page api.ContactPage
	res := a.request(http.MethodGet, "/v1/contacts?companyId="+companyID, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&page)
	out := map[string]api.ContactSummary{}
	for _, c := range page.Items {
		out[c.FullName] = c
	}
	return out
}

func mailboxStatus(c api.ContactSummary) string {
	if c.MailboxStatus.IsNull() {
		return ""
	}
	return string(c.MailboxStatus.MustGet())
}

func TestVerifyingTheEmailsOfACompanysContacts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	fake := a.deps.Mailboxes.(*fakeMailboxes)

	company := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd"})
	other := a.createCompany(token, map[string]any{"name": "Other Bhd"})
	ana := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Ana", "email": "ana@maju.test"})
	gone := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Gone", "email": "gone@maju.test"})
	a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Busy", "email": "busy@maju.test"})
	noEmail := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Nobody"})
	a.createContact(token, map[string]any{"companyId": other.Id, "firstName": "Elsewhere", "email": "x@other.test"})

	// One contact without an email cannot be checked; a viewer cannot check.
	res := a.request(http.MethodPost, "/v1/contacts/"+noEmail.Id+"/mailbox-check", token, nil)
	assert.Equal(t, []string{"email"}, fieldNames(t, res.Error()))
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/contacts/mailbox-checks", tenant.Users[api.RoleViewer].Token,
		map[string]any{"companyId": company.Id}).Status)

	res = a.request(http.MethodPost, "/v1/contacts/mailbox-checks", token, map[string]any{"companyId": company.Id})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var started api.ContactMailboxChecksStarted
	res.JSON(&started)
	assert.Equal(t, api.ContactMailboxChecksStarted{Started: 3, Skipped: 0}, started, "contacts without an email are not listed")
	listed := a.companyContacts(token, company.Id)
	assert.Equal(t, "checking", mailboxStatus(listed["Ana"]))
	assert.Empty(t, mailboxStatus(listed["Nobody"]))

	// Asking again while they are being checked starts nothing twice.
	res = a.request(http.MethodPost, "/v1/contacts/mailbox-checks", token, map[string]any{"companyId": company.Id})
	res.JSON(&started)
	assert.Equal(t, api.ContactMailboxChecksStarted{Started: 0, Skipped: 3}, started)

	runCoreJobsUntilIdle(t, a, tenant.ID)
	listed = a.companyContacts(token, company.Id)
	assert.Equal(t, "accepted", mailboxStatus(listed["Ana"]))
	assert.Equal(t, "invalid", mailboxStatus(listed["Gone"]), "the mailbox does not exist: an email would bounce")
	assert.Equal(t, "unknown", mailboxStatus(listed["Busy"]))
	assert.Empty(t, fake.callsFor("x@other.test"), "only this company's contacts")

	check := a.contact(token, gone.Id).MailboxCheck.MustGet()
	assert.Equal(t, api.MailboxCheckStatusInvalid, check.Status)
	assert.Equal(t, "The mail server explicitly reported that this mailbox does not exist.", check.Reason.MustGet())
	assert.Equal(t, "Microsoft 365", check.Provider.MustGet())
	assert.False(t, check.CheckedAt.IsNull())

	// The inconclusive one is checked again on its own, two minutes later,
	// comparing the servers, about the same made-up mailbox.
	busy := a.contactByName(token, company.Id, "Busy")
	busyCheck := a.contact(token, busy).MailboxCheck.MustGet()
	require.False(t, busyCheck.RecheckAt.IsNull())
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'contact.check_mailbox' AND status = 'pending'`, tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	calls := fake.callsFor("busy@maju.test")
	require.Len(t, calls, 2, "one recheck only")
	assert.Equal(t, []string{mailcheck.Full, mailcheck.Deep}, []string{calls[0].Mode, calls[1].Mode})
	assert.Equal(t, calls[0].ControlID, calls[1].ControlID)
	assert.Regexp(t, `^[a-f0-9]{32}$`, calls[0].ControlID)
	assert.True(t, a.contact(token, busy).MailboxCheck.MustGet().RecheckAt.IsNull(), "no third check")

	// A new check of the same address keeps the made-up mailbox too.
	res = a.request(http.MethodPost, "/v1/contacts/"+ana.Id+"/mailbox-check", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	anaCalls := fake.callsFor("ana@maju.test")
	require.Len(t, anaCalls, 2)
	assert.Equal(t, anaCalls[0].ControlID, anaCalls[1].ControlID)

	// A changed email has no check until it is checked.
	res = a.request(http.MethodPatch, "/v1/contacts/"+gone.Id, token, map[string]any{"version": gone.Version, "email": "nur@maju.test"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.True(t, a.contact(token, gone.Id).MailboxCheck.IsNull())
	assert.Empty(t, mailboxStatus(a.companyContacts(token, company.Id)["Gone"]))

	// Another agency cannot check (or see) these contacts.
	stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/contacts/"+ana.Id+"/mailbox-check", stranger, nil).Status)
	assert.Equal(t, []string{"companyId"}, fieldNames(t, a.request(http.MethodPost, "/v1/contacts/mailbox-checks", stranger, map[string]any{"companyId": company.Id}).Error()))
}

func (a *testApp) contactByName(token, companyID, name string) string {
	a.t.Helper()
	c, ok := a.companyContacts(token, companyID)[name]
	require.True(a.t, ok, name)
	return c.Id
}
