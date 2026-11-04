// Package quote checks that a quote is text from its source (a CV, a job
// ad), ignoring case and spacing. What a reader (rules or an AI model) says
// it found is kept only if it quotes the source: CV parsing and CV vs job
// matching both use this check.
package quote

import (
	"strings"
	"unicode"
)

// Source is text prepared once for checking quotes against it.
type Source string

// typographic turns curly quotes and long dashes into plain ones, which is
// how people (and AI models) usually type what they copy.
var typographic = strings.NewReplacer(
	"‘", "'", "’", "'", "“", `"`, "”", `"`,
	"–", "-", "—", "-", "−", "-",
)

// Prepare lower-cases s, makes its quotes and dashes plain, and collapses
// whitespace.
func Prepare(s string) Source {
	return Source(strings.Join(strings.Fields(strings.ToLower(typographic.Replace(s))), " "))
}

// Has reports whether q is text from src, ignoring case, spacing, plain or
// curly quotes and dashes, a bullet or quote mark before q and sentence
// punctuation after it ("3 years in audit." quotes "- 3 years in audit").
// Nothing else is trimmed, so "C++" or ".NET" must be there as written. A
// quote with no letters or digits quotes nothing.
func (src Source) Has(q string) bool {
	p := strings.TrimRight(strings.TrimLeft(string(Prepare(q)), leading), trailing)
	return strings.IndexFunc(p, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 &&
		strings.Contains(string(src), p)
}

// What may stand around a quote without being part of it.
const (
	leading  = " -*\u2022\u00b7\"'"
	trailing = " .,;:!?\"'"
)
