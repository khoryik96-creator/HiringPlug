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
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// notifySetup is a job owned by a recruiter with: someone who applied
// through JobStreet, someone the recruiter added, an interview tomorrow
// with a hiring manager on the panel, an offer waiting for approval, and
// the recruiter's tasks (one overdue, one next week).
type notifySetup struct {
	tenant               testutil.Tenant
	job                  api.JobOrder
	applied, added       api.Application
	interview            api.Interview
	recruiterID, manager string
}

func newNotifySetup(t *testing.T, a *testApp) notifySetup {
	t.Helper()
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleHiringManager, api.RoleViewer)
	recruiter, manager := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	applied := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aminah"}).Id)
	added := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei", "lastName": "Jie"}).Id)
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE application SET channel = 'jobstreet' WHERE id = $1`, applied.Id)
	require.NoError(t, err)

	y, m, d := time.Now().In(kualaLumpur(t)).Date()
	tomorrow := time.Date(y, m, d+1, 10, 0, 0, 0, kualaLumpur(t))
	iv := a.interview(http.MethodPost, "/v1/applications/"+applied.Id+"/interviews", recruiter.Token, map[string]any{
		"title": "First interview", "startsAt": tomorrow, "durationMinutes": 60, "mode": "phone", "interviewerIds": []string{manager.ID},
	}, http.StatusCreated)
	a.interview(http.MethodPost, "/v1/applications/"+added.Id+"/interviews", recruiter.Token, map[string]any{
		"title": "Later interview", "startsAt": tomorrow.AddDate(0, 0, 3), "durationMinutes": 60, "mode": "phone", "interviewerIds": []string{manager.ID},
	}, http.StatusCreated)

	o := a.offer(http.MethodPost, "/v1/applications/"+added.Id+"/offers", recruiter.Token, map[string]any{}, http.StatusCreated)
	o = a.offer(http.MethodPatch, "/v1/offers/"+o.Id, recruiter.Token, map[string]any{"version": o.Version,
		"salary": map[string]any{"amount": "9123.00", "currency": "MYR", "period": "month"}, "startDate": "2027-01-04"}, http.StatusOK)
	a.offer(http.MethodPost, "/v1/offers/"+o.Id+"/steps", recruiter.Token, map[string]any{"version": o.Version, "step": "request_approval"}, http.StatusOK)

	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	_, err = testutil.Current.Admin.Exec(ctx, `INSERT INTO recruiter_task (tenant_id, user_id, title, due_on, application_id) VALUES
		($1, $2, 'Call Wei back', $3, $4), ($1, $2, 'Plan next week', $5, NULL)`,
		tenant.ID, recruiter.ID, today.AddDate(0, 0, -2), added.Id, today.AddDate(0, 0, 7))
	require.NoError(t, err)
	return notifySetup{tenant: tenant, job: job, applied: applied, added: added, interview: iv, recruiterID: recruiter.ID, manager: manager.ID}
}

func (a *testApp) notifications(token string) api.NotificationList {
	var out api.NotificationList
	res := a.request(http.MethodGet, "/v1/notifications", token, nil)
	if res.Status == http.StatusOK {
		res.JSON(&out)
	}
	return out
}

func kinds(list api.NotificationList) map[api.NotificationKind][]string {
	out := map[api.NotificationKind][]string{}
	for _, it := range list.Items {
		out[it.Kind] = append(out[it.Kind], it.Title+" | "+it.Detail)
	}
	return out
}

func TestNotificationsShowWhatNeedsEachUser(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	recruiter, manager := s.tenant.Users[api.RoleRecruiter], s.tenant.Users[api.RoleHiringManager]

	list := a.notifications(recruiter.Token)
	got := kinds(list)
	assert.Equal(t, []string{"Siti Aminah applied | Accountant, Contoh Berhad"}, got[api.NotificationKindApplicant],
		"only who applied themselves, not who the recruiter added")
	require.Len(t, got[api.NotificationKindTask], 1, "the overdue task; next week's is not due")
	assert.True(t, strings.HasPrefix(got[api.NotificationKindTask][0], "Call Wei back | Overdue, due "), got[api.NotificationKindTask][0])
	assert.Contains(t, got[api.NotificationKindTask][0], "Wei Jie, Accountant")
	assert.Equal(t, []string{"Interview tomorrow, 10:00 AM: Siti Aminah | First interview · Accountant, Contoh Berhad"}, got[api.NotificationKindInterview],
		"the one they arranged, tomorrow in the agency's time; not the one in four days")
	assert.Empty(t, got[api.NotificationKindOfferApproval], "no one approves their own offer (and recruiters cannot approve)")
	assert.Equal(t, len(list.Items), list.Unseen, "never looked: everything is new")
	assert.False(t, list.MorningEmail)
	for _, it := range list.Items {
		assert.True(t, it.New)
		assert.True(t, strings.HasPrefix(it.Href, "#/"), it.Href)
	}

	mgr := kinds(a.notifications(manager.Token))
	assert.Equal(t, []string{"Offer for Wei Jie needs your approval | Accountant, Contoh Berhad"}, mgr[api.NotificationKindOfferApproval])
	assert.Len(t, mgr[api.NotificationKindInterview], 1, "an interviewer on the panel")
	assert.Empty(t, mgr[api.NotificationKindApplicant], "not their job")
	assert.Empty(t, mgr[api.NotificationKindTask], "tasks are each user's own")

	viewer := a.request(http.MethodGet, "/v1/notifications", s.tenant.Users[api.RoleViewer].Token, nil)
	require.Equal(t, http.StatusOK, viewer.Status, "everyone has a bell")
	assert.NotContains(t, string(viewer.Body), "Wei", "a viewer cannot approve offers and has no tasks or jobs")

	body := string(a.request(http.MethodGet, "/v1/notifications", s.tenant.Users[api.RoleOwner].Token, nil).Body)
	assert.Contains(t, body, "needs your approval", "owners approve offers")
	assert.NotContains(t, body, "9123", "never the salary")

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	assert.Empty(t, a.notifications(other.Users[api.RoleOwner].Token).Items, "companies never see each other's")
}

func TestNotificationsSeenMarksWhatIsNew(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	recruiter := s.tenant.Users[api.RoleRecruiter]
	ctx := context.Background()

	shown := a.notifications(recruiter.Token)
	// Someone applies after the list was worked out, before it is marked seen.
	time.Sleep(10 * time.Millisecond)
	meanwhile := a.addToJob(recruiter.Token, s.job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Aina"}).Id)
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE application SET channel = 'linkedin', created_at = now() WHERE id = $1`, meanwhile.Id)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/notifications/seen", recruiter.Token, map[string]any{"upTo": shown.AsOf}).Status)
	list := a.notifications(recruiter.Token)
	assert.Equal(t, 1, list.Unseen, "what came after the list shown stays new")
	assert.Equal(t, "Aina applied", list.Items[0].Title)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/notifications/seen", recruiter.Token, map[string]any{"upTo": list.AsOf}).Status)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/notifications/seen", recruiter.Token, map[string]any{"upTo": shown.AsOf}).Status,
		"an older tab marking seen later")
	list = a.notifications(recruiter.Token)
	assert.Zero(t, list.Unseen)
	require.NotEmpty(t, list.Items, "seen ones stay in the list")
	for _, it := range list.Items {
		assert.False(t, it.New, it.Title)
	}
	assert.Equal(t, 4, a.notifications(s.tenant.Users[api.RoleHiringManager].Token).Unseen+2,
		"looking marks only your own as seen (the manager still has 2 new)")

	time.Sleep(10 * time.Millisecond)
	later := a.addToJob(recruiter.Token, s.job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Nur"}).Id)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE application SET channel = 'careers_page', created_at = now() WHERE id = $1`, later.Id)
	require.NoError(t, err)
	list = a.notifications(recruiter.Token)
	assert.Equal(t, 1, list.Unseen, "someone applied since")
	assert.Equal(t, "Nur applied", list.Items[0].Title, "newest first")
	assert.True(t, list.Items[0].New)

	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE application SET binned = true WHERE id = $1`, later.Id)
	require.NoError(t, err)
	assert.Zero(t, a.notifications(recruiter.Token).Unseen, "an application in the Bin is not shown")

	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'notification.seen'`, s.tenant.ID).Scan(&audited))
	assert.Positive(t, audited, "marking seen is audited")
}

func TestTodaysTasksAreNotCutOffByOldOnes(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO recruiter_task (tenant_id, user_id, title, due_on)
		SELECT $1, $2, 'Old task ' || n, current_date - 30 - n FROM generate_series(1, 60) n`, s.tenant.ID, s.recruiterID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(context.Background(), `INSERT INTO recruiter_task (tenant_id, user_id, title, due_on)
		VALUES ($1, $2, 'Call Siti back', (now() AT TIME ZONE 'Asia/Kuala_Lumpur')::date)`, s.tenant.ID, s.recruiterID)
	require.NoError(t, err)
	assert.Contains(t, kinds(a.notifications(s.tenant.Users[api.RoleRecruiter].Token))[api.NotificationKindTask], "Call Siti back | Due today")
}

func digestJobs(t *testing.T, tenantID string) (pending int, runAfter time.Time) {
	t.Helper()
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT count(*), coalesce(max(run_after), 'epoch') FROM job WHERE tenant_id = $1 AND kind = 'notifications.digest' AND status = 'pending'`,
		tenantID).Scan(&pending, &runAfter))
	return pending, runAfter
}

func digestDueNow(t *testing.T, a *testApp, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'notifications.digest' AND status = 'pending'`, tenantID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenantID)
}

func TestMorningEmail(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	recruiter := s.tenant.Users[api.RoleRecruiter]
	email := "recruiter." + s.tenant.ID + "@test.local"
	set := func(token string, on bool) {
		t.Helper()
		res := a.request(http.MethodPut, "/v1/notifications/settings", token, map[string]any{"morningEmail": on, "mutedKinds": []string{}})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}

	set(recruiter.Token, true)
	assert.True(t, a.notifications(recruiter.Token).MorningEmail)
	n, at := digestJobs(t, s.tenant.ID)
	require.Equal(t, 1, n)
	local := at.In(kualaLumpur(t))
	assert.Equal(t, 8, local.Hour(), "8:00 in the agency's time")
	assert.True(t, at.After(time.Now()) && at.Before(time.Now().Add(24*time.Hour)), "the next 8:00")

	set(recruiter.Token, true)
	n, _ = digestJobs(t, s.tenant.ID)
	assert.Equal(t, 1, n, "already on: nothing more queued")
	set(recruiter.Token, false)
	set(recruiter.Token, true)
	digestDueNow(t, a, s.tenant.ID)
	msgs := mailServer.To(email)
	require.Len(t, msgs, 1, "off and on again: one email, not one per chain")
	assert.Contains(t, msgs[0].Data, "Today: 1 interview, 1 task due, 1 new applicant")
	assert.Contains(t, msgs[0].Data, "Interview tomorrow, 10:00 AM: Siti Aminah")
	assert.Contains(t, msgs[0].Data, "/#/candidates/")
	n, at = digestJobs(t, s.tenant.ID)
	assert.Equal(t, 1, n, "the next morning's is queued, and only one")
	assert.True(t, at.After(time.Now()), "for the next morning")

	// A retry of the same morning's job does not send it again.
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE job SET status = 'pending' WHERE tenant_id = $1 AND kind = 'notifications.digest'`, s.tenant.ID)
	require.NoError(t, err)
	digestDueNow(t, a, s.tenant.ID)
	assert.Len(t, mailServer.To(email), 1)

	// Turned off: the waiting job does nothing and queues no more.
	set(recruiter.Token, false)
	digestDueNow(t, a, s.tenant.ID)
	n, _ = digestJobs(t, s.tenant.ID)
	assert.Zero(t, n)

	// Nothing to say: no email (a viewer has no tasks, jobs or approvals).
	set(s.tenant.Users[api.RoleViewer].Token, true)
	digestDueNow(t, a, s.tenant.ID)
	assert.Empty(t, mailServer.To("viewer."+s.tenant.ID+"@test.local"))
	n, _ = digestJobs(t, s.tenant.ID)
	assert.Equal(t, 1, n, "the viewer's next morning is still queued")
}

func TestNotificationKindsCanBeTurnedOff(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	recruiter := s.tenant.Users[api.RoleRecruiter]
	put := func(body map[string]any) *response {
		return a.request(http.MethodPut, "/v1/notifications/settings", recruiter.Token, body)
	}

	res := put(map[string]any{"morningEmail": true, "mutedKinds": []string{"task", "applicant"}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.NotificationSettings
	res.JSON(&saved)
	assert.Equal(t, []api.NotificationKind{api.NotificationKindApplicant, api.NotificationKindTask}, saved.MutedKinds, "in Settings' order")

	list := a.notifications(recruiter.Token)
	assert.Equal(t, saved.MutedKinds, list.MutedKinds)
	got := kinds(list)
	assert.Empty(t, got[api.NotificationKindApplicant], "turned off")
	assert.Empty(t, got[api.NotificationKindTask], "turned off")
	assert.Len(t, got[api.NotificationKindInterview], 1, "still on")
	assert.Equal(t, 1, list.Unseen, "the count leaves out what is turned off")

	digestDueNow(t, a, s.tenant.ID)
	msgs := mailServer.To("recruiter." + s.tenant.ID + "@test.local")
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Data, "Today: 1 interview")
	assert.NotContains(t, msgs[0].Data, "Call Wei back", "nor in the morning email")
	assert.NotContains(t, msgs[0].Data, "applied")

	assert.Equal(t, http.StatusBadRequest, put(map[string]any{"morningEmail": true, "mutedKinds": []string{"birthdays"}}).Status)
	require.Equal(t, http.StatusOK, put(map[string]any{"morningEmail": true, "mutedKinds": []string{}}).Status)
	assert.Len(t, kinds(a.notifications(recruiter.Token))[api.NotificationKindTask], 1, "turned back on")
	n, _ := digestJobs(t, s.tenant.ID)
	assert.Equal(t, 1, n, "changing the kinds while the email stays on queues no second email")
}

func TestMorningEmailWaitsOutAnInactiveUserAndRestartsAStoppedChain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	s := newNotifySetup(t, a)
	ctx := context.Background()
	recruiter, owner := s.tenant.Users[api.RoleRecruiter], s.tenant.Users[api.RoleOwner]
	email := "recruiter." + s.tenant.ID + "@test.local"
	put := func(body map[string]any) {
		t.Helper()
		res := a.request(http.MethodPut, "/v1/notifications/settings", recruiter.Token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}
	put(map[string]any{"morningEmail": true, "mutedKinds": []string{}})

	// Deactivated for a while: mornings pass without an email, the chain goes on.
	require.Equal(t, http.StatusOK, a.request(http.MethodPatch, "/v1/team/"+recruiter.ID, owner.Token, map[string]any{"active": false}).Status)
	digestDueNow(t, a, s.tenant.ID)
	assert.Empty(t, mailServer.To(email))
	n, _ := digestJobs(t, s.tenant.ID)
	require.Equal(t, 1, n, "the next morning is still queued")
	require.Equal(t, http.StatusOK, a.request(http.MethodPatch, "/v1/team/"+recruiter.ID, owner.Token, map[string]any{"active": true}).Status)
	digestDueNow(t, a, s.tenant.ID)
	assert.Len(t, mailServer.To(email), 1, "back: the email comes back with them")

	// The chain's job failed for good: saving the settings starts a new one.
	// (The owner's: deactivating signed the recruiter out.)
	set := func(body map[string]any) {
		t.Helper()
		res := a.request(http.MethodPut, "/v1/notifications/settings", owner.Token, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	}
	pending := func() int {
		t.Helper()
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'notifications.digest'
			AND status = 'pending' AND payload->>'userId' = $2`, s.tenant.ID, owner.ID).Scan(&n))
		return n
	}
	set(map[string]any{"morningEmail": true, "mutedKinds": []string{}})
	require.Equal(t, 1, pending())
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'dead' WHERE tenant_id = $1 AND kind = 'notifications.digest' AND status = 'pending'`, s.tenant.ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE notification_state SET digest_due_at = now() - interval '1 day' WHERE user_id = $1`, owner.ID)
	require.NoError(t, err)
	set(map[string]any{"morningEmail": true, "mutedKinds": []string{"applicant"}})
	assert.Equal(t, 1, pending(), "a stopped chain starts again")
	set(map[string]any{"morningEmail": true, "mutedKinds": []string{}})
	assert.Equal(t, 1, pending(), "a running chain is not started twice")
}
