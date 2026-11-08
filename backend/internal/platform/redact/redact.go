// Package redact takes what identifies a person out of free text (emails,
// links, phone numbers and their names), and finds it in text. Blind CVs and
// what the AI assistant may see use it.
package redact

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Identity is what identifies a person in text.
type Identity struct {
	Full   []string // the whole name's words in order, e.g. Siti, Aisyah, binti, Rahman
	Names  []string // its words that may name the person alone (no lineage words), e.g. Siti, Aisyah, Rahman
	Emails []string // lower case
	Phones []string // digits only
}

// IdentityOf gathers a person's name parts, email and phone (empty ones are
// left out).
func IdentityOf(name, email, phone string) Identity {
	var id Identity
	id.Full = splitName(name)
	for _, word := range id.Full {
		if isParticle(word) {
			continue
		}
		// A double-barrelled name stands alone in either form: Smith-Jones and Jones.
		for _, part := range append([]string{word}, strings.Split(word, "-")...) {
			if utf8.RuneCountInString(part) >= 2 && !slices.Contains(id.Names, part) {
				id.Names = append(id.Names, part)
			}
		}
	}
	if e := strings.TrimSpace(email); e != "" {
		id.Emails = append(id.Emails, strings.ToLower(e))
	}
	if d := digits(phone); len(d) >= 7 {
		id.Phones = append(id.Phones, d)
	}
	return id
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

var (
	emailRe = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	linkRe  = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|\b(?:linkedin\.com|facebook\.com|instagram\.com|github\.com|x\.com|twitter\.com)/\S*`)
	phoneRe = regexp.MustCompile(`\+?\d[\d\s().\-]{6,}\d`)
	groupRe = regexp.MustCompile(`\d+`)
)

// isPhone tells a phone number (8 digits or more) from runs of years such as
// 2019-2021 or "2017 2021".
func isPhone(m string) bool {
	if len(digits(m)) < 8 {
		return false
	}
	for _, g := range groupRe.FindAllString(m, -1) {
		if len(g) != 4 || (g[:2] != "19" && g[:2] != "20") {
			return true
		}
	}
	return false
}

// Scrub takes identifying details out of free text: every email, link and
// phone number (8 digits or more), and the person's name where it reads as
// them (names.go: CV Studio's rules), as "the candidate".
func Scrub(text string, id Identity) string { return ScrubWith(text, id, "the candidate") }

// ScrubWith is Scrub with the words that replace a name.
func ScrubWith(text string, id Identity, name string) string {
	text = emailRe.ReplaceAllString(text, "[email removed]")
	text = linkRe.ReplaceAllString(text, "[link removed]")
	text = phoneRe.ReplaceAllStringFunc(text, func(m string) string {
		if isPhone(m) {
			return "[phone removed]"
		}
		return m
	})
	return id.replaceNames(text, name)
}

// Found returns what kinds of the person's details text still holds:
// "name", "email", "phone".
func Found(text string, id Identity) []string {
	lower := strings.ToLower(text)
	var kinds []string
	if len(id.nameSpans(text)) > 0 {
		kinds = append(kinds, "name")
	}
	for _, e := range id.Emails {
		if strings.Contains(lower, e) {
			kinds = append(kinds, "email")
			break
		}
	}
	// Each number written in the text (not all digits run together: years
	// next to each other would look like anything).
	numbers := phoneRe.FindAllString(text, -1)
	for _, p := range id.Phones {
		tail := p
		if len(tail) > 8 {
			tail = tail[len(tail)-8:] // without the country code it is still the number
		}
		if slices.ContainsFunc(numbers, func(n string) bool { return isPhone(n) && strings.Contains(digits(n), tail) }) {
			kinds = append(kinds, "phone")
			break
		}
	}
	return kinds
}
