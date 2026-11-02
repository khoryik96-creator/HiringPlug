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

func TestChannels(t *testing.T) {
	a, _ := Form{FirstName: "Li", Email: "li@example.com", Consent: "yes", Via: " LinkedIn "}.Validate()
	assert.Equal(t, "linkedin", a.Channel)
	a, _ = Form{FirstName: "Li", Email: "li@example.com", Consent: "yes", Via: "made-up"}.Validate()
	assert.Equal(t, "careers_page", a.Channel, "unknown or missing links count as the careers page")
	assert.Equal(t, "jobstreet", CandidateSource("jobstreet"))
	assert.Equal(t, "careers_page", CandidateSource("indeed"), "boards that are not candidate sources")
}

func TestFeed(t *testing.T) {
	loc, salary, typ := "Kuala Lumpur", "RM 8,000", "full_time"
	closes := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	out, err := Feed("Contoh & Co", "contoh-co", "https://hire.example", []Ad{{
		ID: "ad_1", Title: "Senior <Accountant>", Location: &loc, SalaryText: &salary, WorkType: &typ,
		Description: "Join us.\n- SAP", ClosesOn: &closes, PublishedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
	}}, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	assert.NoError(t, err)
	xml := string(out)
	assert.Contains(t, xml, "<publisher>Contoh &amp; Co</publisher>")
	assert.Contains(t, xml, "<title>Senior &lt;Accountant&gt;</title>", "text is escaped")
	assert.Contains(t, xml, "<url>https://hire.example/careers/contoh-co/ad_1?via=indeed</url>")
	assert.Contains(t, xml, "<jobtype>fulltime</jobtype>")
	assert.Contains(t, xml, "<expirationdate>2026-12-31</expirationdate>")
}
