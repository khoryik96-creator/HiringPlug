package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func moveInOutlook(start, end time.Time) func(e map[string]any) {
	return func(e map[string]any) {
		e["start"], e["end"] = outlookTime(start), outlookTime(end)
	}
}

func (c candidateFollowUp) syncedOK() bool {
	c.t.Helper()
	var status string
	require.NoError(c.t, testutil.Current.Admin.QueryRow(c.t.Context(),
		"SELECT status FROM outlook_availability WHERE tenant_id=$1 AND user_id=$2", c.tenant, c.userID).Scan(&status))
	return status == "ok"
}

func TestOutlookMovedInterviewMovesInHyPlugWithItsReminders(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{panel: true})
	patches := c.patches()
	oldStart := c.iv.StartsAt
	start := oldStart.Add(24 * time.Hour)
	end := start.Add(75 * time.Minute)
	c.editInOutlook(moveInOutlook(start, end))

	iv := c.interview()
	assert.True(t, start.Equal(iv.StartsAt), "the interview moves to Outlook's time")
	assert.Equal(t, 75, iv.DurationMinutes)
	assert.False(t, iv.OutlookChangedAt.IsNull())
	assert.EqualValues(t, "sent", c.invitation().Status, "moved in Outlook is not a conflict")
	assert.Equal(t, patches, c.patches(), "Outlook already told the candidate")

	var payloads []string
	rows, err := testutil.Current.Admin.Query(t.Context(), "SELECT payload::text FROM job WHERE tenant_id=$1 AND kind='interview.remind'", c.tenant)
	require.NoError(t, err)
	for rows.Next() {
		var p string
		require.NoError(t, rows.Scan(&p))
		payloads = append(payloads, p)
	}
	require.NoError(t, rows.Err())
	moved := false
	for _, p := range payloads {
		var job api.InterviewRemindJob
		require.NoError(t, json.Unmarshal([]byte(p), &job))
		moved = moved || job.StartsAt.Equal(start)
	}
	assert.True(t, moved, "the interviewers are reminded of the new time")
	var runAfter time.Time
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT max(run_after) FROM job WHERE tenant_id=$1 AND kind='email.interview_reminder'", c.tenant).Scan(&runAfter))
	assert.WithinDuration(t, start.Add(-24*time.Hour), runAfter, time.Second, "the candidate's reminder follows the interview")

	var oldLocal, newBusy, holdMoved int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM outlook_busy WHERE tenant_id=$1 AND user_id=$2 AND starts_at=$3),
		(SELECT count(*) FROM outlook_busy WHERE tenant_id=$1 AND user_id=$2 AND starts_at=$4 AND ends_at=$5),
		(SELECT count(*) FROM calendar_event WHERE tenant_id=$1 AND user_id=$6 AND source='interview' AND starts_at=$4 AND ends_at=$5)`,
		c.tenant, c.userID, oldStart, start, end, c.managerID).Scan(&oldLocal, &newBusy, &holdMoved))
	assert.Zero(t, oldLocal, "the organizer is no longer busy at the old time")
	assert.Positive(t, newBusy, "the organizer is busy at the new time")
	assert.Equal(t, 1, holdMoved, "the hiring manager's hold follows the interview")
	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND entity_id=$2 AND action='interview.updated' AND changes->>'via'='outlook' AND changes ? 'startsAt'",
		c.tenant, c.iv.Id).Scan(&audited))
	assert.Equal(t, 1, audited, "the audit says it was moved in Outlook")

	c.deliver()
	items := mailServer.To(c.email)
	require.Len(t, items, 1, "one reminder, of the new time")
	assert.Contains(t, strings.ReplaceAll(items[0].Data, "=\r\n", ""), humantime.When(start, c.a.deps.Config.TimeZone))

	refreshOutlook(t, c.a, c.user, c.tenant)
	refreshOutlook(t, c.a, c.user, c.tenant)
	assert.Equal(t, patches, c.patches(), "no loop back to Outlook")
	assert.Equal(t, iv.Version, c.interview().Version)
}

func TestOutlookChangeWaitsWhileTheInterviewIsBeingEdited(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	_, _, _, key := c.stored("interview_invitation")
	conn, err := testutil.Current.Admin.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(t.Context(), "SELECT pg_advisory_lock(hashtextextended($1 || ':calendar-source:interview:' || $2, 0))", c.tenant, c.iv.Id)
	require.NoError(t, err)
	start := c.iv.StartsAt.Add(time.Hour)
	c.editInOutlook(moveInOutlook(start, start.Add(45*time.Minute)))
	assert.True(t, c.iv.StartsAt.Equal(c.interview().StartsAt), "an interview being edited is left for the next sync")
	_, _, _, still := c.stored("interview_invitation")
	assert.Equal(t, key, still, "Outlook's change is not marked as taken")
	assert.True(t, c.syncedOK(), "the sync itself goes on")

	_, err = conn.Exec(t.Context(), "SELECT pg_advisory_unlock(hashtextextended($1 || ':calendar-source:interview:' || $2, 0))", c.tenant, c.iv.Id)
	require.NoError(t, err)
	refreshOutlook(t, c.a, c.user, c.tenant)
	assert.True(t, start.Equal(c.interview().StartsAt), "taken at the next sync")
}

func TestOutlookAndHyPlugChangedTogetherIsAConflict(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	c.f.mu.Lock()
	c.f.strictMatch = true
	c.f.mu.Unlock()
	ours := c.iv.StartsAt.Add(2 * time.Hour)
	c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "startsAt": ours}, http.StatusOK)
	c.editInOutlook(moveInOutlook(c.iv.StartsAt.Add(3*time.Hour), c.iv.StartsAt.Add(3*time.Hour+45*time.Minute)))
	assert.EqualValues(t, "conflict", c.invitation().Status, "both sides changed: flagged for review")
	assert.True(t, ours.Equal(c.interview().StartsAt), "HyPlug's time is kept")
}

// Changed in Outlook to more people than a sync takes, then moved in
// HyPlug: Outlook refuses the stale change, and the invitation is flagged
// for review (both sides changed), not failed with the sync's limit message.
func TestOutlookAndHyPlugChangedTogetherBeyondTheAttendeeLimitIsAConflict(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	c.f.mu.Lock()
	c.f.strictMatch = true
	for id, e := range c.f.events {
		people := e["attendees"].([]any)
		for i := range 101 {
			people = append(people, outlookAttendee(fmt.Sprintf("guest%03d@example.test", i), "optional"))
		}
		e["attendees"] = people
		e["changeKey"] = "outlook-" + id + "-crowded"
	}
	c.f.mu.Unlock()
	ours := c.iv.StartsAt.Add(2 * time.Hour)
	c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "startsAt": ours}, http.StatusOK)
	refreshOutlook(t, c.a, c.user, c.tenant) // runs the queued push too
	inv := c.invitation()
	assert.EqualValues(t, "conflict", inv.Status, "both sides changed: flagged for review")
	assert.True(t, ours.Equal(c.interview().StartsAt), "HyPlug's time is kept")
}

func TestOutlookUnsupportedMoveIsFlagged(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(e map[string]any){
		"all day": func(e map[string]any) {
			day := time.Now().UTC().Add(72 * time.Hour).Truncate(24 * time.Hour)
			e["isAllDay"] = true
			moveInOutlook(day, day.Add(24*time.Hour))(e)
		},
		"ten hours": func(e map[string]any) {
			start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Hour)
			moveInOutlook(start, start.Add(10*time.Hour))(e)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := startOutlookInvitation(t, outlookInvite{})
			c.editInOutlook(edit)
			assert.EqualValues(t, "conflict", c.invitation().Status)
			iv := c.interview()
			assert.True(t, c.iv.StartsAt.Equal(iv.StartsAt))
			assert.Equal(t, c.iv.Version, iv.Version)
			assert.True(t, c.syncedOK(), "a refused change never fails the sync")
		})
	}
}

// holdInOutlook is a recruiter's connected Outlook with one private hold in it
// (an interview with no invitation, or a booking).
func holdInOutlook(t *testing.T, booking bool) (candidateFollowUp, string) {
	t.Helper()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	c := candidateFollowUp{a: a, t: t, f: f, tenant: tenant.ID, user: user.Token, userID: user.ID}
	source := "interview"
	if booking {
		source = "booking"
		link := openBookingLink(t, a, user.Token, tenant.ID)
		page := a.publicBooking(link)
		require.NotEmpty(t, page.Slots)
		res := a.request(http.MethodPost, "/v1/public/bookings/"+link, "", map[string]any{"startsAt": page.Slots[0]})
		require.Equal(t, 200, res.Status, "%s", res.Body)
	} else {
		c.iv, _ = scheduleOutlookTestInterview(t, a, user.Token, "nadia."+tenant.ID+"@example.test", []string{user.ID})
		c.path = "/v1/interviews/" + c.iv.Id
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.Len(t, f.events, 1)
	f.strictMatch = true
	f.mu.Unlock()
	return c, source
}

func TestCalendarHoldRenamedInOutlookIsKeptButMovedIsFlagged(t *testing.T) {
	t.Parallel()
	for _, booking := range []bool{false, true} {
		name := map[bool]string{false: "interview", true: "booking"}[booking]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, source := holdInOutlook(t, booking)
			_, _, _, key := c.stored(source)
			c.editInOutlook(func(e map[string]any) {
				e["subject"] = "Blocked: renamed in Outlook"
				e["attendees"] = []any{outlookAttendee("assistant@example.test", "optional")}
			})
			hold := c.event(source)
			assert.EqualValues(t, "synced", hold.SyncStatus, "a rename is no conflict")
			assert.Equal(t, "Blocked: renamed in Outlook", hold.Title)
			assert.False(t, hold.OutlookChangedAt.IsNull())
			version, sent, _, advanced := c.stored(source)
			assert.NotEqual(t, key, advanced, "Outlook's change is taken")
			assert.Equal(t, version, sent)

			if !booking {
				patches := c.patches()
				moved := c.iv.StartsAt.Add(time.Hour)
				c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "startsAt": moved}, http.StatusOK)
				runCoreJobsUntilIdle(t, c.a, c.tenant)
				assert.Equal(t, patches+1, c.patches(), "the move reaches Outlook (no stale version)")
				hold = c.event(source)
				assert.EqualValues(t, "synced", hold.SyncStatus)
				assert.True(t, moved.Equal(hold.StartsAt))
				c.f.mu.Lock()
				assert.NotContains(t, c.f.lastPatch, "attendees", "the person added in Outlook stays")
				c.f.mu.Unlock()
				c.iv = c.interview()
			}

			start := hold.StartsAt.Add(30 * time.Minute)
			c.editInOutlook(moveInOutlook(start, start.Add(time.Hour)))
			assert.EqualValues(t, "conflict", c.event(source).SyncStatus, "a hold moved in Outlook waits for review")
			if !booking {
				assert.True(t, c.iv.StartsAt.Equal(c.interview().StartsAt), "a private hold never moves the interview")
			}
		})
	}
}

// People added in Outlook to a private hold stay there, but a hold made again
// (in another account, after a reconnect) never invites them.
func TestCalendarHoldMadeAgainNeverInvitesPeopleAddedInOutlook(t *testing.T) {
	t.Parallel()
	c, source := holdInOutlook(t, false)
	c.editInOutlook(func(e map[string]any) {
		e["subject"] = "Blocked: renamed in Outlook"
		e["attendees"] = []any{outlookAttendee("assistant@example.test", "optional")}
	})
	require.Equal(t, "Blocked: renamed in Outlook", c.event(source).Title)
	c.f.mu.Lock()
	c.f.account = "new-work-account"
	c.f.mu.Unlock()
	connectOutlook(t, c.a, c.user)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	made := 0
	for _, e := range c.f.events {
		if accountOf(e) == "new-work-account" {
			made++
			assert.Empty(t, e["attendees"], "the hold is made again without the people added in Outlook")
		}
	}
	assert.Equal(t, 1, made)
}
