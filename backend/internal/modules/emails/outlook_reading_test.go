package emails

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAReceivedMessageKeepsItsFormattingButNothingThatRunsOrLoads(t *testing.T) {
	raw := `<html><head><style>
@import url("https://evil.test/x.css");
.MsoNormal { margin: 0; font-family: Calibri; background: url(https://track.test/p.gif) }
</style></head><body>
<p class="MsoNormal" style="color:#1f497d;font-size:11pt" onclick="steal()">Hi <b>Samuel</b></p>
<table width="600" cellpadding="4" bgcolor="#ffffff"><tr><td style="border:1px solid #ccc">Role</td></tr></table>
<script>steal()</script><iframe src="https://evil.test"></iframe><form action="https://evil.test"><input name="p"></form>
<img src="cid:image001.png@01DB3A" alt="Hyppies logo" width="120">
<img src="https://track.test/open.gif" width="1" height="1">
<img src="http://plain.test/logo.png" alt="old">
<a href="https://hyppies.com">Hyppies</a> <a href="javascript:steal()">bad</a>
</body></html>`
	shown, blocked := readableHTML(raw, map[string]string{"image001.png@01db3a": "data:image/png;base64,AAAA"})

	assert.Contains(t, shown, `class="MsoNormal"`)
	assert.Contains(t, shown, `color: #1f497d`, "inline styles stay")
	assert.Contains(t, shown, `<b>Samuel</b>`)
	assert.Contains(t, shown, `cellpadding="4"`)
	assert.Contains(t, shown, `font-family: Calibri`, "the message's own styles stay")
	assert.Contains(t, shown, `src="data:image/png;base64,AAAA"`, "a picture sent with it is shown")
	assert.Contains(t, shown, `data-hp-src="https://track.test/open.gif"`, "a web picture is marked for the web app to load")
	assert.Contains(t, shown, `data-hp-src="https://plain.test/logo.png"`, "an old http address is tried over https")
	assert.Equal(t, 2, blocked)
	assert.Contains(t, shown, `target="_blank"`)
	for _, gone := range []string{"<script", "steal()", "onclick", "<iframe", "<form", "<input", "@import", "url(", "javascript:", "http://plain.test"} {
		assert.NotContains(t, shown, gone)
	}
	assert.NotRegexp(t, `\ssrc="https?:`, shown, "no picture loads from the web by itself")
}

func TestAStyleThatCouldEscapeIsDropped(t *testing.T) {
	assert.Empty(t, cleanCSS(`p { color: red } </style><script>x()</script>`))
	assert.Empty(t, cleanCSS(`p { width: expression(alert(1)) }`))
	assert.Equal(t, "p { color: red; background: none }", cleanCSS(`p { color: red; background: url('https://t.test/a') }`))
	shown, _ := readableHTML(`<style>p{color:red}</style ><p>x</p>`, nil)
	assert.Equal(t, 1, strings.Count(shown, "<style>"))
}

func TestContentIDsAreMatchedHoweverTheyAreWritten(t *testing.T) {
	ids := contentIDs(`<img src="cid:Logo.PNG@01"><img src='cid:<sig@x>'>`)
	assert.True(t, ids["logo.png@01"])
	assert.True(t, ids["sig@x"])
	shown, _ := readableHTML(`<img src="cid:missing@x" alt="logo">`, nil)
	assert.NotContains(t, shown, "src=", "a picture that could not be had shows its alt text")
}
