package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenamesAreOptionsSentWithTheirOldWord(t *testing.T) {
	saved := words("ACCA", "CFA", "CISCO")
	got, err := Renames("certification", []OptionInput{
		{Key: "ACCA", Label: "ACCA"},        // unchanged
		{Key: "CFA", Label: "CFA  Level 3"}, // renamed (spaces tidied)
		{Key: "CISCO", Label: "Cisco"},      // only its case
		{Label: "PMP"},                      // new
		{Key: "Gone", Label: "Gone again"},  // not in the list now
	}, saved)
	require.NoError(t, err)
	assert.Equal(t, []Rename{{From: "CFA", To: "CFA Level 3"}, {From: "CISCO", To: "Cisco"}}, got)

	got, err = Renames("certification", []OptionInput{{Key: "CFA", Label: "CFA 2"}, {Label: "CFA"}}, saved)
	require.NoError(t, err)
	assert.Empty(t, got, "the old word is still in the list: a new word, not a rename")

	got, err = Renames("certification", []OptionInput{{Key: "CFA", Label: "CFA Level 3"}, {Label: "cfa"}}, saved)
	require.NoError(t, err)
	assert.Empty(t, got, "still in the list in another case: not a rename")

	got, err = Renames("candidate_source", []OptionInput{{Key: "linkedin", Label: "LinkedIn Recruiter"}}, opts("linkedin", "LinkedIn"))
	require.NoError(t, err)
	assert.Empty(t, got, "other lists keep their keys: nothing to carry over")
}

func TestRenamesIntoEachOtherAreRefused(t *testing.T) {
	saved := words("ACCA", "CFA", "CIA")
	for name, in := range map[string][]OptionInput{
		"swapped":        {{Key: "ACCA", Label: "CFA"}, {Key: "CFA", Label: "ACCA"}},
		"chained":        {{Key: "ACCA", Label: "CFA"}, {Key: "CFA", Label: "CIA 2"}},
		"one word twice": {{Key: "ACCA", Label: "ACCA 1"}, {Key: "ACCA", Label: "ACCA 2"}},
	} {
		_, err := Renames("certification", in, saved)
		assert.Equal(t, []string{"options"}, fieldsOf(t, err), name)
	}
}

func TestBuiltInOptionsComeInPairs(t *testing.T) {
	assert.Len(t, opts("a", "A", "b", "B"), 2)
	// (Written in place, the linter refuses an odd count already; a list built some other way is caught at start.)
	odd := []string{"a", "A", "job_fair"}
	assert.Panics(t, func() { opts(odd...) }, "a key without its label is a mistake, not an option to drop")
}
