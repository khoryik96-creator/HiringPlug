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
		if label, _, ok := strings.Cut(host, "."); ok && len(label) >= 4 {
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

// Blind returns an ad's blind title and text: the client named by
// description the first time (or "our client"), then "our client"; the
// hiring contact, emails, phone numbers and links removed.
func Blind(d AdDetails, c Client) (title string, text *string) {
	re := namesPattern(ClientNames(c))
	description := ""
	if d.ClientDescription != nil {
		description = strings.TrimSpace(*d.ClientDescription)
	}
	first := true
	id := redact.Identity{Names: c.Contacts}
	clean := func(s string) string {
		// Emails, links and phone numbers first: they may hold the client's name.
		return replaceNames(redact.ScrubWith(s, id, "the hiring manager"), re, description, &first)
	}
	title = replaceNames(d.Title, re, "", new(bool)) // a title never takes the description
	if d.Description != nil {
		t := clean(*d.Description)
		text = &t
	}
	return title, text
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
