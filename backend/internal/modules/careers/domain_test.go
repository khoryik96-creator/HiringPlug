package careers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplyFormRules(t *testing.T) {
	a, errs := Form{FirstName: "  Siti   Aisyah ", Email: " Siti@Example.COM ", Phone: " 012-345 6789 ", Consent: "yes"}.Validate()
	assert.NoError(t, errs.Err())
	assert.Equal(t, "Siti Aisyah", a.FirstName)
	assert.Equal(t, "siti@example.com", a.Email)
	assert.Equal(t, "012-345 6789", *a.Phone)

	_, errs = Form{Email: "not-an-email", Consent: ""}.Validate()
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	assert.Equal(t, map[string]bool{"firstName": true, "email": true, "consent": true}, fields)

	assert.True(t, Form{Website: "http://spam.example"}.IsRobot())
	assert.False(t, Form{}.IsRobot())
}

func TestLimiterAllowsAFewApplicationsPerAddress(t *testing.T) {
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	l := newLimiter(func() time.Time { return now })
	for range burstMax {
		assert.True(t, l.Allow("203.0.113.7"))
	}
	assert.False(t, l.Allow("203.0.113.7"), "a burst is cut off")
	assert.True(t, l.Allow("198.51.100.1"), "other addresses are not affected")

	now = now.Add(burstWindow) // the burst window has passed
	for range dayMax - burstMax {
		assert.True(t, l.Allow("203.0.113.7"))
		now = now.Add(burstWindow)
	}
	assert.False(t, l.Allow("203.0.113.7"), "the daily limit holds across bursts")
	now = now.Add(dayWindow)
	assert.True(t, l.Allow("203.0.113.7"), "and resets after a day")
}
