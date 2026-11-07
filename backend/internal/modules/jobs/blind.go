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

// legalEnding is the end of a company's name that people often leave out or
// write differently (Sdn Bhd, Berhad, Bhd...).
const legalEnding = `(?:sdn\.?\s*bhd|berhad|bhd|pte\.?\s*ltd|ltd|limited|inc|plt|llc|corporation|corp|co)`

// legalSuffix is that ending at the end of a name.
var legalSuffix = regexp.MustCompile(`(?i)[\s,]+` + legalEnding + `\.?$`)

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

// genericWords are first words of company names too common to stand for the
// company alone ("Global" in "Global Logistics Sdn Bhd").
var genericWords = map[string]bool{
	"asia": true, "asian": true, "pacific": true, "global": true, "international": true, "national": true, "united": true,
	"general": true, "first": true, "new": true, "royal": true, "golden": true, "star": true, "prime": true, "smart": true,
	"digital": true, "standard": true, "malaysia": true, "malaysian": true, "singapore": true, "east": true, "west": true,
	"north": true, "south": true, "central": true, "eastern": true, "western": true, "southern": true, "northern": true,
	"great": true, "grand": true, "best": true, "top": true, "the": true, "bank": true, "group": true, "holdings": true,
	"tech": true, "technology": true, "technologies": true, "systems": true, "solutions": true, "services": true,
	"management": true, "consulting": true, "capital": true, "energy": true, "power": true, "green": true, "blue": true,
	"red": true, "apex": true, "alpha": true, "summit": true, "premier": true, "advanced": true, "modern": true, "future": true,
}

// ShortName is the first word of a longer client name, as a JD often calls
// the client ("Maju's HR Director" for Maju Foods Berhad); "" when that word
// is too short or too common to stand for the company alone. It is matched
// only as written with a capital, so the Malay word "maju" stays.
func ShortName(c Client) string {
	words := strings.Fields(strings.Trim(coreName(c.Name), " ,"))
	if len(words) < 2 {
		return ""
	}
	w := strings.Trim(words[0], ".,'&-()")
	if len([]rune(w)) < 4 || genericWords[strings.ToLower(w)] || !unicode.IsUpper([]rune(w)[0]) {
		return ""
	}
	return w
}

// coreName is a company's name without its legal ending.
func coreName(name string) string {
	core := name
	for {
		next := legalSuffix.ReplaceAllString(core, "")
		if next == core {
			return core
		}
		core = next
	}
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
	add(coreName(c.Name))
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

func namesPattern(names []string, short string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names), len(names)+1)
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	if short != "" { // as written with a capital: "Maju" or "MAJU", never "maju"
		r := []rune(strings.ToLower(short))
		title := string(unicode.ToUpper(r[0])) + string(r[1:])
		quoted = append(quoted, `(?-i:`+regexp.QuoteMeta(title)+`|`+regexp.QuoteMeta(strings.ToUpper(short))+`)`)
	}
	// After the name, the rest of the company's name as the text writes it,
	// up to its legal ending: "MAJU FOODS BHD" on record still takes all of
	// "Maju Foods Berhad", and "Maju" takes "Maju Foods (M) Sdn Bhd". Without
	// a legal ending only the name is taken ("Maju is hiring").
	rest := `(?:(?:[ \t]+(?-i:\p{Lu})[\p{L}&'.-]*){0,3}(?:[ \t]*\((?:M|Malaysia)\))?[ \t,]+` + legalEnding + `\.?)?` // on the same line
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])((?:` + strings.Join(quoted, "|") + `)` + rest + `)('s|’s|'|’)?($|[^\p{L}\p{N}])`)
}

// sentenceStart reports whether text before a position ends a sentence (or
// is empty), so a replacement there starts with a capital.
func sentenceStart(before string) bool {
	line := before[strings.LastIndex(before, "\n")+1:]
	if bulletOnly.MatchString(line) { // a line's start, or a bullet point's
		return true
	}
	t := strings.TrimRightFunc(before, unicode.IsSpace)
	return t == "" || strings.ContainsAny(t[len(t)-1:], ".!?:")
}

// bulletOnly is the start of a line up to its text: spaces, then perhaps a
// bullet ("-", "*", "•", "1.").
var bulletOnly = regexp.MustCompile(`^\s*(?:[-*•▪◦]|\d{1,2}[.)])?\s*$`)

func capitalize(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

// lowerArticle writes a description's leading A, An or The in lower case,
// for its place mid-sentence ("About a food maker").
func lowerArticle(s string) string {
	for _, a := range []string{"A ", "An ", "The "} {
		if strings.HasPrefix(s, a) {
			return strings.ToLower(a) + s[len(a):]
		}
	}
	return s
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
		// A lone apostrophe is a possessive only after an s ("Maju Foods' plants").
		possessive := m[6] >= 0 && (strings.HasSuffix(text[m[6]:m[7]], "s") || strings.HasSuffix(strings.ToLower(text[start:end]), "s"))
		if possessive {
			end = m[7]
		}
		b.WriteString(text[last:start])
		words := "our client"
		if *first && description != "" {
			words, *first = description, false
		}
		if possessive {
			words += "'s"
		}
		if sentenceStart(text[:start]) {
			words = capitalize(words)
		} else {
			words = lowerArticle(words)
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
	re := namesPattern(ClientNames(c), ShortName(c))
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
	re := namesPattern(ClientNames(c), ShortName(c))
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
