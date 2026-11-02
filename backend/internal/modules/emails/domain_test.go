package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplates(t *testing.T) {
	_, errs := NormalizeTemplate(Template{Name: "x", Subject: "Hi {{candidate.salary}}", Body: "x"})
	assert.Contains(t, errs.Err().Error(), "")
	assert.Len(t, errs, 1, "unknown placeholders (and salary) are refused")
	for _, s := range Starters {
		_, errs := NormalizeTemplate(s)
		assert.NoError(t, errs.Err(), s.Name)
	}
	text, missing := Fill("Dear {{candidate.firstName}}, about {{ job.title }} at {{company.name}}.",
		map[string]string{"candidate.firstName": "Siti", "job.title": "Accountant"})
	assert.Equal(t, "Dear Siti, about Accountant at [company.name].", text)
	assert.Equal(t, []string{"company.name"}, missing)
	assert.True(t, ForClient("Submit candidate to client"))
	assert.False(t, ForClient("Interview invitation"))
}

func TestOutgoing(t *testing.T) {
	o, errs := NormalizeOutgoing(Outgoing{To: []string{" Siti@Example.com ", "siti@example.com"}, Subject: "Hi\nthere", Body: " x "})
	assert.NoError(t, errs.Err())
	assert.Equal(t, []string{"siti@example.com"}, o.To)
	assert.Equal(t, "Hi there", o.Subject, "no line breaks in a subject")
	_, errs = NormalizeOutgoing(Outgoing{To: []string{"not an address"}, Subject: "x", Body: "x"})
	assert.Len(t, errs, 2) // the bad address, and so no recipient
}
