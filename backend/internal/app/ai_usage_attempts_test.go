package app_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// From the owner's post-migration report (P1): an AI attempt that failed after
// the AI answered was charged by the provider but never counted, so a retried
// reading cost more than the month's use showed. The AI worker reports it
// (ai.attempt_spent); it counts under the job's feature, once per attempt.
func TestAFailedAIAttemptStillCounts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	report := func(attempt int) {
		_, err := testutil.Current.Admin.Exec(context.Background(), `INSERT INTO job (tenant_id, queue, kind, payload)
			VALUES ($1, 'core', 'ai.attempt_spent', jsonb_build_object('jobId', 'job_01jbz6w0m0k8q9x2r3s4t5v6w7',
			  'jobKind', 'document.parse_cv', 'attempt', $2::int, 'inputTokens', 1200, 'outputTokens', 300))`, tenant.ID, attempt)
		require.NoError(t, err)
	}
	report(1)
	report(1) // the same attempt told twice
	report(2) // the retry failed too
	runCoreJobsUntilIdle(t, a, tenant.ID)

	usage := a.aiUsage(owner, "")
	var cv *api.AiUsageFeature
	for i := range usage.ByFeature {
		if usage.ByFeature[i].Feature == api.AiFeatureCvReading {
			cv = &usage.ByFeature[i]
		}
	}
	require.NotNil(t, cv, "counted under CV reading")
	assert.EqualValues(t, 2, cv.Uses, "each attempt once")
	assert.EqualValues(t, 2400, cv.InputTokens)
	assert.EqualValues(t, 600, cv.OutputTokens)
}
