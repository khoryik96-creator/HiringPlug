package pipeline

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

func counter() func() string {
	n := 0
	return func() string { n++; return "new" + strconv.Itoa(n) }
}

func fieldsOf(errs apperr.FieldErrors) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Field)
	}
	return out
}

func TestStagesAreTidiedAndKeepTheirKeys(t *testing.T) {
	got, errs := NormalizeSteps([]Step{
		{Key: "applied", Label: "  Applied ", Stage: Applied},
		{Label: "Phone   screen", Stage: Screening},
		{Label: "Tech test", Stage: Interview},
		{Key: "applied", Label: "Panel", Stage: Interview}, // a key used twice: the second gets a new one
		{Label: "Placed", Stage: Placed},
	}, counter())
	require.Empty(t, errs)
	assert.Equal(t, []Step{
		{Key: "applied", Label: "Applied", Stage: Applied},
		{Key: "new1", Label: "Phone screen", Stage: Screening},
		{Key: "new2", Label: "Tech test", Stage: Interview},
		{Key: "new3", Label: "Panel", Stage: Interview},
		{Key: "new4", Label: "Placed", Stage: Placed},
	}, got)
}

func TestStagesMustBeNamedOnceOfAnOpenKindAndInOrder(t *testing.T) {
	_, errs := NormalizeSteps([]Step{
		{Label: "", Stage: Applied},
		{Label: "Interview", Stage: Interview},
		{Label: "interview", Stage: Interview},
		{Label: "Screen", Stage: Screening}, // after an interview stage
		{Label: "Gone", Stage: Rejected},
		{Label: "A very long name for a stage that goes on and on", Stage: Offer},
	}, counter())
	assert.Equal(t, []string{"steps.0.label", "steps.2.label", "steps.3.stage", "steps.4.stage", "steps.5.label"}, fieldsOf(errs))
	_, errs = NormalizeSteps(nil, counter())
	assert.Equal(t, []string{"steps"}, fieldsOf(errs))
	_, errs = NormalizeSteps(make([]Step, MaxSteps+1), counter())
	assert.Equal(t, []string{"steps"}, fieldsOf(errs))
}

func TestTheStandardStagesAreOneOfEachKind(t *testing.T) {
	std := StandardSteps()
	require.Len(t, std, len(OpenStages))
	got, errs := NormalizeSteps(std, counter())
	assert.Empty(t, errs)
	assert.Equal(t, std, got, "valid as they are")
	_, ok := StepFor(std, Interview, Interview)
	assert.True(t, ok)
	_, ok = StepFor(std, Interview, Offer)
	assert.False(t, ok, "a stage of another kind")
}

func TestMovingBetweenTwoStagesOfOneKindIsAChange(t *testing.T) {
	test, panel := "tech", "panel"
	current := Application{Stage: Interview, Step: &test}
	_, changed, err := CheckMove(current, Move{To: Interview, Step: &panel})
	require.NoError(t, err)
	assert.True(t, changed)
	_, changed, err = CheckMove(current, Move{To: Interview, Step: &test})
	require.NoError(t, err)
	assert.False(t, changed, "already there")
	reason := "other"
	m, _, err := CheckMove(current, Move{To: Rejected, Step: &panel, Reason: &reason})
	require.NoError(t, err)
	assert.Nil(t, m.Step, "closing leaves the job's stages")
}
