package meaning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A past job with no dates is not their job now: it ended no later than the
// next job began (or, with nothing known, it is a past job of unknown date).
func TestAPastJobWithNoDatesIsNotTheirJobNow(t *testing.T) {
	t.Parallel()
	src := Source{CandidateID: "cand_1", Work: []Work{
		{Position: 0, Title: "Finance Manager", StartDate: "2019", Current: true},
		{Position: 1, Title: "Recruitment Consultant"},
		{Position: 2, Title: "Sales Executive", StartDate: "2010", EndDate: "2014"},
		{Position: 3, Title: "Clerk"},
	}}
	roles := sourceForAI(src, "fp", nil).Roles
	require.Len(t, roles, 4)
	assert.True(t, roles[0].Current)
	assert.False(t, roles[1].Current, "a past job, though undated")
	end, err := roles[1].EndYear.Get()
	require.NoError(t, err)
	assert.Equal(t, 2019, end, "ended no later than the Finance Manager job began")
	assert.False(t, roles[3].Current)
	end, err = roles[3].EndYear.Get()
	require.NoError(t, err)
	assert.Equal(t, 2010, end, "ended no later than the Sales Executive job began")

	nothing := sourceForAI(Source{CandidateID: "cand_2", Work: []Work{
		{Position: 0, Title: "Accountant"}, {Position: 1, Title: "Audit Assistant"},
	}}, "fp", nil).Roles
	require.Len(t, nothing, 2)
	assert.True(t, nothing[0].Current, "their newest job, nothing saying it ended")
	assert.False(t, nothing[1].Current, "an older job is not their job now")
	assert.False(t, nothing[1].EndYear.IsSpecified() && !nothing[1].EndYear.IsNull(), "and when it ended is not known")
}

func TestIterativeScansCameInPgvector08(t *testing.T) {
	t.Parallel()
	for v, want := range map[string]bool{"0.8.0": true, "0.8.1": true, "0.10.2": true, "1.0": true, "0.7.4": false, "0.6.0": false, "": false, "x": false} {
		assert.Equal(t, want, versionAtLeast(v, 0, 8), v)
	}
}
