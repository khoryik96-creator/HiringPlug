package contacts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameCompany(t *testing.T) {
	same := [][2]string{
		{"Maju Holdings Sdn. Bhd.", "MAJU (M) SDN BHD"},
		{"Petronas", "PETRONAS Chemicals Group Berhad"},
		{"Contoh Recruitment", "contoh recruitment sdn bhd"},
		{"Grab", "Grab Holdings Inc."},
	}
	for _, c := range same {
		assert.True(t, SameCompany(c[0], c[1]), "%q = %q", c[0], c[1])
	}
	different := [][2]string{
		{"Maju Sdn Bhd", "Majulah Tech Sdn Bhd"},
		{"Maybank", "CIMB Bank Berhad"},
		{"Shopee Malaysia", "Lazada Malaysia"},
		{"Bank Rakyat", "Rakyat Insurance"}, // words out of order is another company
	}
	for _, c := range different {
		assert.False(t, SameCompany(c[0], c[1]), "%q != %q", c[0], c[1])
	}
}

func TestCompare(t *testing.T) {
	p := Profile{
		Name: ptr("Siti Aisyah binti Ahmad"), URL: ptr("my.linkedin.com/in/siti-aisyah/"),
		Positions: []Position{
			{Title: ptr("Talent Acquisition Lead"), Company: ptr("Lazada Malaysia"), StartDate: ptr("2026-06"), Current: true},
			{Title: ptr("HR Manager"), Company: ptr("Shopee Malaysia Sdn Bhd"), StartDate: ptr("2021-03"), EndDate: ptr("2026-05")},
		},
	}
	e := Compare(p, "Shopee Malaysia", "Siti", "Aisyah", ptr("https://www.linkedin.com/in/siti-aisyah"))
	assert.Equal(t, "moved", e.Verdict)
	assert.Equal(t, "Lazada Malaysia", *e.Current.Company)
	assert.True(t, *e.NameMatches)
	assert.True(t, *e.URLMatches)

	assert.Equal(t, "same_company", Compare(p, "Lazada", "Siti", "Aisyah", nil).Verdict)
	assert.Nil(t, Compare(p, "Lazada", "Siti", "Aisyah", nil).URLMatches, "nothing to compare with")
	assert.False(t, *Compare(p, "Lazada", "Nur", "Aina", ptr("linkedin.com/in/nur-aina")).NameMatches)
	assert.False(t, *Compare(p, "Lazada", "Nur", "Aina", ptr("linkedin.com/in/nur-aina")).URLMatches)

	p.Positions[0].Current = false
	assert.Equal(t, "no_current_job", Compare(p, "Lazada", "Siti", "Aisyah", nil).Verdict)
	p.Positions = []Position{{Title: ptr("Consultant"), Current: true}}
	assert.Equal(t, "unknown", Compare(p, "Lazada", "Siti", "Aisyah", nil).Verdict, "a current job without a company")
}
