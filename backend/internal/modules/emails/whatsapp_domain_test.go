package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChatNumber(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"012-345 6789", "60123456789", true},    // local, read as Malaysian
		{"+60 12-345 6789", "60123456789", true}, // with its country code
		{"+65 9123 4567", "6591234567", true},    // another country's
		{"12", "", false},
		{"call me", "", false},
	} {
		got, ok := ChatNumber(tc.in, "MY")
		assert.Equal(t, tc.ok, ok, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}
}

func TestWhatsAppTemplatesLeaveOutSalary(t *testing.T) {
	assert.NotContains(t, WhatsAppPlaceholders, "candidate.expectedSalary")
	assert.NotContains(t, WhatsAppPlaceholders, "interview.when")
	assert.Contains(t, WhatsAppPlaceholders, "candidate.firstName")

	_, errs := NormalizeWhatsAppTemplate(WhatsAppTemplate{Name: "Pay", Body: "{{candidate.currentSalary}}"})
	assert.NotEmpty(t, errs)
	tpl, errs := NormalizeWhatsAppTemplate(WhatsAppTemplate{Name: "  Hello   there ", Body: " Hi {{candidate.firstName}} "})
	assert.Empty(t, errs)
	assert.Equal(t, WhatsAppTemplate{Name: "Hello there", Body: "Hi {{candidate.firstName}}"}, tpl)
	_, errs = NormalizeWhatsAppTemplate(WhatsAppTemplate{Name: "", Body: ""})
	assert.Len(t, errs, 2)

	// Even a salary value among a draft's values is never filled.
	text, missing := Fill("{{candidate.firstName}}: {{candidate.expectedSalary}}", WithoutSalary(map[string]string{
		"candidate.firstName": "Siti", "candidate.expectedSalary": "RM 12,000 per month",
	}))
	assert.Equal(t, "Siti: [candidate.expectedSalary]", text)
	assert.Equal(t, []string{"candidate.expectedSalary"}, missing)
}

func TestWhatsAppStartersAreValid(t *testing.T) {
	for _, s := range WhatsAppStarters {
		_, errs := NormalizeWhatsAppTemplate(s)
		assert.Empty(t, errs, s.Name)
	}
}
