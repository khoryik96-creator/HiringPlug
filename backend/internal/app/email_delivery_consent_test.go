package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestQueuedEmailRespectsConsentWithdrawnBeforeDelivery(t *testing.T) {
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	address := "consent." + tenant.ID + "@example.test"
	c := a.createCandidate(token, map[string]any{"firstName": "Review", "email": address})
	draft := map[string]any{"to": []string{address}, "subject": "Synthetic review", "body": "Testing queued mail consent", "candidateId": c.Id}
	res := a.request(http.MethodPost, "/v1/emails", token, draft)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	require.Empty(t, mailServer.To(address))
	// A recipient in CC is protected too, even when no candidate ID is supplied.
	cc := a.request(http.MethodPost, "/v1/emails", token, map[string]any{"to": []string{"team." + tenant.ID + "@example.test"}, "cc": []string{address}, "subject": "Copy", "body": "Checking delivery"})
	require.Equal(t, http.StatusAccepted, cc.Status, "%s", cc.Body)
	a.setConsent(token, c.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	res = a.request(http.MethodPost, "/v1/emails", token, draft)
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Empty(t, mailServer.To(address))
	require.Empty(t, mailServer.To("team."+tenant.ID+"@example.test"))
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		"SELECT status FROM email WHERE tenant_id=$1 AND $2=ANY(to_addresses)", tenant.ID, address).Scan(&status))
	require.Equal(t, "failed", status)
}
