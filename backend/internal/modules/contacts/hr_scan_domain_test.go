package contacts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHRSearchesStayShortEnoughForLinkedIn(t *testing.T) {
	assert.Equal(t, []string{`"talent acquisition" OR "recruiter" OR "recruitment"`, `"human resources" OR "HR" OR "people and culture"`}, HRQueries(HRDefaultKeywords))
	assert.Equal(t, []string{`"HR"`}, HRQueries([]string{` "HR" `, "hr", ""}), "quotes, spaces and repeats are tidied")
}

func TestHRPeopleAreRankedByHowLikelyTheyHire(t *testing.T) {
	for title, tier := range map[string]string{
		"Head of Talent Acquisition": "A", "Senior Recruiter at Maju Bank": "A", "HR Director": "A", "CHRO": "A",
		"HR Business Partner": "B", "Human Resources Manager": "B", "People & Culture Executive": "B",
		"HR Executive (Payroll)": "C", "HR Assistant": "C",
	} {
		assert.Equal(t, tier, HRTier(title), title)
	}
	assert.NotEmpty(t, HRHidden("Ex-HR Manager at Maju"))
	assert.NotEmpty(t, HRHidden("HR Intern"))
	assert.Empty(t, HRHidden("HR Manager"))
}

func TestAScanReadsEachSearchPageByPageThenMovesOn(t *testing.T) {
	q, p, done := HRNext(0, 1, 3, 2, true)
	assert.Equal(t, []any{0, 2, false}, []any{q, p, done})
	q, p, done = HRNext(0, 3, 3, 2, true) // its pages are read
	assert.Equal(t, []any{1, 1, false}, []any{q, p, done})
	q, p, done = HRNext(1, 1, 3, 2, false) // LinkedIn has no more
	assert.Equal(t, []any{1, 1, true}, []any{q, p, done})
}

func TestEachProfileIsKeptOnce(t *testing.T) {
	a := HRPerson{URL: "https://www.linkedin.com/in/a", Name: "Aina", Title: "HR Executive"}
	b := HRPerson{URL: "https://www.linkedin.com/in/b", Name: "Farah", Title: "Head of Talent Acquisition"}
	got := MergeHRPeople([]HRPerson{a}, []HRPerson{a, b})
	assert.Len(t, got, 2)
	SortHRPeople(got)
	assert.Equal(t, "Farah", got[0].Name, "tier A first")
}
