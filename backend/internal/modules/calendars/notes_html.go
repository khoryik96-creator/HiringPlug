package calendars

// Formatted notes: what HTML an appointment's or invitation's body may hold
// (text formatting, lists, alignment and links; nothing that runs or loads
// from elsewhere), and their plain text, which HiringPlug shows and compares
// with Outlook's.

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var notesPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "div", "span", "b", "strong", "i", "em", "u", "s", "strike", "del",
		"ul", "ol", "li", "blockquote", "h1", "h2", "h3", "h4", "hr", "font", "sub", "sup")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("align").OnElements("p", "div")
	p.AllowStyles("color", "background-color", "font-family", "font-size", "font-weight", "font-style",
		"text-decoration", "text-align", "margin-left", "padding-left", "line-height").Globally()
	return p
}()

var (
	breakRe  = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|h[1-4]|blockquote|tr)>`)
	tagRe    = regexp.MustCompile(`<[^>]*>`)
	blanksRe = regexp.MustCompile(`[ \t\x{a0}]+`)
	linesRe  = regexp.MustCompile(`\n{3,}`)
)

// cleanNotes makes formatted notes safe, with their plain text: ("", "") for
// notes that hold no text.
func cleanNotes(raw string) (safe, text string) {
	safe = strings.TrimSpace(notesPolicy.Sanitize(raw))
	text = html.UnescapeString(tagRe.ReplaceAllString(breakRe.ReplaceAllString(safe, "\n"), ""))
	lines := strings.Split(blanksRe.ReplaceAllString(text, " "), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	text = strings.TrimSpace(linesRe.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
	if text == "" {
		return "", ""
	}
	return safe, text
}

// orNil is nil for "": no formatted notes.
func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
