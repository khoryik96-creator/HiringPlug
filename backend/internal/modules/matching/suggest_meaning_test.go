package matching

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Those found by meaning come first; then those found only by the job's
// words, unless meaning read them and turned them down (they share words
// with the job, not its work); someone found both ways comes once.
func TestPossibleCandidatesByMeaningFirstThenByWordsMeaningDidNotTurnDown(t *testing.T) {
	t.Parallel()
	found := MeaningFound{
		Matches:    []MeaningMatch{{CandidateID: "recruiter"}, {CandidateID: "headhunter"}},
		NotMatched: map[string]bool{"buyer": true},
	}
	words := []CVHit{{CandidateID: "buyer"}, {CandidateID: "headhunter"}, {CandidateID: "not-read-yet"}}
	assert.Equal(t, []string{"recruiter", "headhunter", "not-read-yet"}, suggestionOrder(found, words))
	assert.Equal(t, []string{"buyer", "headhunter", "not-read-yet"}, suggestionOrder(MeaningFound{}, words), "without meaning: by words")
}
