package emails

// WhatsApp messages, as pure rules: the template rules, the fields a message
// may use (never salary: a message sent is kept as a note), and the number a
// chat link (https://wa.me/<number>) opens.

import (
	"slices"
	"strings"

	"github.com/nyaruka/phonenumbers"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

const maxWhatsAppBody = 4000

// salaryPlaceholders are never filled in a WhatsApp message.
var salaryPlaceholders = []string{
	"candidate.currentSalary", "candidate.expectedSalary", "candidate.salaryNotes",
	"candidate.currentBreakdown", "candidate.expectedNotes",
}

// WhatsAppPlaceholders are the placeholders a WhatsApp template may use: the
// email ones, salary aside, and none about an interview.
var WhatsAppPlaceholders = slices.DeleteFunc(slices.Clone(Placeholders), func(p string) bool {
	return slices.Contains(salaryPlaceholders, p) || strings.HasPrefix(p, "interview.")
})

// WhatsAppTemplate is a message to start from.
type WhatsAppTemplate struct{ Name, Body string }

// NormalizeWhatsAppTemplate tidies a template and checks its placeholders.
func NormalizeWhatsAppTemplate(t WhatsAppTemplate) (WhatsAppTemplate, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	t.Name = strings.Join(strings.Fields(t.Name), " ")
	t.Body = strings.TrimSpace(t.Body)
	if t.Name == "" {
		errs.Add("name", "is required")
	}
	switch {
	case t.Body == "":
		errs.Add("body", "is required")
	case len([]rune(t.Body)) > maxWhatsAppBody:
		errs.Add("body", "is too long (at most 4,000 characters)")
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(t.Body, -1) {
		if !slices.Contains(WhatsAppPlaceholders, m[1]) {
			errs.Add("body", "{{"+m[1]+"}} cannot be used in WhatsApp; use one of: "+strings.Join(WhatsAppPlaceholders, ", "))
			break
		}
	}
	return t, errs
}

// WithoutSalary drops the salary values from a draft's values, so they stay
// in brackets.
func WithoutSalary(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		if !slices.Contains(salaryPlaceholders, k) {
			out[k] = v
		}
	}
	return out
}

// ChatNumber is a phone number as a WhatsApp chat link takes it: country code
// and number, digits only (a number without a country code is region's); ok
// is false when it is not a valid number.
func ChatNumber(number, region string) (string, bool) {
	parsed, err := phonenumbers.Parse(number, region)
	if err != nil || !phonenumbers.IsValidNumber(parsed) {
		return "", false
	}
	return strings.TrimPrefix(phonenumbers.Format(parsed, phonenumbers.E164), "+"), true
}

// WhatsAppStarters are the WhatsApp templates every company begins with.
var WhatsAppStarters = []WhatsAppTemplate{
	{
		Name: "Introduce a role",
		Body: "Hi {{candidate.firstName}}, this is {{sender.name}} from {{agency.name}}. I have a {{job.title}} role at {{company.name}} " +
			"that may suit you. Are you open to a quick call this week?",
	},
	{
		Name: "Ask for an updated CV",
		Body: "Hi {{candidate.firstName}}, {{sender.name}} from {{agency.name}} here. Could you send me your latest CV? " +
			"I would like to put you forward for {{job.title}}.",
	},
	{
		Name: "Follow up",
		Body: "Hi {{candidate.firstName}}, just following up on my last message about {{job.title}}. Let me know a good time to talk. " +
			"Thank you! {{sender.name}}",
	},
}
