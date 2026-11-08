package cvformats

// Blind CVs, as pure functions: what identifies a candidate, taking it out of
// a CV's data, and checking a finished file for anything left.

import (
	"html"
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

var (
	tagRe = regexp.MustCompile(`<[^>]*>`)
	// In document order: a run's text, or where a line or space falls
	// (a paragraph's end, a text box's start, a tab, a break).
	textTokenRe = regexp.MustCompile(`<w:(?:t|delText|instrText)(?: [^>]*)?>([^<]*)</w:(?:t|delText|instrText)>|</w:p>|<w:p/>|<w:txbxContent[ >]|<w:(?:tab|br|cr)(?: [^>]*)?/>`)
	targetRe    = regexp.MustCompile(`Target="([^"]*)"`)
)

// fileText is a Word file's text as a reader sees it: each paragraph's runs
// joined (Word splits words across runs: "Ali" + "ce" is Alice), paragraphs
// and text boxes on their own lines (a text box sits inside a paragraph,
// whose text goes on after it), tabs and breaks as spaces; then its
// properties and every link's address.
func fileText(contents map[string][]byte) string {
	var all strings.Builder
	for name, b := range contents {
		switch {
		case strings.HasSuffix(name, ".rels"):
			for _, m := range targetRe.FindAllSubmatch(b, -1) {
				all.WriteString(html.UnescapeString(string(m[1])) + "\n")
			}
		case strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml"):
			for _, m := range textTokenRe.FindAllSubmatch(b, -1) {
				switch tok := string(m[0]); {
				case strings.HasPrefix(tok, "</w:p") || strings.HasPrefix(tok, "<w:p/") || strings.HasPrefix(tok, "<w:txbxContent"):
					all.WriteString("\n")
				case strings.HasPrefix(tok, "<w:tab") || strings.HasPrefix(tok, "<w:br") || strings.HasPrefix(tok, "<w:cr"):
					all.WriteString(" ")
				default:
					all.WriteString(html.UnescapeString(string(m[1])))
				}
			}
			all.WriteString("\n")
		case strings.HasSuffix(name, ".xml"): // properties: author, title, company...
			all.WriteString(html.UnescapeString(tagRe.ReplaceAllString(string(b), "\n")) + "\n")
		}
	}
	return all.String()
}

// Leaks returns what kinds of identifying detail a finished file still holds
// anywhere (text, headers, footers, properties, links): "name", "email",
// "phone". The name by CV Studio's rules (redact): "May 2023" is a date.
func Leaks(docx []byte, id Identity) ([]string, error) {
	_, contents, err := readParts(docx)
	if err != nil {
		return nil, err
	}
	return redact.Found(fileText(contents), id), nil
}
