package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The blind ad without the AI kept everything but the client's name: "A
// regional investment bank is a leading Malaysian investment bank with 2,000
// staff across 5 countries" still points to one bank.
func TestSoftenTakesOutWhatPointsToTheClient(t *testing.T) {
	ad := AdDetails{Title: "Risk Analyst", Description: ptr("Contoh Capital is a leading Malaysian investment bank with 2,000 staff across 5 countries.\n- Report to Contoh Capital's Head of Risk"),
		ClientDescription: ptr("a regional investment bank")}
	assert.Equal(t, "Our client is an established Malaysian investment bank with a sizeable team across the region.\n- Report to our client's Head of Risk",
		*Blind(ad, Client{Name: "Contoh Capital Berhad"}).Description)
	for in, want := range map[string]string{
		"Founded in 1965 and listed on Bursa Malaysia, it runs over 200 branches.": "Well established and publicly listed, it runs a network of sites.",
		"The largest food maker in Asia, with revenue of RM 2.5 billion.":          "Established food maker in Asia, with revenue of a significant amount.",
		"A team of 300 people in 12 markets; the No. 1 brand since 1998.":          "A sizeable team across the region; an established brand well established.",
	} {
		assert.Equal(t, want, Soften(in), in)
	}
	// What the job needs stays as written: years of experience, the salary, team sizes they manage.
	for _, keep := range []string{"3+ years in credit risk at a bank", "Salary RM 8,000 - 10,000 per month", "Lead a team of 5 analysts"} {
		assert.Equal(t, keep, Soften(keep))
	}
}
