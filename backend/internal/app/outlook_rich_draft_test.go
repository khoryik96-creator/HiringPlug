package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestOutlookDraftRichFormattingCanBeScheduledAndIsSanitized(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := newDraftProvider()
	connectTestOutlook(t, a, token, http.HandlerFunc(f.serve))
	require.Equal(t, http.StatusNoContent, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	draft := createDraftForTest(t, a, token)
	path := "/v1/outlook/drafts/" + draft.MessageId
	r := a.request("PATCH", path, token, map[string]any{
		"version":  draft.Version,
		"bodyHtml": "<p><strong>Interview</strong> at 10.</p><script>unsafe()</script><img src=\"https://example.test/pixel\"><img src=\"cid:logo.png@abc\">",
	})
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var formatted api.OutlookDraft
	r.JSON(&formatted)
	require.NotNil(t, formatted.BodyHtml)
	assert.Contains(t, *formatted.BodyHtml, "<strong>Interview</strong>")
	assert.NotContains(t, *formatted.BodyHtml, "unsafe")
	assert.NotContains(t, *formatted.BodyHtml, "pixel")
	assert.Contains(t, *formatted.BodyHtml, `src="cid:logo.png@abc"`)
	assert.Equal(t, "Interview at 10.", formatted.Body)

	r = a.request("PUT", path+"/send", token, map[string]any{
		"draftVersion": formatted.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour),
		"changes": map[string]string{"bodyHtml": "<p>We are <em>confirmed</em>.</p>"},
	})
	require.Equal(t, http.StatusAccepted, r.Status, "%s", r.Body)
	r = a.request("GET", path, token, nil)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var scheduled api.OutlookDraft
	r.JSON(&scheduled)
	require.NotNil(t, scheduled.BodyHtml)
	assert.Equal(t, "<p>We are <em>confirmed</em>.</p>", *scheduled.BodyHtml)
	assert.False(t, scheduled.ScheduleNeedsReview)
	f.mu.Lock()
	attempts := f.attempts
	f.mu.Unlock()
	assert.Zero(t, attempts)
}
