package app_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestAPIKeys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin := tenant.Users[api.RoleAdmin].Token

	res := a.request(http.MethodPost, "/v1/api-keys", admin, map[string]any{"name": "  Payroll   sync ", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var created api.ApiKeyCreated
	res.JSON(&created)
	key := created.Key
	assert.True(t, strings.HasPrefix(key, "hpk_"))
	assert.Equal(t, "Payroll sync", created.ApiKey.Name)
	assert.Equal(t, key[:8], created.ApiKey.Prefix)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/api-keys", tenant.Users[api.RoleRecruiter].Token, map[string]any{"name": "x", "role": "viewer"}).Status)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/api-keys", admin, map[string]any{"name": "x", "role": "owner"}).Status, "never as owner")

	t.Run("the key works as its role, and is recorded as the actor", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/candidates", key, map[string]any{"firstName": "Siti"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var c api.Candidate
		res.JSON(&c)
		var actor string
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
			`SELECT actor_id FROM audit_log WHERE entity_id = $1 AND action = 'candidate.created'`, c.Id).Scan(&actor))
		assert.Equal(t, created.ApiKey.Id, actor)
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/api-keys", key, nil).Status, "a recruiter key")

		var keys api.ApiKeyList
		a.request(http.MethodGet, "/v1/api-keys", admin, nil).JSON(&keys)
		require.Len(t, keys.Items, 1)
		assert.NotContains(t, string(a.request(http.MethodGet, "/v1/api-keys", admin, nil).Body), key, "never the key itself")
		assert.True(t, keys.Items[0].LastUsedAt.IsSpecified() && !keys.Items[0].LastUsedAt.IsNull())
	})

	t.Run("keys cannot manage keys, even as admin", func(t *testing.T) {
		var adminKey api.ApiKeyCreated
		a.request(http.MethodPost, "/v1/api-keys", admin, map[string]any{"name": "Ops", "role": "admin"}).JSON(&adminKey)
		res := a.request(http.MethodPost, "/v1/api-keys", adminKey.Key, map[string]any{"name": "More", "role": "admin"})
		assert.Equal(t, http.StatusForbidden, res.Status)
		assert.Contains(t, string(res.Body), "API keys cannot manage")
	})

	t.Run("a revoked or unknown key is refused", func(t *testing.T) {
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/api-keys/"+created.ApiKey.Id, admin, nil).Status)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/candidates", key, nil).Status)
		assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/candidates", "hpk_"+strings.Repeat("0", 64), nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/api-keys/"+created.ApiKey.Id, admin, nil).Status)
	})
}

// receiver is a webhook receiver for tests.
type receiver struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	status   atomic.Int32
}

func newReceiver(t *testing.T) (*receiver, *httptest.Server) {
	r := &receiver{}
	r.status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.requests, r.bodies = append(r.requests, req), append(r.bodies, body)
		r.mu.Unlock()
		w.WriteHeader(int(r.status.Load()))
	}))
	t.Cleanup(srv.Close)
	return r, srv
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func TestWebhooks(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	rec, srv := newReceiver(t)

	res := a.request(http.MethodPost, "/v1/webhooks", owner, map[string]any{"url": srv.URL + "/hooks", "events": []string{"candidate.created", "application.moved"}})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var created api.WebhookCreated
	res.JSON(&created)
	hook := created.Webhook
	assert.NotContains(t, string(a.request(http.MethodGet, "/v1/webhooks", owner, nil).Body), created.Secret, "the secret is shown once")

	cand := a.createCandidate(owner, map[string]any{"firstName": "Siti"})
	a.createCompany(owner, map[string]any{"name": "Not listened for"})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Equal(t, 1, rec.count(), "one event listened for, one delivery")
	req, body := rec.requests[0], rec.bodies[0]
	assert.Equal(t, "candidate.created", req.Header.Get("X-HiringPlug-Event"))
	sig := req.Header.Get("X-HiringPlug-Signature")
	ts, _, _ := strings.Cut(strings.TrimPrefix(sig, "t="), ",")
	unix, err := strconv.ParseInt(ts, 10, 64)
	require.NoError(t, err)
	assert.Equal(t, integrations.Signature(created.Secret, time.Unix(unix, 0), body), sig, "signed with the webhook's secret")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "candidate.created", payload["event"])
	assert.Equal(t, cand.Id, payload["entity"].(map[string]any)["id"])
	assert.Equal(t, req.Header.Get("X-HiringPlug-Delivery"), payload["id"])

	var deliveries api.WebhookDeliveryList
	a.request(http.MethodGet, "/v1/webhooks/"+hook.Id+"/deliveries", owner, nil).JSON(&deliveries)
	require.Len(t, deliveries.Items, 1)
	assert.Equal(t, api.WebhookDeliveryStatusDelivered, deliveries.Items[0].Status)

	t.Run("a failing receiver is tried again later, with the reason", func(t *testing.T) {
		rec.status.Store(http.StatusInternalServerError)
		res := a.request(http.MethodPost, "/v1/webhooks/"+hook.Id+"/deliveries", owner, nil)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		a.request(http.MethodGet, "/v1/webhooks/"+hook.Id+"/deliveries", owner, nil).JSON(&deliveries)
		ping := deliveries.Items[0]
		assert.Equal(t, api.WebhookEventWebhookPing, ping.Event)
		assert.Equal(t, api.WebhookDeliveryStatusPending, ping.Status)
		assert.Equal(t, 1, ping.Attempts)
		lastError, _ := ping.LastError.Get()
		assert.Contains(t, lastError, "HTTP 500")
		next, _ := ping.NextAttemptAt.Get()
		assert.WithinDuration(t, time.Now().Add(time.Minute), next, 10*time.Second)
	})

	t.Run("a webhook turned off sends nothing; others see nothing", func(t *testing.T) {
		before := rec.count()
		res := a.request(http.MethodPatch, "/v1/webhooks/"+hook.Id, owner, map[string]any{"version": hook.Version, "active": false})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		a.createCandidate(owner, map[string]any{"firstName": "Wei"})
		runCoreJobsUntilIdle(t, a, tenant.ID)
		assert.Equal(t, before, rec.count())
		stranger := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
		assert.Empty(t, a.request(http.MethodGet, "/v1/webhooks", stranger, nil).Map()["items"])
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/webhooks/"+hook.Id, stranger, nil).Status)
		assert.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/webhooks/"+hook.Id, owner, nil).Status)
	})
}
