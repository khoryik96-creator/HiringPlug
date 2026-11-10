package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnEmailTakesUpTo20MBOfAttachments(t *testing.T) {
	assert.NoError(t, attachmentsFit(14<<20), "more than the old 10 MB: a few formatted profiles")
	assert.NoError(t, attachmentsFit(20<<20))
	assert.Error(t, attachmentsFit(20<<20+1))
}

func TestACVGoesWithAnEmailAboutItsCandidate(t *testing.T) {
	siti := "cand_siti"
	assert.True(t, cvAllowed("cand_siti", &siti, nil))
	assert.False(t, cvAllowed("cand_farid", &siti, nil))
	assert.False(t, cvAllowed("cand_siti", nil, nil), "an email about no candidate takes no CV")
	list := []Shortlisted{{CandidateID: "cand_farid"}, {CandidateID: "cand_nurul"}}
	assert.True(t, cvAllowed("cand_nurul", &list[0].CandidateID, list), "any of the candidates'")
	assert.False(t, cvAllowed("cand_siti", &list[0].CandidateID, list))
}
