package app_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestOutlookReconnectionKeepsOnlyTheSameMailboxesSchedules(t *testing.T) {
	t.Parallel()
	r := startOutlookSendTest(t, nil)
	planned := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	connect := func(id string) {
		connectTestOutlook(t, r.a, r.user, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/profile" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "mail": "recruiter@example.test"})
				return
			}
			r.provider.serve(w, request)
		}))
	}

	connect("original-account") // Upgrade a legacy grant that has no Graph account ID.
	require.Equal(t, []api.OutlookScheduledSend{planned}, r.list(t))
	connect("original-account")
	require.Equal(t, []api.OutlookScheduledSend{planned}, r.list(t))
	require.Equal(t, 200, r.a.request("GET", r.path, r.user, nil).Status, "mail index stays linked")

	connect("different-account") // The displayed address alone cannot replace a known ID.
	require.Equal(t, api.OutlookScheduledSendStatusCancelled, r.list(t)[0].Status)
	require.Zero(t, r.sends())
}

func TestOutlookPagesPendingSendsBeyondTheFormerLimit(t *testing.T) {
	t.Parallel()
	r := startOutlookSendTest(t, nil)
	planned := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(2 * time.Hour)})
	_, err := testutil.Current.Admin.Exec(t.Context(), `
		INSERT INTO email_outlook_send(tenant_id,user_id,mailbox_id,message_id,remote_id,draft_version,subject,send_at)
		SELECT p.tenant_id,p.user_id,p.mailbox_id,p.message_id,'test-draft-'||n,p.draft_version,'Earlier send',p.send_at-n*interval '1 second'
		FROM email_outlook_send p CROSS JOIN generate_series(1,200) n WHERE p.id=$1`, planned.Id)
	require.NoError(t, err)

	ids := map[string]bool{}
	offset := 0
	for range 5 {
		res := r.a.request("GET", "/v1/outlook/scheduled?offset="+strconv.Itoa(offset), r.user, nil)
		require.Equal(t, 200, res.Status, "%s", res.Body)
		var page api.OutlookScheduleList
		res.JSON(&page)
		for _, item := range page.Items {
			require.False(t, ids[item.Id], "Each pending send appears once")
			ids[item.Id] = true
		}
		next, nextErr := page.NextOffset.Get()
		if nextErr != nil {
			break
		}
		offset = next
	}
	require.Len(t, ids, 201)
	require.True(t, ids[planned.Id])
	require.Equal(t, 200, r.a.request("POST", "/v1/outlook/scheduled/"+planned.Id+"/cancel", r.user, map[string]int{"version": planned.Version}).Status)
}

func TestOutlookRechecksUncertainSendWithoutResending(t *testing.T) {
	t.Parallel()
	for _, sent := range []bool{false, true} {
		t.Run(strconv.FormatBool(sent), func(t *testing.T) {
			r := startOutlookSendTest(t, nil)
			planned := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
			_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_send SET status='uncertain' WHERE id=$1", planned.Id)
			require.NoError(t, err)
			if sent {
				r.provider.mu.Lock()
				for id := range r.provider.drafts {
					r.provider.accepted[id] = true
				}
				r.provider.mu.Unlock()
			}
			path := "/v1/outlook/scheduled/" + planned.Id + "/resolve"
			body := map[string]int{"version": planned.Version}
			require.Equal(t, 404, r.a.request("POST", path, r.owner, body).Status)
			res := r.a.request("POST", path, r.user, body)
			require.Equal(t, 200, res.Status, "%s", res.Body)
			var resolved api.OutlookScheduledSend
			res.JSON(&resolved)
			require.Equal(t, planned.Version+1, resolved.Version)
			if sent {
				require.Equal(t, api.OutlookScheduledSendStatusSent, resolved.Status)
			} else {
				require.Equal(t, api.OutlookScheduledSendStatusNeedsReview, resolved.Status)
				require.Equal(t, 200, r.a.request("POST", "/v1/outlook/scheduled/"+planned.Id+"/cancel", r.user,
					map[string]int{"version": resolved.Version}).Status)
			}
			require.Equal(t, 409, r.a.request("POST", path, r.user, body).Status)
			require.Zero(t, r.sends(), "Looking for a receipt never sends")
		})
	}
}
