package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) automations(token string) api.AutomationList {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/automations", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var l api.AutomationList
	res.JSON(&l)
	return l
}

func (a *testApp) automationRuns(token, query string) []api.AutomationRun {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/automation-runs"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var l api.AutomationRunList
	res.JSON(&l)
	return l.Items
}

// switchOn turns a ready-made automation on, with how it sends.
func (a *testApp) switchOn(token string, au api.Automation, mode api.AutomationSendMode) api.Automation {
	a.t.Helper()
	body := map[string]any{"version": au.Version, "name": au.Name, "enabled": true, "trigger": au.Trigger, "offsetDays": au.OffsetDays,
		"actions": au.Actions, "sendMode": mode}
	if s, err := au.Stage.Get(); err == nil {
		body["stage"] = s
	}
	res := a.request(http.MethodPut, "/v1/automations/"+au.Id, token, body)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.Automation
	res.JSON(&out)
	return out
}

func preset(t *testing.T, l api.AutomationList, key string) api.Automation {
	t.Helper()
	for _, au := range l.Items {
		if au.Preset.IsSpecified() && !au.Preset.IsNull() && au.Preset.MustGet() == key {
			return au
		}
	}
	t.Fatalf("no ready-made %q", key)
	return api.Automation{}
}

// dueNow makes every waiting automation job of a tenant due.
func dueNow(t *testing.T, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'automation.run' AND status = 'pending'`, tenantID)
	require.NoError(t, err)
}

// Automations: WHEN · IF · THEN rules started by what happens, emailing from
// the recruiter's mailbox on their own or after approval, and giving tasks.
func TestAutomations(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner, recruiter, viewer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleViewer].Token

	list := a.automations(owner)
	require.Len(t, list.Items, 7, "the ready-made automations")
	for _, au := range list.Items {
		assert.False(t, au.Enabled, "added switched off: %s", au.Name)
		assert.Equal(t, api.AutomationSendModeApprove, au.SendMode, "asking first, the default")
	}
	assert.Len(t, a.automations(owner).Items, 7, "added once")
	names := make([]string, len(list.Items))
	for i, au := range list.Items {
		names[i] = au.Name
	}
	assert.Equal(t, []string{"Application received", "Interview reminder, the day before", "Not progressing, 2 days after",
		"Placement check-in, 30 days", "Placement check-in, 90 days", "Guarantee ending, a week before", "Invoice overdue"}, names, "in their own order")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/automations", viewer, nil).Status)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/automations", recruiter, map[string]any{}).Status, "only owners and admins change them")

	bad := a.request(http.MethodPost, "/v1/automations", owner, map[string]any{"name": "x", "enabled": true, "trigger": "stage_changed",
		"offsetDays": -1, "sendMode": "send", "actions": []any{map[string]any{"kind": "email_candidate", "subject": "Hi"}}})
	require.Equal(t, http.StatusBadRequest, bad.Status)
	assert.ElementsMatch(t, []string{"stage", "offsetDays", "actions.0.body"}, fieldNames(t, bad.Error()))

	res := a.request(http.MethodPost, "/v1/automations", owner, map[string]any{"name": "Invite to interview", "enabled": true,
		"trigger": "stage_changed", "stage": "interview", "offsetDays": 0, "sendMode": "send",
		"actions": []any{map[string]any{"kind": "email_candidate", "subject": "Interview: {{job.title}}", "body": "Hi {{candidate.firstName}}, let us talk."}}})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	a.switchOn(owner, preset(t, list, "application_received"), api.AutomationSendModeApprove)
	a.switchOn(owner, preset(t, list, "guarantee_ending"), api.AutomationSendModeApprove)
	a.switchOn(owner, preset(t, list, "not_progressing"), api.AutomationSendModeSend)

	company := a.createCompany(recruiter, map[string]any{"name": "Maju Foods Berhad"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Accountant"})
	siti := "siti." + tenant.ID + "@candidates.test"
	appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": siti}).Id)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	t.Run("an email waits for approval, then goes from the approver", func(t *testing.T) {
		waiting := a.automationRuns(recruiter, "?status=to_approve")
		require.Len(t, waiting, 1)
		assert.Equal(t, "Application received", waiting[0].AutomationName)
		assert.Equal(t, "Siti Aisyah", waiting[0].CandidateName.MustGet())
		assert.Empty(t, mailServer.To(siti), "nothing sent yet")
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/automation-runs/"+waiting[0].Id+"/send", viewer, nil).Status)

		res := a.request(http.MethodPost, "/v1/automation-runs/"+waiting[0].Id+"/send", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var run api.AutomationRun
		res.JSON(&run)
		assert.Equal(t, api.AutomationRunStatusDone, run.Status)
		assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, "/v1/automation-runs/"+run.Id+"/send", recruiter, nil).Status, "only once")
		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To(siti)
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].Data, "Thank you for applying: Accountant")
	})

	t.Run("an email that sends on its own", func(t *testing.T) {
		res := a.move(recruiter, appl, map[string]any{"stage": "interview"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To(siti)
		require.Len(t, msgs, 2)
		assert.Contains(t, msgs[1].Data, "Interview: Accountant")
		assert.Contains(t, msgs[1].Data, "Hi Siti, let us talk.")
		done := a.automationRuns(owner, "?status=done")
		require.Len(t, done, 2)
		assert.Contains(t, done[0].Detail.MustGet(), "Emailed from Test recruiter")
	})

	t.Run("a run that no longer applies is skipped", func(t *testing.T) {
		wei := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Wei", "email": "wei." + tenant.ID + "@candidates.test"}).Id)
		res := a.move(recruiter, wei, map[string]any{"stage": "rejected", "reason": "failed_interview"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&wei)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		scheduled := a.automationRuns(owner, "?status=scheduled")
		require.Len(t, scheduled, 1, "two days after the rejection")
		assert.WithinDuration(t, time.Now().AddDate(0, 0, 2), scheduled[0].DueAt, time.Minute)
		require.Equal(t, http.StatusOK, a.move(recruiter, wei, map[string]any{"stage": "screening"}).Status)
		dueNow(t, tenant.ID)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		var skipped *api.AutomationRun
		for _, r := range a.automationRuns(owner, "?status=skipped") {
			if r.Id == scheduled[0].Id {
				skipped = &r
			}
		}
		require.NotNil(t, skipped)
		assert.Equal(t, "The candidate is no longer at rejected.", skipped.Detail.MustGet())
		assert.Empty(t, mailServer.To("wei."+tenant.ID+"@candidates.test"))
	})

	t.Run("a task for the recruiter, only theirs to see", func(t *testing.T) {
		a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", recruiter, map[string]any{"startDate": time.Now().Format(time.DateOnly), "guaranteeDays": 30}, http.StatusCreated)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		dueNow(t, tenant.ID) // a week before the guarantee ends
		runCoreJobsUntilIdle(t, a, tenant.ID)
		var tasks api.TaskList
		a.request(http.MethodGet, "/v1/tasks", recruiter, nil).JSON(&tasks)
		require.Len(t, tasks.Items, 1)
		assert.Equal(t, "Guarantee ends in a week: check in with Siti Aisyah and Maju Foods Berhad", tasks.Items[0].Title)
		assert.Equal(t, "Guarantee ending, a week before", tasks.Items[0].AutomationName.MustGet())
		var others api.TaskList
		a.request(http.MethodGet, "/v1/tasks", owner, nil).JSON(&others)
		assert.Empty(t, others.Items)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPatch, "/v1/tasks/"+tasks.Items[0].Id, owner, map[string]any{"done": true}).Status)
		res := a.request(http.MethodPatch, "/v1/tasks/"+tasks.Items[0].Id, recruiter, map[string]any{"done": true})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var done api.Task
		res.JSON(&done)
		assert.False(t, done.DoneAt.IsNull())
	})

	t.Run("switched off: what is scheduled is skipped", func(t *testing.T) {
		l := a.automations(owner)
		np := preset(t, l, "not_progressing")
		body := map[string]any{"version": np.Version, "name": np.Name, "enabled": false, "trigger": np.Trigger, "stage": "rejected",
			"offsetDays": np.OffsetDays, "actions": np.Actions, "sendMode": np.SendMode}
		require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/automations/"+np.Id, owner, body).Status)
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, "/v1/automations/"+np.Id, owner, body).Status, "a stale version")
		lee := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Lee", "email": "lee." + tenant.ID + "@candidates.test"}).Id)
		require.Equal(t, http.StatusOK, a.move(recruiter, lee, map[string]any{"stage": "rejected", "reason": "failed_interview"}).Status)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		for _, r := range a.automationRuns(owner, "?status=scheduled") {
			assert.NotEqual(t, np.Id, r.AutomationId, "a switched-off automation schedules nothing")
		}
	})

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	assert.Empty(t, a.automationRuns(other.Users[api.RoleOwner].Token, ""), "companies never see each other's")
}
