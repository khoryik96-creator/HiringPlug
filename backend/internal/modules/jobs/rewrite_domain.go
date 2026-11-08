package jobs

// AI rewrites of a blind ad (CV Studio's way), as pure functions: what the
// AI is given, and its answer turned into the blind ad's title and text.

import (
	"strings"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// The longest blind title and text an ad keeps (as for a person's edit).
const (
	MaxBlindTitle = 200
	MaxBlindText  = 20000
)

// MinRewriteText is the least job description CV Studio rewrites (it asks
// for more below 80 characters).
const MinRewriteText = 80

// RewriteSource is the ad as written, for the AI: its title, then its text.
func RewriteSource(d AdDetails) string {
	text := strings.TrimSpace(d.Title)
	if d.Description != nil && strings.TrimSpace(*d.Description) != "" {
		text += "\n\n" + strings.TrimSpace(*d.Description)
	}
	return text
}

var workModes = map[string]string{"onsite": "On-site", "hybrid": "Hybrid", "remote": "Remote"}

// WorkMode is how the AI is told the ad's work arrangement ("" when unknown).
func WorkMode(d AdDetails) string {
	if d.WorkArrangement == nil {
		return ""
	}
	return workModes[*d.WorkArrangement]
}

// RemoveAlso is what the AI is told to take out besides the client's name:
// the person's own list, the client's website and its hiring contact.
func RemoveAlso(own *string, c Client) string {
	var parts []string
	if own != nil && strings.TrimSpace(*own) != "" {
		parts = append(parts, strings.TrimSpace(*own))
	}
	if c.Website != "" {
		parts = append(parts, c.Website)
	}
	if len(c.Contacts) > 0 && c.Contacts[0] != "" {
		parts = append(parts, c.Contacts[0]) // the full name
	}
	return strings.Join(parts, ", ")
}

// RewriteText is the blind ad's title and text from the AI's rewrite, in
// the sections CV Studio shows (About our client, Tech stack, What you will
// do, What you need to succeed, Nice to have, Why join us), as plain text
// with "• " bullets (the ad's own format).
func RewriteText(jd api.BlindJd, fallbackTitle string) (title, text string) {
	title = clipText(oneLine(jd.JobTitle), MaxBlindTitle)
	if title == "" {
		title = fallbackTitle
	}
	var b strings.Builder
	section := func(heading string, body string) {
		if body = strings.TrimSpace(body); body == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if heading != "" {
			b.WriteString(heading + "\n")
		}
		b.WriteString(body)
	}
	bullets := func(items []string) string {
		var lines []string
		for _, item := range items {
			if item = oneLine(item); item != "" {
				lines = append(lines, "• "+item)
			}
		}
		return strings.Join(lines, "\n")
	}
	about := strings.TrimSpace(jd.AboutClient)
	if role := strings.TrimSpace(jd.AboutRole); role != "" {
		if about != "" {
			about += "\n\n"
		}
		about += role
	}
	section("About our client", about)
	var stack []string
	for _, t := range jd.TechStack {
		if t = oneLine(t); t != "" {
			stack = append(stack, t)
		}
	}
	if len(stack) > 0 {
		section("", "Tech stack: "+strings.Join(stack, ", "))
	}
	section("What you will do", bullets(jd.Responsibilities))
	section("What you need to succeed", bullets(jd.Requirements))
	section("Nice to have", bullets(jd.NiceToHave))
	section("Why join us", bullets(jd.WhyJoin))
	return title, clipLines(b.String(), MaxBlindText)
}

// clipLines keeps text within n characters, ending at a whole line when it
// has to be cut.
func clipLines(text string, n int) string {
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	cut := clipText(text, n)
	if i := strings.LastIndex(cut, "\n"); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
