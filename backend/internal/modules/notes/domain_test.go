package notes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	d, errs := Normalize(Draft{SubjectType: "candidate", SubjectID: "cand_x", Body: "  Good communicator.\n "}, false)
	require.NoError(t, errs.Err())
	assert.Equal(t, "Good communicator.", d.Body)
	assert.Equal(t, "general", d.Kind)

	_, errs = Normalize(Draft{Body: "  "}, true)
	require.Len(t, errs, 1)
	assert.Equal(t, "body", errs[0].Field)

	_, errs = Normalize(Draft{Body: "Wants 15k", Sensitive: true}, false)
	require.Len(t, errs, 1)
	assert.Equal(t, "sensitive", errs[0].Field, "only those who may read sensitive notes may write them")
}

func TestMayDelete(t *testing.T) {
	n := Note{AuthorID: "usr_a"}
	assert.True(t, MayDelete(n, "usr_a", false), "your own")
	assert.False(t, MayDelete(n, "usr_b", false), "someone else's")
	assert.True(t, MayDelete(n, "usr_b", true), "with notes:manage")
}
