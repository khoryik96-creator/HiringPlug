package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

func ptr(s string) *string { return &s }

// Every move from every stage to every stage, with and without a reason.
func TestEveryMove(t *testing.T) {
	all := append(append([]string{}, OpenStages...), Rejected, Withdrawn)
	reason := ptr("not_qualified")
	for _, from := range all {
		current := Application{Stage: from}
		if IsClosed(from) {
			current.Reason = reason
		}
		for _, to := range all {
			move := Move{To: to}
			if IsClosed(to) {
				move.Reason = reason
			}
			_, changed, err := CheckMove(current, move)
			allowed := err == nil
			want := true
			switch {
			case IsClosed(from) && to == Placed:
				want = false // reopen first
			case from == Placed && to != Placed && to != Offer && to != Withdrawn:
				want = false // only back to offer, or withdrawn
			}
			assert.Equal(t, want, allowed, "%s -> %s: %v", from, to, err)
			if allowed {
				assert.Equal(t, from != to, changed, "%s -> %s", from, to)
			}
		}
	}
}

func TestClosingNeedsAReasonAndOpeningTakesNone(t *testing.T) {
	fieldOf := func(err error) string {
		var e *apperr.Error
		require.ErrorAs(t, err, &e)
		fields, ok := e.Details["fields"].(apperr.FieldErrors)
		require.True(t, ok)
		require.Len(t, fields, 1)
		return fields[0].Field
	}
	_, _, err := CheckMove(Application{Stage: Screening}, Move{To: Rejected})
	assert.Equal(t, "reason", fieldOf(err))
	_, _, err = CheckMove(Application{Stage: Screening}, Move{To: Interview, Reason: ptr("other")})
	assert.Equal(t, "reason", fieldOf(err))

	m, changed, err := CheckMove(Application{Stage: Screening}, Move{To: Withdrawn, Reason: ptr("accepted_other_offer"), Note: ptr("  went to a bank  ")})
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "went to a bank", *m.Note)

	_, _, err = CheckMove(Application{Stage: Placed}, Move{To: Screening})
	var notAllowed MoveNotAllowedError
	require.ErrorAs(t, err, &notAllowed)
	assert.Equal(t, "A placed candidate can only go back to offer, or be withdrawn.", notAllowed.Message)
}

func TestChangingOnlyTheReasonIsAChange(t *testing.T) {
	current := Application{Stage: Rejected, Reason: ptr("not_qualified")}
	_, changed, err := CheckMove(current, Move{To: Rejected, Reason: ptr("salary_mismatch")})
	require.NoError(t, err)
	assert.True(t, changed)
	_, changed, err = CheckMove(current, Move{To: Rejected, Reason: ptr("not_qualified")})
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestNewApplicationsStartInAnOpenStage(t *testing.T) {
	assert.Empty(t, CheckNewStage(Interview))
	assert.Len(t, CheckNewStage(Rejected), 1)
}
