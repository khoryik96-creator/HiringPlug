package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCVsSentTo(t *testing.T) {
	cvs := []CVFile{{CandidateID: "cand_a"}, {CandidateID: "cand_a"}, {CandidateID: "cand_b"}, {CandidateID: "cand_c"}}
	assert.Equal(t, []string{"cand_a", "cand_c"}, CVsSentTo(cvs, []string{"cand_b"}),
		"each candidate once, not a CV sent to its own candidate")
	assert.Empty(t, CVsSentTo(cvs[2:3], []string{"cand_b"}))
	assert.Empty(t, CVsSentTo(nil, nil))
}
