package emails

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWhenAndLength(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	assert.NoError(t, err)
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	assert.NoError(t, err)
	at := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	assert.Equal(t, "Tuesday, 10 November 2026, 10:00 AM (GMT+8)", When(at, kl))
	assert.Equal(t, "Tuesday, 10 November 2026, 7:30 AM (GMT+5:30)", When(at, kolkata))
	assert.Equal(t, "Tuesday, 10 November 2026, 2:00 AM (GMT)", When(at, nil))
	assert.Equal(t, "45 minutes", Length(45))
	assert.Equal(t, "1 hour", Length(60))
	assert.Equal(t, "2 hours 15 minutes", Length(135))
}

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
