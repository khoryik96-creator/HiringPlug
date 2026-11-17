package candidates

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWorkAuthorizationIsSuggestedFromWhatTheCVSays(t *testing.T) {
	local := SuggestWorkAuth("MY", "", "MY")
	require.NotNil(t, local)
	assert.Equal(t, "local", local.Status)
	assert.Contains(t, local.Reason, "Malaysia")

	for text, basis := range map[string]string{
		"Holder of RP-T since 2022": "rp_t", "Residence Pass-Talent holder": "rp_t",
		"On a spouse visa": "spouse_visa", "Dependant pass (DP)": "spouse_visa",
		"Malaysian permanent resident": "permanent_resident", "PR holder": "permanent_resident",
	} {
		s := SuggestWorkAuth("IN", text, "MY")
		require.NotNil(t, s, text)
		assert.Equal(t, "expat_eligible", s.Status, text)
		assert.Equal(t, basis, *s.Basis, text)
	}
	needs := SuggestWorkAuth("IN", "Worked in Bangalore", "MY")
	require.NotNil(t, needs)
	assert.Equal(t, "requires_visa", needs.Status)
	assert.Nil(t, SuggestWorkAuth("", "Worked in Kuala Lumpur", "MY"), "not enough to tell")
	assert.Nil(t, SuggestWorkAuth("", "a PRT report", "MY"), "PRT is not RP-T")
}
