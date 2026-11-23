package cvformats

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// A batch skips a candidate who cannot be formatted now, and stops for what
// would stop every one (no AI) or a fault.
func TestBatchSkipReason(t *testing.T) {
	reason, ok := BatchSkipReason(apperr.New(api.ErrorCodeDOCUMENTLIMIT, "Choose which formatted CVs to replace."))
	assert.True(t, ok)
	assert.Contains(t, reason, "as many formatted CVs as they can keep")
	_, ok = BatchSkipReason(apperr.NotFound("candidate"))
	assert.True(t, ok)
	_, ok = BatchSkipReason(apperr.New(api.ErrorCodeSTUDIOCVOFF, "no AI"))
	assert.False(t, ok, "no AI: the whole batch stops")
	_, ok = BatchSkipReason(errors.New("database down"))
	assert.False(t, ok)
}
