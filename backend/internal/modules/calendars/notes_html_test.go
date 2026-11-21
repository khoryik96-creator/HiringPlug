package calendars

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormattedNotesKeepFormattingOnly(t *testing.T) {
	safe, text := cleanNotes(`<p>Hello&nbsp;<b>Nadia</b></p><p><br></p><ol><li>Bring your IC</li></ol><a href="javascript:x()">click</a><iframe src="https://x.test"></iframe>`)
	assert.Equal(t, "<p>Hello\u00a0<b>Nadia</b></p><p><br></p><ol><li>Bring your IC</li></ol>click", safe)
	assert.Equal(t, "Hello Nadia\n\nBring your IC\nclick", text)

	safe, text = cleanNotes(`<p><br></p><div> </div><script>alert(1)</script>`)
	assert.Empty(t, safe, "no text: no formatted notes")
	assert.Empty(t, text)
}
