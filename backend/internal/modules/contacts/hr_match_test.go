package contacts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTheLikeliestContactIsChosen(t *testing.T) {
	email := "farah@maju.example"
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	elsewhereSameName := HRContact{ID: "cont_a", CompanyID: "comp_other", Kind: HRMatchSameName, Email: &email, UpdatedAt: day}
	hereSimilar := HRContact{ID: "cont_b", CompanyID: "comp_here", Kind: HRMatchSimilarName, Email: &email, UpdatedAt: day}
	profileElsewhere := HRContact{ID: "cont_c", CompanyID: "comp_other", Kind: HRMatchProfile, UpdatedAt: day}

	best, more, ok := BestHRMatch([]HRContact{hereSimilar, elsewhereSameName, profileElsewhere}, "comp_here")
	assert.True(t, ok)
	assert.Equal(t, "cont_c", best.ID, "the same LinkedIn profile is them, wherever they are")
	assert.Equal(t, 2, more)

	best, _, _ = BestHRMatch([]HRContact{hereSimilar, elsewhereSameName}, "comp_here")
	assert.Equal(t, "cont_a", best.ID, "the same name before a similar one")

	left := elsewhereSameName
	left.ID, left.CompanyID, left.Left = "cont_d", "comp_here", true
	stillThere := elsewhereSameName
	stillThere.ID, stillThere.CompanyID = "cont_e", "comp_here"
	noEmail := stillThere
	noEmail.ID, noEmail.Email, noEmail.UpdatedAt = "cont_f", nil, day.Add(time.Hour)
	best, _, _ = BestHRMatch([]HRContact{elsewhereSameName, left, noEmail, stillThere}, "comp_here")
	assert.Equal(t, "cont_e", best.ID, "at this company, still there, with an email")

	newer := stillThere
	newer.ID, newer.UpdatedAt = "cont_g", day.Add(time.Hour)
	best, _, _ = BestHRMatch([]HRContact{stillThere, newer}, "comp_here")
	assert.Equal(t, "cont_g", best.ID, "then the one saved last")

	_, _, ok = BestHRMatch(nil, "comp_here")
	assert.False(t, ok)
}

func TestProfileKeysIgnoreCaseAndHowTheAddressIsWritten(t *testing.T) {
	for _, url := range []string{"https://www.linkedin.com/in/Farah-Aziz/", "my.linkedin.com/in/farah-aziz", "http://linkedin.com/in/FARAH-AZIZ?trk=x"} {
		assert.Equal(t, "https://www.linkedin.com/in/farah-aziz", hrProfileKey(url), url)
	}
}
