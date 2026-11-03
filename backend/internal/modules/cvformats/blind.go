package cvformats

// Blind CVs, as pure functions: what identifies a candidate, taking it out of
// a CV's data, and checking a finished file for anything left.

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Reference names a candidate in a blind CV: C- and the end of their id.
func Reference(candidateID string) string {
	id := strings.ToUpper(candidateID)
	if len(id) > 6 {
		id = id[len(id)-6:]
	}
	return "C-" + id
}

// Identity is what identifies a candidate in text.
type Identity struct {
	Names  []string // name parts, e.g. Siti, Aisyah
	Emails []string
	Phones []string // digits only
}

// Malay, Tamil, Arabic and Dutch name connectors are not identifying alone.
var connectors = []string{"bin", "binti", "bte", "bt", "a/l", "a/p", "al", "el", "van", "de", "der", "von", "s/o", "d/o"}

// IdentityOf gathers a candidate's names, email and phone.
func IdentityOf(c Candidate) Identity {
	var id Identity
	for _, part := range strings.Fields(c.FirstName + " " + c.LastName) {
		part = strings.Trim(part, ".,'\"()")
		if len([]rune(part)) >= 2 && !slices.Contains(connectors, strings.ToLower(part)) && !slices.Contains(id.Names, part) {
			id.Names = append(id.Names, part)
		}
	}
	if e := val(c.Email); e != "" {
		id.Emails = append(id.Emails, strings.ToLower(e))
	}
	if d := digits(val(c.Phone)); len(d) >= 7 {
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
)

var groupRe = regexp.MustCompile(`\d+`)

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

// nameRe matches a name part as a whole word, in any case.
func nameRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(name) + `($|[^\p{L}\p{N}])`)
}

// Scrub takes identifying details out of free text: emails, links, phone
// numbers (8 digits or more) and the candidate's names.
func Scrub(text string, id Identity) string {
	text = emailRe.ReplaceAllString(text, "[email removed]")
	text = linkRe.ReplaceAllString(text, "[link removed]")
	text = phoneRe.ReplaceAllStringFunc(text, func(m string) string {
		if isPhone(m) {
			return "[phone removed]"
		}
		return m
	})
	for _, n := range id.Names {
		text = nameRe(n).ReplaceAllString(text, "${1}the candidate${2}")
	}
	return text
}

// Blind makes a CV's data blind: a reference instead of the name, no contact
// details, nationality or work authorization (they hint at origin), and
// every other value scrubbed.
func Blind(d Data, c Candidate, ref string) Data {
	id := IdentityOf(c)
	values := make(map[string]string, len(d.Values))
	for k, v := range d.Values {
		values[k] = Scrub(v, id)
	}
	values["candidate.fullName"] = "Candidate " + ref
	values["candidate.reference"] = ref
	for _, k := range []string{"candidate.firstName", "candidate.lastName", "candidate.initials", "candidate.email", "candidate.phone",
		"candidate.nationality", "candidate.workAuthorization"} {
		values[k] = ""
	}
	lists := make(map[string][]map[string]string, len(d.Lists))
	for name, items := range d.Lists {
		for _, item := range items {
			clean := make(map[string]string, len(item))
			for k, v := range item {
				clean[k] = Scrub(v, id)
			}
			lists[name] = append(lists[name], clean)
		}
	}
	return Data{Values: values, Lists: lists}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// Leaks returns what kinds of identifying detail a finished file still holds
// anywhere (text, headers, footers, properties): "name", "email", "phone".
func Leaks(docx []byte, id Identity) ([]string, error) {
	_, contents, err := readParts(docx)
	if err != nil {
		return nil, err
	}
	var all strings.Builder
	for name, b := range contents {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			all.WriteString(tagRe.ReplaceAllString(string(b), " "))
			all.WriteString("\n")
		}
	}
	text := all.String()
	lower := strings.ToLower(text)
	var kinds []string
	for _, n := range id.Names {
		if nameRe(n).MatchString(text) {
			kinds = append(kinds, "name")
			break
		}
	}
	for _, e := range id.Emails {
		if strings.Contains(lower, e) {
			kinds = append(kinds, "email")
			break
		}
	}
	// Each number written in the file (not all digits run together: years
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
	return kinds, nil
}
