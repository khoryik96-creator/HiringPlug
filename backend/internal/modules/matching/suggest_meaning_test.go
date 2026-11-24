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

// A job is read by meaning from its description and ad, without salary
// figures (they are never sent to be read, nor kept with the reading).
func TestAJobIsReadByMeaningWithoutItsSalary(t *testing.T) {
	t.Parallel()
	desc := "Handle full set of accounts.\nSalary: RM 4,500 - 5,500 per month\nLiaise with auditors."
	ad := "Accounts Executive wanted. Basic pay: RM5000 + bonus"
	got := JobMeaningText(Job{Title: "Accounts Executive", Description: &desc, AdText: &ad})
	assert.Contains(t, got, "Handle full set of accounts.")
	assert.Contains(t, got, "Liaise with auditors.")
	assert.NotContains(t, got, "4,500")
	assert.NotContains(t, got, "5000")
}
