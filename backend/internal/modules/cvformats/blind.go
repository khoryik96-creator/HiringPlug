package cvformats

// Blind CVs, as pure functions: what identifies a candidate, taking it out of
// a CV's data, and checking a finished file for anything left.

import (
	"regexp"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
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
type Identity = redact.Identity

// IdentityOf gathers a candidate's names, email and phone.
func IdentityOf(c Candidate) Identity {
	return redact.IdentityOf(c.FirstName+" "+c.LastName, val(c.Email), val(c.Phone))
}

// Scrub takes identifying details out of free text.
func Scrub(text string, id Identity) string { return redact.Scrub(text, id) }

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
	return redact.Found(all.String(), id), nil
}
