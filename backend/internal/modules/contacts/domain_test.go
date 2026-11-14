package contacts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckEmail(t *testing.T) {
	tests := []struct {
		email, mailServer, status string
	}{
		{"aisyah@maybank.com.my", MailServerFound, EmailValid},
		{"hr@maybank.com.my", MailServerFound, EmailRisky},
		{"someone@mailinator.com", MailServerFound, EmailRisky},
		{"aisyah@nomail.example", MailServerNone, EmailInvalid},
		{"aisyah@slow.example", MailServerFailed, EmailUnknown},
		{"siti..aisyah@maybank.com", MailServerFound, EmailInvalid},
		{"aisyah@localhost", MailServerFound, EmailInvalid},
	}
	for _, tc := range tests {
		status, reasons := CheckEmail(tc.email, tc.mailServer)
		assert.Equal(t, tc.status, status, tc.email)
		if status == EmailValid {
			assert.Empty(t, reasons, tc.email)
		} else {
			assert.NotEmpty(t, reasons, "%s: say why", tc.email)
		}
	}
	_, reasons := CheckEmail("hr@maybank.com.my", MailServerFound)
	assert.Equal(t, []string{"a shared mailbox (hr@), not a person"}, reasons)
}

func TestNormalize(t *testing.T) {
	d, errs := Normalize(Details{
		FirstName: "  Siti ", LastName: " Aisyah  binti  Ali", Email: ptr(" Siti.Aisyah@Maybank.COM "),
		LinkedinURL: ptr("linkedin.com/in/siti-aisyah/"), Title: ptr("  "),
	})
	require.NoError(t, errs.Err())
	assert.Equal(t, "Siti", d.FirstName)
	assert.Equal(t, "Aisyah binti Ali", d.LastName)
	assert.Equal(t, "siti.aisyah@maybank.com", *d.Email)
	assert.Equal(t, "https://www.linkedin.com/in/siti-aisyah", *d.LinkedinURL)
	assert.Nil(t, d.Title)
	assert.Equal(t, "active", d.Status)

	_, errs = Normalize(Details{FirstName: " ", Email: ptr("siti@"), LinkedinURL: ptr("facebook.com/siti")})
	fields := []string{}
	for _, e := range errs {
		fields = append(fields, e.Field)
	}
	assert.ElementsMatch(t, []string{"firstName", "email", "linkedinUrl"}, fields)
}
