package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The AI worker reports whether the provider answered; while its latest call
// failed, everyone signed in is told the AI is down, why and since when.
func TestAIStatusTellsWhenTheProviderIsDown(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleViewer)
	other := testutil.Current.NewTenant(t, api.RoleViewer)
	viewer := tenant.Users[api.RoleViewer].Token
	ctx := context.Background()
	report := func(ok bool, problem string, at time.Time) {
		t.Helper()
		r := api.AiProviderReportedJob{Provider: "deepseek", Ok: ok, At: at, Error: nullable.NewNullNullable[string]()}
		if problem != "" {
			r.Error = nullable.NewNullableWithValue(problem)
		}
		require.NoError(t, database.InTx(ctx, a.deps.Pool, authz.System(tenant.ID), func(tx pgx.Tx) error {
			_, err := jobqueue.Enqueue(ctx, tx, tenant.ID, api.JobKindAiproviderReported, r)
			return err
		}))
		runCoreJobsUntilIdle(t, a, tenant.ID)
	}
	status := func(token string) api.AiStatus {
		t.Helper()
		res := a.request(http.MethodGet, "/v1/ai-status", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var s api.AiStatus
		res.JSON(&s)
		return s
	}

	assert.Equal(t, http.StatusUnauthorized, a.request(http.MethodGet, "/v1/ai-status", "", nil).Status)
	assert.False(t, status(viewer).Down, "nothing tried yet")

	began := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	report(false, "the AI provider could not be reached", began)
	report(false, "the AI provider did not answer in time", began.Add(5*time.Minute))
	s := status(viewer)
	require.True(t, s.Down)
	assert.Equal(t, "AI 1", s.Provider.MustGet(), "numbered, not named")
	assert.Equal(t, "the AI provider did not answer in time", s.Message.MustGet(), "the latest reason")
	assert.True(t, s.Since.MustGet().Equal(began), "since the failures began")
	assert.False(t, status(other.Users[api.RoleViewer].Token).Down, "another company's AI is its own")

	report(false, "late news", began.Add(-time.Hour))
	assert.Equal(t, "the AI provider did not answer in time", status(viewer).Message.MustGet(), "an older report does not replace a newer one")

	report(true, "", began.Add(6*time.Minute))
	s = status(viewer)
	assert.False(t, s.Down, "it answered: the notice goes")
	assert.True(t, s.Message.IsNull())

	report(false, "the AI provider could not be reached", time.Now().Add(-2*time.Hour))
	assert.False(t, status(viewer).Down, "older than the answer")
}
