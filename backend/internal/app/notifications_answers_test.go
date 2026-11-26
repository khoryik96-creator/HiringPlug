package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The recruiter who sent a client link hears in the bell when the client
// answers, with the client's comment; no one else does.
func TestTheBellSaysWhenAClientAnswersALink(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	sj := a.shortlistJob(recruiter, tenant.ID, map[string]map[string]any{"Siti": {}, "Wei": {}})
	created, token := a.makeShortlist(recruiter, sj.job.Id, map[string]any{"candidates": []any{pick(sj, "Siti"), pick(sj, "Wei")}}, http.StatusCreated)
	bell := func(tok string) []api.Notification {
		var out api.NotificationList
		res := a.request(http.MethodGet, "/v1/notifications", tok, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&out)
		return kindsOf(out, api.NotificationKindClientAnswer)
	}
	require.Empty(t, bell(recruiter), "nothing until the client answers")

	items := created.Shortlist.Candidates
	require.Equal(t, http.StatusOK, a.clientAnswer(token, items[0].Id, map[string]any{"decision": "approved", "comment": "Strong on SAP; please arrange\n a call."}).Status)
	require.Equal(t, http.StatusOK, a.clientAnswer(token, items[1].Id, map[string]any{"decision": "rejected"}).Status)

	got := bell(recruiter)
	require.Len(t, got, 2)
	titles := []string{got[0].Title, got[1].Title}
	assert.ElementsMatch(t, []string{"Client approved " + items[0].Label, "Client rejected " + items[1].Label}, titles)
	for _, n := range got {
		assert.Equal(t, "#/jobs/"+sj.job.Id, n.Href, "opens the job, where its links are")
		assert.True(t, n.New)
		if n.Title == "Client approved "+items[0].Label {
			assert.Contains(t, n.Detail, `"Strong on SAP; please arrange a call."`, "the client's comment, on one line")
		}
	}
	assert.Empty(t, bell(admin), "only who sent the link hears of it")
}
