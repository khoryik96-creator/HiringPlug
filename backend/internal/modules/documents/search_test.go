package documents

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func TestMarkedTextBecomesParts(t *testing.T) {
	parts, n := Highlights("Led \x01SAP\x02 \x01BW\x02 upgrades")
	assert.Equal(t, 2, n)
	assert.Equal(t, []api.DocumentHighlightPart{
		{Text: "Led "}, {Text: "SAP", Match: true}, {Text: " "}, {Text: "BW", Match: true}, {Text: " upgrades"},
	}, parts)
	parts, n = Highlights("no match here")
	assert.Equal(t, 0, n)
	assert.Equal(t, []api.DocumentHighlightPart{{Text: "no match here"}}, parts)
	parts, _ = Highlights("cut \x01SA")
	assert.Equal(t, []api.DocumentHighlightPart{{Text: "cut "}, {Text: "SA", Match: true}}, parts, "an unclosed mark runs to the end")
}
