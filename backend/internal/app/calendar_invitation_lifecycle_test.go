package app_test

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// interviewLockFree says whether an edit to the interview could take its
// calendar lock right now (the lock a push or a reminder must not hold while
// it waits on Microsoft or the mail server).
func interviewLockFree(t *testing.T, tenant, interview string) bool {
	var free bool
	if err := testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT pg_try_advisory_xact_lock(hashtextextended($1 || ':calendar-source:interview:' || $2, 0))", tenant, interview).Scan(&free); err != nil {
		t.Error(err)
	}
	return free
}

func TestOutlookInterviewIsNotLockedWhileOutlookOrTheMailServerAnswers(t *testing.T) {
	t.Parallel()
	c := startCandidateFollowUp(t)
	var mu sync.Mutex
	var seen []bool
	observe := func() {
		free := interviewLockFree(t, c.tenant, c.iv.Id)
		mu.Lock()
		seen = append(seen, free)
		mu.Unlock()
	}
	c.f.mu.Lock()
	c.f.onWrite = observe
	c.f.mu.Unlock()
	res := c.a.request(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "title": "Panel interview"})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	mailServer.OnMessage(c.email, observe)
	c.deliver()

	require.Len(t, mailServer.To(c.email), 1)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 2, "one change sent to Outlook, one reminder sent")
	assert.True(t, seen[0], "an interview edit does not wait on Outlook's answer")
	assert.True(t, seen[1], "an interview edit does not wait on the mail server")
}

func TestOutlookInterviewConflictIsMarkedOnce(t *testing.T) {
	t.Parallel()
	c := startCandidateFollowUp(t)
	c.f.mu.Lock()
	for _, event := range c.f.events {
		event["attendees"] = []any{} // the candidate was taken off the meeting in Outlook
		event["changeKey"] = "removed"
	}
	c.f.mu.Unlock()
	refreshOutlook(t, c.a, c.user, c.tenant)
	stamp := func() (status, xmin string) {
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
			"SELECT sync_status,xmin::text FROM calendar_event WHERE tenant_id=$1 AND source='interview_invitation'", c.tenant).Scan(&status, &xmin))
		return
	}
	status, before := stamp()
	require.Equal(t, "conflict", status)
	refreshOutlook(t, c.a, c.user, c.tenant)
	status, after := stamp()
	assert.Equal(t, "conflict", status)
	assert.Equal(t, before, after, "a sync with nothing new in Outlook leaves the conflict as it was")
}

func TestDeletingACandidateCancelsTheirOutlookInvitation(t *testing.T) {
	t.Parallel()
	c := startCandidateFollowUp(t)
	c.f.mu.Lock()
	require.Len(t, c.f.events, 1)
	c.f.mu.Unlock()
	var admin string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"UPDATE app_user SET role=$2 WHERE id=$1 RETURNING id", c.userID, api.RoleAdmin).Scan(&admin))
	res := c.a.bin(c.user, map[string]any{"kind": "candidate", "id": c.candidate})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	purgeNow(t, c.a, c.tenant)
	runCoreJobsUntilIdle(t, c.a, c.tenant)

	c.f.mu.Lock()
	assert.Empty(t, c.f.events, "the candidate's Outlook invitation is cancelled")
	c.f.mu.Unlock()
	var rows, removed int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM calendar_event WHERE tenant_id=$1 AND source='interview_invitation'),
		(SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='interview.outlook_invitation_removed')`, c.tenant).Scan(&rows, &removed))
	assert.Zero(t, rows, "nothing is kept of the invitation, which held the candidate's address")
	assert.Equal(t, 1, removed)
}
