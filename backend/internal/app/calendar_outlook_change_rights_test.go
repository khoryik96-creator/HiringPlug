package app_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// setRecruiterAccess sets the company's access for recruiters as an owner
// would in Settings: these permissions (from the defaults) and candidate scope.
func (c candidateFollowUp) setRecruiterAccess(perms []api.Permission, scope string) {
	c.t.Helper()
	known := authz.PermissionsForRole(api.RoleRecruiter)
	_, err := testutil.Current.Admin.Exec(c.t.Context(), `INSERT INTO role_setting (tenant_id, role, permissions, known_permissions, candidate_scope, job_candidates, updated_by)
		VALUES ($1, 'recruiter', $2, $3, $4, false, 'synthetic-owner')`, c.tenant, perms, known, scope)
	require.NoError(c.t, err)
}

// storedInterview is the interview as the database has it (one out of the
// organizer's scope is hidden from them).
func (c candidateFollowUp) storedInterview() (title string, version int, start time.Time) {
	c.t.Helper()
	require.NoError(c.t, testutil.Current.Admin.QueryRow(c.t.Context(), "SELECT title, version, starts_at FROM interview WHERE id=$1", c.iv.Id).Scan(&title, &version, &start))
	return
}

func renameAndMoveInOutlook(c candidateFollowUp) {
	start := c.iv.StartsAt.Add(2 * time.Hour)
	c.editInOutlook(func(e map[string]any) {
		e["subject"] = "Renamed in Outlook"
		moveInOutlook(start, start.Add(45*time.Minute))(e)
	})
}

// A change in Outlook is taken as its organizer's: one who may no longer
// schedule interviews, or no longer sees this one, changes nothing in
// HiringPlug; a move waits for review as before.
func TestOutlookChangeIsTakenOnlyWhileTheOrganizerMayMakeIt(t *testing.T) {
	t.Parallel()
	defaults := authz.PermissionsForRole(api.RoleRecruiter)
	for name, take := range map[string]func(c candidateFollowUp){
		"may no longer schedule interviews": func(c candidateFollowUp) {
			c.setRecruiterAccess(without(defaults, api.PermissionInterviewsSchedule), "all")
		},
		"candidate out of their scope": func(c candidateFollowUp) {
			c.setRecruiterAccess(defaults, "own")
			_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE candidate SET owner_id=NULL WHERE id=$1", c.iv.CandidateId)
			require.NoError(t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := startOutlookInvitation(t, outlookInvite{})
			take(c)
			renameAndMoveInOutlook(c)
			title, version, start := c.storedInterview()
			assert.Equal(t, c.iv.Title, title, "the interview is not changed")
			assert.Equal(t, c.iv.Version, version)
			assert.True(t, c.iv.StartsAt.Equal(start))
			_, _, status, _ := c.stored("interview_invitation")
			assert.Equal(t, "conflict", status, "the move waits for review")
			assert.True(t, c.syncedOK())
		})
	}
}

// An interview HiringPlug no longer schedules (done, or the candidate did not
// come) and then moved in Outlook is flagged, never left unseen.
func TestOutlookMoveOfAnInterviewNoLongerScheduledIsFlagged(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"completed", "no_show"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			c := startOutlookInvitation(t, outlookInvite{})
			c.iv = c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "status": status}, http.StatusOK)
			runCoreJobsUntilIdle(t, c.a, c.tenant)
			renameAndMoveInOutlook(c)
			_, _, state, _ := c.stored("interview_invitation")
			assert.Equal(t, "conflict", state, "moved in Outlook: waits for review")
			iv := c.interview()
			assert.True(t, c.iv.StartsAt.Equal(iv.StartsAt))
			assert.Equal(t, c.iv.Version, iv.Version)
			assert.EqualValues(t, status, iv.Status)
		})
	}
}

// An invitation moved outside the sync's window (read on its own) with more
// people than a sync holds fails the sync with its reason, as one inside the
// window does: nothing half-written, previous busy times kept.
func TestOutlookInvitationBeyondTheWindowWithTooManyPeopleStopsTheSyncCleanly(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	_, _, _, key := c.stored("interview_invitation")
	start := c.iv.StartsAt.Add(200 * 24 * time.Hour)
	c.f.mu.Lock()
	c.f.outsideWindow = map[string]bool{}
	for id := range c.f.events {
		c.f.outsideWindow[id] = true
	}
	c.f.mu.Unlock()
	c.editInOutlook(func(e map[string]any) {
		people := e["attendees"].([]any)
		for i := range 100 {
			people = append(people, outlookAttendee(fmt.Sprintf("guest%03d@example.test", i), "optional"))
		}
		e["attendees"] = people
		moveInOutlook(start, start.Add(45*time.Minute))(e)
	})
	assert.False(t, c.syncedOK(), "the sync stops")
	var syncError string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT coalesce(sync_error,'') FROM calendar_connection WHERE tenant_id=$1 AND user_id=$2", c.tenant, c.userID).Scan(&syncError))
	assert.Contains(t, syncError, "event limit", "and says why")
	_, _, status, still := c.stored("interview_invitation")
	assert.Equal(t, key, still, "nothing of Outlook's change is written")
	assert.Equal(t, "synced", status)
	assert.True(t, c.iv.StartsAt.Equal(c.interview().StartsAt))
}
