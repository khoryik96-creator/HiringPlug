package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The moment a CV is formatted CV Studio's way, the person who asked for it
// is told in the bell, by the candidate's name (new until they look); a run
// that failed is said too. Nobody else is told.
func TestTheBellSaysWhichCandidatesCVIsFormatted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	token, other := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	aiConn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = aiConn.Close(ctx) }()
	a := newTestApp(t, matchingOn)
	format := func(name string, blind bool) api.StudioCv {
		c := a.createCandidate(token, map[string]any{"firstName": name, "lastName": "Example"})
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.docx", minimalDOCX(t))
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var doc api.Document
		res.JSON(&doc)
		res = a.request(http.MethodPost, "/v1/candidates/"+c.Id+"/studio-cvs", token, map[string]any{"documentId": doc.Id, "blind": blind})
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		var run api.StudioCv
		res.JSON(&run)
		return run
	}
	bell := func(tok string) api.NotificationList {
		var out api.NotificationList
		res := a.request(http.MethodGet, "/v1/notifications", tok, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&out)
		return out
	}
	siti := format("Siti", true)
	assert.Empty(t, kindsOf(bell(token), api.NotificationKindCvFormatted), "not while it is being made")
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "studioCvId", siti.Id), api.JobKindCvStudioFormatted,
		studioResult(siti.Id, simpleDocx(t, "Senior Accountant"), ""))

	list := bell(token)
	done := kindsOf(list, api.NotificationKindCvFormatted)
	require.Len(t, done, 1)
	assert.Equal(t, "Blind CV formatted: Siti Example", done[0].Title)
	assert.Equal(t, "#/candidates/"+siti.CandidateId, done[0].Href)
	assert.True(t, done[0].New)
	assert.Positive(t, list.Unseen)
	assert.Empty(t, kindsOf(bell(other), api.NotificationKindCvFormatted), "only who asked for it")

	lim := format("Lim", false)
	answerAIJob(t, a, aiConn, claimAIJobWhere(t, aiConn, "studioCvId", lim.Id), api.JobKindCvStudioFormatted,
		studioFailure(lim.Id, "The CV could not be read."))
	titles := []string{}
	for _, it := range kindsOf(bell(token), api.NotificationKindCvFormatted) {
		titles = append(titles, it.Title)
	}
	assert.Equal(t, []string{"CV could not be formatted: Lim Example", "Blind CV formatted: Siti Example"}, titles, "newest first")

	// Turned off in Settings: no longer in the bell.
	res := a.request(http.MethodPut, "/v1/notifications/settings", token, map[string]any{"morningEmail": false, "mutedKinds": []string{"cv_formatted"}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Empty(t, kindsOf(bell(token), api.NotificationKindCvFormatted))
}

func kindsOf(list api.NotificationList, kind api.NotificationKind) []api.Notification {
	var out []api.Notification
	for _, it := range list.Items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func studioFailure(id, why string) api.CvStudioFormattedJob {
	r := studioResult(id, nil, "")
	r.Error = nullable.NewNullableWithValue(why)
	return r
}
