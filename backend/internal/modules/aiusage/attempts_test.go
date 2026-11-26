package aiusage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Every job kind that asks a paid AI has the feature its spend counts under.
func TestEveryPaidAIJobHasAFeature(t *testing.T) {
	for _, kind := range append(append([]api.JobKind{}, jobqueue.PaidAI...), jobqueue.PaidAIIfUsed...) {
		f := FeatureOfJob(kind)
		assert.True(t, IsFeature(f), "%s spends for %q", kind, f)
	}
}
