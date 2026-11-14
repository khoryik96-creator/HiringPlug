package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBouncedRecipientsAreTheAddressesABounceNoticeNames(t *testing.T) {
	recipients := []string{"Ana@Maju.test", "nur@maju.test", "ana@maju.test", " "}
	cases := []struct {
		name, subject, body string
		want                []string
	}{
		{"Exchange", "Undeliverable: Shortlist", "<p>Delivery has failed to these recipients:</p><a href=\"mailto:ana@maju.test\">ana@maju.test</a>", []string{"ana@maju.test"}},
		{"Gmail", "Delivery Status Notification (Failure)", "Address not found. Your message wasn't delivered to NUR@maju.test because the address couldn't be found.", []string{"nur@maju.test"}},
		{"Postfix", "Undelivered Mail Returned to Sender", "<ana@maju.test>: host mx.maju.test said: 550 5.1.1 User unknown\n<nur@maju.test>: 550", []string{"ana@maju.test", "nur@maju.test"}},
		{"Exim", "Mail delivery failed: returning message to sender", "  ana@maju.test\n    No such user here", []string{"ana@maju.test"}},
		{"an address at the end of a sentence", "Undeliverable: Hi", "Could not deliver to ana@maju.test.", []string{"ana@maju.test"}},
		{"entity-encoded HTML", "Undeliverable: Hi", "&lt;ana@maju.test&gt;", []string{"ana@maju.test"}},
		{"not a bounce notice", "Re: Undeliverable?", "ana@maju.test", nil},
		{"an out-of-office", "Automatic reply: Shortlist", "ana@maju.test is away", nil},
		{"a longer address containing it", "Undeliverable: Hi", "joana@maju.test, ana@maju.test.my, ana@maju.testing", nil},
		{"none of the recipients", "Undeliverable: Hi", "someone@else.test", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, BouncedRecipients(c.subject, c.body, recipients))
		})
	}
}

func TestContainsAddressMatchesWholeAddressesOnly(t *testing.T) {
	assert.True(t, containsAddress("ana@x.com", "ana@x.com"))
	assert.True(t, containsAddress("to: ana@x.com, nur@x.com", "nur@x.com"))
	assert.True(t, containsAddress("joana@x.com then ana@x.com", "ana@x.com"), "a later whole match counts")
	assert.False(t, containsAddress("joana@x.com", "ana@x.com"))
	assert.False(t, containsAddress("ana@x.com.my", "ana@x.com"))
	assert.False(t, containsAddress("ana@x.community", "ana@x.com"))
	assert.False(t, containsAddress("x.ana@x.com", "ana@x.com"))
}
