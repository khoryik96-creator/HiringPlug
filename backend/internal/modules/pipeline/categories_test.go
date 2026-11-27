package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardCategoriesHaveThreeInterviewRounds(t *testing.T) {
	cats := StandardCategories()
	var labels []string
	for _, c := range cats {
		labels = append(labels, c.Label)
	}
	assert.Equal(t, []string{"Applied", "Screening", "Submitted", "1st Interview", "2nd Interview", "Final Interview", "Offer", "Placed"}, labels)
	steps := StandardStepsOf(cats)
	assert.Equal(t, Step{Key: "interview_2", Label: "2nd Interview", Stage: Interview, Category: "interview_2"}, steps[4], "a job with no template has a stage per category")
}

func TestCategoriesAreTidiedCheckedAndGroupedByKind(t *testing.T) {
	before := StandardCategories()
	got, errs := NormalizeCategories([]Category{
		{Key: "placed", Label: "Placed", Kind: Placed}, // out of order: grouped by kind
		{Key: "applied", Label: " Applied ", Kind: Applied},
		{Key: "screening", Label: "Phone   screen", Kind: Screening}, // renamed
		{Key: "submitted", Label: "Submitted", Kind: Submitted},
		{Key: "interview", Label: "1st Interview", Kind: Interview},
		{Label: "Technical Test", Kind: Interview}, // new
		{Key: "interview_final", Label: "Final Interview", Kind: Interview},
		{Key: "offer", Label: "Offer", Kind: Offer},
	}, before)
	require.Empty(t, errs)
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	assert.Equal(t, []string{"applied", "screening", "submitted", "interview", "technical_test", "interview_final", "offer", "placed"}, keys)
	assert.Equal(t, "Phone screen", got[1].Label)
	assert.Equal(t, []string{"interview_2"}, Removed(before, got))
}

func TestCategoriesRefuseWhatBreaksTheRules(t *testing.T) {
	before := StandardCategories()
	_, errs := NormalizeCategories([]Category{
		{Key: "applied", Label: "Applied", Kind: Screening}, // a category keeps its kind
		{Label: "applied", Kind: Applied},                   // the same name
		{Label: "", Kind: Submitted},
		{Label: "Interview", Kind: "rejected"},
	}, before)
	assert.ElementsMatch(t, []string{"items.0.kind", "items.1.label", "items.2.label", "items.3.kind",
		"items", "items", "items"}, fieldsOf(errs), "kinds left without a category: interview, offer, placed")
}

func TestStepsAndApplicationsFallUnderACategoryOfTheirKind(t *testing.T) {
	cats := StandardCategories()
	steps, errs := CheckStepCategories([]Step{
		{Key: "s1", Label: "Applied", Stage: Applied},
		{Key: "s2", Label: "Panel", Stage: Interview, Category: "interview_2"},
		{Key: "s3", Label: "Bad", Stage: Offer, Category: "interview_2"},
	}, cats)
	assert.Equal(t, []string{"steps.2.category"}, fieldsOf(errs), "an offer stage cannot fall under an interview category")
	assert.Equal(t, "applied", steps[0].Category, "none chosen: the kind's first")
	assert.Equal(t, "interview_2", steps[1].Category)

	assert.Equal(t, "interview", ResolveSteps([]Step{{Stage: Interview, Category: "gone"}}, cats)[0].Category, "a category that is gone: the kind's first")
	key := "s2"
	assert.Equal(t, "interview_2", *AppCategory(steps, cats, Interview, &key))
	assert.Equal(t, "interview_2", *AppCategory(steps, cats, Interview, nil), "no step: the job's first stage of the kind")
	assert.Nil(t, AppCategory(steps, cats, Rejected, nil), "closed: none")
	assert.Equal(t, "offer", *AppCategory(nil, cats, Offer, nil))
}

func TestANewCategoryNeverTakesARemovedOnesKey(t *testing.T) {
	before := StandardCategories()
	var in []Category
	for _, c := range before {
		if c.Key != "interview_2" { // removed...
			in = append(in, c)
		}
	}
	in = append(in, Category{Label: "Interview 2", Kind: Offer}) // ...and one added with a name that makes the same key
	got, errs := NormalizeCategories(in, before)
	require.Empty(t, errs)
	assert.Equal(t, "interview_2_2", got[len(got)-2].Key, "a key of its own")
	assert.Equal(t, []string{"interview_2"}, Removed(before, got), "the removed one is seen as removed (its use is checked)")
}

// A recorded move reads as today's category: the one recorded while the
// company keeps it, else its stage's standard one, else its stage's first.
// Reordering categories never moves past numbers.
func TestHistoryReadsMovesAgainstTodaysCategories(t *testing.T) {
	str := func(s string) *string { return &s }
	reordered := []Category{
		{Key: "technical_test", Kind: Interview}, {Key: Interview, Kind: Interview}, {Key: "interview_final", Kind: Interview},
		{Key: Offer, Kind: Offer},
	}
	h := historyOf(reordered)
	assert.Equal(t, Interview, h.categoryOf(Interview, nil), "before categories: the standard 1st Interview, not the first listed")
	assert.Equal(t, "technical_test", h.categoryOf(Interview, str("technical_test")))
	assert.Equal(t, Interview, h.categoryOf(Interview, str("interview_2")), "a round removed since: the stage's standard one")
	assert.Empty(t, h.categoryOf("rejected", nil), "a closed stage has none")

	custom := historyOf([]Category{{Key: "phone_screen", Kind: Interview}, {Key: "panel", Kind: Interview}})
	assert.Equal(t, "phone_screen", custom.categoryOf(Interview, nil), "without the standard one: the stage's first")
}
