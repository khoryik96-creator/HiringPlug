package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValues(t *testing.T) {
	where := "Level 3, Menara Contoh"
	v := Values(Context{
		Candidate:   Candidate{FullName: "Siti Aisyah"},
		Application: &Application{JobTitle: "Accountant"},
		Interview:   &Interview{Mode: "in_person", Minutes: 30, Location: &where},
	})
	assert.Equal(t, "Siti Aisyah", v["candidate.firstName"], "the full name when there is no first name")
	assert.Equal(t, "In person", v["interview.how"])
	assert.Equal(t, where, v["interview.where"])
	_, hasCompany := v["company.name"]
	assert.False(t, hasCompany, "no company name: missing, not blank")
	assert.Equal(t, "", v["interview.instructions"], "no instructions: left out, not missing")
	body, missing := Fill("{{interview.how}}\n\n{{interview.instructions}}\n\nThanks", v)
	assert.Equal(t, "In person\n\nThanks", body)
	assert.Empty(t, missing)
}
