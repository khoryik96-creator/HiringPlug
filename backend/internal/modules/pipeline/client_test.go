package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientApprovedStep(t *testing.T) {
	step, ok := ClientApprovedStep(StandardSteps(), false)
	assert.True(t, ok)
	assert.Nil(t, step, "the standard interview stage")

	own := []Step{{Key: "s1", Stage: Applied}, {Key: "s2", Stage: Submitted}, {Key: "s3", Stage: Interview}, {Key: "s4", Stage: Interview}}
	step, ok = ClientApprovedStep(own, true)
	assert.True(t, ok)
	assert.Equal(t, "s3", *step, "the job's first interview stage")

	_, ok = ClientApprovedStep([]Step{{Key: "s1", Stage: Applied}, {Key: "s5", Stage: Offer}}, true)
	assert.False(t, ok, "no interview stage: it stays where it is")
}
