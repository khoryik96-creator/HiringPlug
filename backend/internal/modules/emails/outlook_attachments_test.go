package emails

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestOutlookAttachmentNamesAreSafeAndKeepTheirExtension(t *testing.T) {
	long := strings.Repeat("Nadia Rahman curriculum vitae ", 12) + "final.pdf"
	name := outlookAttachmentName(long)
	assert.True(t, strings.HasSuffix(name, "….pdf"), name)
	assert.Equal(t, 240, utf8.RuneCountInString(name))

	odd := strings.Repeat("x", 300) + "." + strings.Repeat("y", 30) // not an extension
	assert.Equal(t, 240, utf8.RuneCountInString(outlookAttachmentName(odd)))

	assert.Equal(t, "CV.pdf", outlookAttachmentName(`C:\uploads\CV.pdf`))
	assert.Equal(t, "a_b.docx", outlookAttachmentName("a\x00b.docx"))
	assert.Equal(t, "attachment", outlookAttachmentName(".."))
	assert.Equal(t, ".pdf", outlookAttachmentName(".pdf"))
}
