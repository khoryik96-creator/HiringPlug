package quote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuotesMustBeSourceText(t *testing.T) {
	text := Prepare("SITI  AISYAH\nSenior   Accountant | ABC Sdn Bhd\n• 3–5 years in audit; “Big 4” preferred.")
	assert.True(t, text.Has("Siti Aisyah"), "case and spacing may differ")
	assert.True(t, text.Has("Senior Accountant | ABC"))
	assert.True(t, text.Has(`3-5 years in audit; "Big 4" preferred`), "plain dashes and quotes for typographic ones")
	assert.True(t, text.Has("- 3-5 years in audit."), "punctuation at the ends")
	assert.False(t, text.Has("Big Bank"))
	assert.False(t, text.Has("5 years in audit and tax"), "every word must be there")
	assert.False(t, text.Has("   "))
	assert.False(t, text.Has(" - . "), "no letters or digits, nothing quoted")
	assert.True(t, text.Has(`"Senior Accountant"`), "quote marks around the quote")

	skills := Prepare("Skills: C, Java, network admin")
	assert.False(t, skills.Has("C++"), "signs that belong to a word are kept")
	assert.False(t, skills.Has(".NET"))
	assert.True(t, Prepare("C++ and .NET").Has(".NET."))
}
