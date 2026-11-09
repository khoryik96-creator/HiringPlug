// Package findquery reads what someone types into the candidate search box,
// following the rules of Kano's /find command:
//
//   - Several searches at once, split on commas ("ali abu, sam liew"), at
//     most six, the same one typed twice searched once.
//   - Each is an email (it has an @), a phone number (nothing but digits and
//   - ( ) . - and spaces) or a name; an incomplete email or phone number
//     says so instead of matching something.
//   - "name - company" (a dash with a space before it) looks only among
//     those who work or worked there; "Anne-Marie" stays a name.
//   - A phone number matches only the whole number, never its last digits
//     (see Numbers); an email matches exactly; names alike are scored with
//     NameScore.
package findquery

import (
	"regexp"
	"strings"
)

// MaxSearches is how many comma-separated searches are run at once.
const MaxSearches = 6

// Kind is what a search looks for.
type Kind string

// The kinds of search.
const (
	Name  Kind = "name"
	Email Kind = "email"
	Phone Kind = "phone"
)

// The messages for searches that cannot be run as typed.
const (
	ErrCompany = "Use name - company, for example ali abu - nintex."
	ErrEmail   = "Enter a complete email address, for example aaa@gmail.com."
	ErrPhone   = "Enter a complete phone number. You can include + and a country code."
)

// Search is one search: what to look for and, optionally, where they work
// or worked. Error is set when it cannot be run as typed.
type Search struct {
	Text    string // as typed, spaces tidied
	Kind    Kind
	Value   string // the name, the email or the phone number as typed
	Company string
	Error   string
}

// Split returns the searches in raw, one per comma, and how many were left
// out past MaxSearches.
func Split(raw string) (searches []string, skipped int) {
	seen := map[string]bool{}
	for _, piece := range strings.Split(raw, ",") {
		s := tidy(piece)
		key := strings.ToLower(s)
		if s == "" || seen[key] {
			continue
		}
		seen[key] = true
		if len(searches) == MaxSearches {
			skipped++
			continue
		}
		searches = append(searches, s)
	}
	return searches, skipped
}

func tidy(s string) string { return strings.Join(strings.Fields(s), " ") }

var (
	phoneChars = regexp.MustCompile(`^[+\d\s().-]+$`)
	// A phone number, then " - company": the whole number first, so the
	// dashes inside it are not taken for the company's.
	phoneThenCompany = regexp.MustCompile(`^([+\d\s().-]+)\s+[-–—](?:\s+|$)(.*)$`)
	nameThenCompany  = regexp.MustCompile(`^(.*?)\s+[-–—](?:\s+|$)(.*)$`)
	leadingDash      = regexp.MustCompile(`^[-–—]\s`)
	completeEmail    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	completePhone    = regexp.MustCompile(`^\+?\d{4,15}$`)
	plusFirst        = regexp.MustCompile(`^\+?[\d\s().-]+$`)
)

// Parse reads one search (one of Split's).
func Parse(raw string) Search {
	text := tidy(raw)
	if phoneChars.MatchString(text) { // a spaced dash in a phone number is formatting
		return identify(text, text, "")
	}
	m := phoneThenCompany.FindStringSubmatch(text)
	if m == nil {
		m = nameThenCompany.FindStringSubmatch(text)
	}
	if m == nil && !leadingDash.MatchString(text) {
		return identify(text, text, "")
	}
	var value, company string
	if m != nil {
		value, company = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	} else {
		company = strings.TrimSpace(text[len(leadingDash.FindString(text)):])
	}
	s := identify(text, value, company)
	if value == "" || CompanyKey(company) == "" {
		s.Error = ErrCompany
	}
	return s
}

func identify(text, value, company string) Search {
	s := Search{Text: text, Kind: Name, Value: value, Company: company}
	if email := strings.ReplaceAll(value, `\@`, "@"); strings.Contains(email, "@") {
		s.Kind, s.Value = Email, email
		if !completeEmail.MatchString(email) {
			s.Error = ErrEmail
		}
		return s
	}
	if value != "" && phoneChars.MatchString(value) {
		s.Kind = Phone
		if !completePhone.MatchString(cleanPhone(value)) || !plusFirst.MatchString(value) {
			s.Error = ErrPhone
		}
	}
	return s
}
