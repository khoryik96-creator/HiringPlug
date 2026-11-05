package jobs

// Blind job ads, as pure functions: the client's identity taken out of an
// ad (its name, website, hiring contact, and every email, phone number and
// link), and a check that a person's own blind text does not name it.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

// Client is what identifies a job's client in an ad.
type Client struct {
	Name     string   // e.g. Maybank Berhad
	Website  string   // e.g. https://www.maybank2u.com.my
	Contacts []string // the hiring contact's full name, then its parts
}

// legalSuffix is the end of a company's name that people often leave out.
var legalSuffix = regexp.MustCompile(`(?i)[\s,]+(sdn\.?\s*bhd\.?|berhad|bhd\.?|pte\.?\s*ltd\.?|ltd\.?|limited|inc\.?|plt|llc|corporation|corp\.?|co\.?)$`)

// suffixes are the endings of web addresses that are not the company's name
// (com.my, co.uk, ...).
var suffixes = map[string]bool{
	"com": true, "co": true, "net": true, "org": true, "gov": true, "edu": true, "biz": true, "info": true, "io": true, "ai": true,
	"my": true, "sg": true, "id": true, "th": true, "ph": true, "vn": true, "in": true, "hk": true, "tw": true, "cn": true,
	"jp": true, "kr": true, "au": true, "nz": true, "uk": true, "us": true, "ae": true,
}

// nameLabel is the part of a host that names the company: careers.acme.com.my
// gives acme, not careers.
func nameLabel(host string) string {
	labels := strings.Split(host, ".")
	for len(labels) > 1 && suffixes[labels[len(labels)-1]] {
		labels = labels[:len(labels)-1]
	}
	if len(labels) == 0 || len(labels) == len(strings.Split(host, ".")) {
		return "" // no known ending: not sure which part is the name
	}
	return labels[len(labels)-1]
}

// ClientNames are the ways an ad may name its client: the full name, the
// name without its legal ending, the website's address and its name part.
func ClientNames(c Client) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if len([]rune(s)) >= 3 && !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, s) }) {
			out = append(out, s)
		}
	}
	add(c.Name)
	core := c.Name
	for {
		next := legalSuffix.ReplaceAllString(core, "")
		if next == core {
			break
		}
		core = next
	}
	add(core)
	if u, err := url.Parse(c.Website); err == nil && u.Hostname() != "" {
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		add(host)
		if label := nameLabel(host); len(label) >= 4 {
			add(label)
		}
	}
	// Longest first, so "Maybank Berhad" goes before "Maybank".
	slices.SortStableFunc(out, func(a, b string) int { return len(b) - len(a) })
	return out
}

func namesPattern(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(` + strings.Join(quoted, "|") + `)('s)?($|[^\p{L}\p{N}])`)
}

// sentenceStart reports whether text before a position ends a sentence (or
// is empty), so a replacement there starts with a capital.
func sentenceStart(before string) bool {
	t := strings.TrimRightFunc(before, unicode.IsSpace)
	return t == "" || strings.HasSuffix(before, "\n") || strings.ContainsAny(t[len(t)-1:], ".!?:")
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// replaceNames puts description (first) and "our client" (after) where the
// client is named. first reports whether the description is still unused.
func replaceNames(text string, re *regexp.Regexp, description string, first *bool) string {
	if re == nil {
		return text
	}
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		// m: whole, lead, name, possessive, tail
		start, end := m[4], m[5]
		if m[6] >= 0 {
			end = m[7]
		}
		b.WriteString(text[last:start])
		words := "our client"
		if *first && description != "" {
			words, *first = description, false
		}
		if m[6] >= 0 {
			words += "'s"
		}
		if sentenceStart(text[:start]) {
			words = capitalize(words)
		}
		b.WriteString(words)
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

// Blind returns an ad's blind version: the client named by description the
// first time in the text (or "our client"), then "our client", in the title,
// text, location and salary line; the hiring contact, emails, phone numbers
// and links removed.
func Blind(d AdDetails, c Client) BlindView {
	re := namesPattern(ClientNames(c))
	description := ""
	if d.ClientDescription != nil {
		description = strings.TrimSpace(*d.ClientDescription)
	}
	first := true
	id := redact.Identity{Names: c.Contacts}
	clean := func(s string) string {
		// Emails, links and phone numbers first: they may hold the client's name.
		return replaceAll(redact.ScrubWith(s, id, "the hiring manager"), re, description, &first)
	}
	words := func(s *string) *string { // a title, a location, a salary line: never the description
		if s == nil {
			return nil
		}
		t := replaceAll(*s, re, "", new(bool))
		return &t
	}
	out := BlindView{Title: *words(&d.Title), Location: words(d.Location), SalaryText: words(d.SalaryText)}
	if d.Description != nil {
		t := clean(*d.Description)
		out.Description = &t
	}
	return out
}

// replaceAll replaces until no mention is left: a match takes the character
// after it, so "Maybank/Maybank" needs a second pass.
func replaceAll(text string, re *regexp.Regexp, description string, first *bool) string {
	for range 5 {
		next := replaceNames(text, re, description, first)
		if next == text {
			break
		}
		text = next
	}
	return text
}

// NamesClient reports whether text still names the client.
func NamesClient(text string, c Client) bool {
	re := namesPattern(ClientNames(c))
	return re != nil && re.MatchString(text)
}

// SourceHash identifies the ad a person's blind text was written for.
func SourceHash(d AdDetails) string {
	desc := ""
	if d.Description != nil {
		desc = *d.Description
	}
	sum := sha256.Sum256([]byte(d.Title + "\x00" + desc))
	return hex.EncodeToString(sum[:16])
}
