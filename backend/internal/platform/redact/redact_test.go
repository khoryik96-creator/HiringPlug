package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubAndFound(t *testing.T) {
	id := IdentityOf("Ahmad bin Ali", "Ahmad.Ali@Mail.com", "+60 12-345 6789")
	assert.Equal(t, Identity{Names: []string{"Ahmad", "Ali"}, Emails: []string{"ahmad.ali@mail.com"}, Phones: []string{"60123456789"}}, id)

	text := "Ahmad led audits. Email ahmad.ali@mail.com, call 012-345 6789, see linkedin.com/in/ahmad. 2019-2021 at Alibaba."
	assert.ElementsMatch(t, []string{"name", "email", "phone"}, Found(text, id))
	clean := Scrub(text, id)
	assert.Equal(t, "the candidate led audits. Email [email removed], call [phone removed], see [link removed] 2019-2021 at Alibaba.", clean)
	assert.Empty(t, Found(clean, id))
	assert.Empty(t, IdentityOf("", "", "123").Phones, "too short to be a phone")
}
