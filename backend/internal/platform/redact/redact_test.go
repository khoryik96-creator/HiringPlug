package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubAndFound(t *testing.T) {
	id := IdentityOf("Ahmad bin Ali", "Ahmad.Ali@Mail.com", "+60 12-345 6789")
	assert.Equal(t, Identity{Full: []string{"Ahmad", "bin", "Ali"}, Names: []string{"Ahmad", "Ali"},
		Emails: []string{"ahmad.ali@mail.com"}, Phones: []string{"60123456789"}}, id)

	text := "Ahmad led audits. Email ahmad.ali@mail.com, call 012-345 6789, see linkedin.com/in/ahmad. 2019-2021 at Alibaba."
	assert.ElementsMatch(t, []string{"name", "email", "phone"}, Found(text, id))
	clean := Scrub(text, id)
	assert.Equal(t, "the candidate led audits. Email [email removed], call [phone removed], see [link removed] 2019-2021 at Alibaba.", clean)
	assert.Empty(t, Found(clean, id))
	assert.Empty(t, IdentityOf("", "", "123").Phones, "too short to be a phone")
}

// A single name word that is also a month, a noun, a place, a lineage word
// or a verb is not the person (CV Studio's rules): a blind CV holding it must
// not be refused, nor the text rewritten.
func TestOrdinaryWordsInANameAreNotThePerson(t *testing.T) {
	for _, c := range []struct{ name, text string }{
		{"May Tan Siew Ling", "Account Executive, Jan 2020 - May 2023."},
		{"Arun Jaya", "Based in Petaling Jaya, Selangor."},
		{"Grace Lim", "Handled a 30-day grace period for invoices."},
		{"Muhammad Hafiz bin Abdul Rahman", "Worked with Abdul's team on audits."},
		{"Daniel Do", "Able to do month-end closing."},
		{"Siti Nur binti Rahman", "Reported to Aminah binti Rahman."},
		{"Ruby Wong", "Built apps with Ruby on Rails."},
	} {
		id := IdentityOf(c.name, "", "")
		assert.Empty(t, Found(c.text, id), c.name)
		assert.Equal(t, c.text, Scrub(c.text, id), c.name)
	}
}

func TestTheNameIsFoundWhereItReadsAsThePerson(t *testing.T) {
	for _, c := range []struct{ name, text, scrubbed string }{
		{"May Tan Siew Ling", "MAY TAN SIEW LING\nSenior Auditor", "the candidate\nSenior Auditor"},
		{"May Tan Siew Ling", "Siew Ling led the audit.", "the candidate the candidate led the audit."},
		{"Arun Jaya", "Contact Arun for details.", "Contact the candidate for details."},
		{"Grace Lim", "grace lim, auditor", "the candidate, auditor"},
		{"Vinay Lariya", "Vinay's team; Lariya-led review.", "the candidate's team; the candidate-led review."},
		{"Dr. Vinay Lariya", "Dr. Vinay Lariya", "Dr. the candidate"},
		{"Suharto", "Suharto led delivery.", "the candidate led delivery."},
		{"May", "May led delivery from May 2023.", "the candidate led delivery from May 2023."},
	} {
		id := IdentityOf(c.name, "", "")
		assert.Equal(t, []string{"name"}, Found(c.text, id), c.name)
		assert.Equal(t, c.scrubbed, Scrub(c.text, id), c.name)
	}
}

func TestNamesInScriptsWithoutCapitalsAreFound(t *testing.T) {
	chinese := IdentityOf("陈 伟明", "", "")
	assert.Equal(t, "Contact the candidate for details", Scrub("Contact 伟明 for details", chinese))
	assert.Equal(t, "推荐the candidate担任经理", Scrub("推荐伟明担任经理", chinese), "Chinese has no spaces between words")
	assert.Equal(t, []string{"name"}, Found("由伟明负责", chinese))

	tamil := IdentityOf("Kumar முருகன்", "", "")
	assert.Equal(t, "Referee: the candidate", Scrub("Referee: முருகன்", tamil))
	assert.Equal(t, "முருகன்கள்", Scrub("முருகன்கள்", tamil), "inside a longer Tamil word it is not the name")

	assert.Equal(t, "a grace period", Scrub("a grace period", IdentityOf("Grace Tan", "", "")), "Latin keeps its rule: capitals only")
}
