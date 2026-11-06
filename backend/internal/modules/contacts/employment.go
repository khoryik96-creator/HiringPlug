package contacts

// Where a contact works now, read from their LinkedIn profile saved as PDF,
// and how it compares with the company they are recorded at. Pure rules.

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/linkedin"
)

// Position is a job on a profile.
type Position struct {
	Title     *string `json:"title"`
	Company   *string `json:"company"`
	StartDate *string `json:"startDate"`
	EndDate   *string `json:"endDate"`
	Current   bool    `json:"current"`
}

// Profile is what is kept of a LinkedIn profile: the name, the address and
// the jobs (the latest first). Never the file.
type Profile struct {
	Name      *string    `json:"name"`
	URL       *string    `json:"url"`
	Positions []Position `json:"positions"`
}

// Employment is a profile compared with the contact.
type Employment struct {
	Verdict     string    // same_company, moved, no_current_job, unknown
	Current     *Position // their current job
	NameMatches *bool
	URLMatches  *bool
}

const maxPositions = 10

// Current is the profile's current job: the first without an end date.
func (p Profile) Current() *Position {
	for i := range p.Positions {
		if p.Positions[i].Current {
			return &p.Positions[i]
		}
	}
	return nil
}

// Compare says whether the profile's current job is at the contact's
// company, and whether it is the contact's profile at all.
func Compare(p Profile, companyName, firstName, lastName string, contactURL *string) Employment {
	out := Employment{Verdict: "no_current_job", Current: p.Current()}
	if out.Current != nil {
		out.Verdict = "moved"
		if out.Current.Company == nil {
			out.Verdict = "unknown"
		} else if SameCompany(*out.Current.Company, companyName) {
			out.Verdict = "same_company"
		}
	}
	if p.Name != nil {
		m := sameName(*p.Name, firstName, lastName)
		out.NameMatches = &m
	}
	if p.URL != nil && contactURL != nil {
		a, okA := linkedin.Normalize(*p.URL)
		b, okB := linkedin.Normalize(*contactURL)
		if okA && okB && !strings.Contains(a, "lnkd.in") && !strings.Contains(b, "lnkd.in") {
			m := strings.EqualFold(a, b)
			out.URLMatches = &m
		}
	}
	return out
}

// companyWords are the parts of a company's name that say what kind of
// company it is, not which: left out when comparing names.
var companyWords = map[string]bool{
	"sdn": true, "bhd": true, "berhad": true, "pte": true, "ltd": true, "limited": true, "inc": true,
	"incorporated": true, "llc": true, "llp": true, "plc": true, "corp": true, "corporation": true,
	"co": true, "company": true, "the": true, "m": true, "malaysia": true, "singapore": true,
	"group": true, "holdings": true, "pvt": true, "gmbh": true, "ag": true, "sa": true, "bv": true,
}

var nonWord = regexp.MustCompile(`[^\pL\pN]+`)

func companyKey(name string) []string {
	var out []string
	for _, w := range nonWord.Split(strings.ToLower(name), -1) {
		if w != "" && !companyWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// SameCompany reports whether two company names name the same company:
// "Maju Holdings Sdn. Bhd." and "MAJU (M) SDN BHD" do; "Maju" and "Majulah
// Tech" do not. One name's words must all be in the other, in order.
func SameCompany(a, b string) bool {
	ka, kb := companyKey(a), companyKey(b)
	if len(ka) == 0 || len(kb) == 0 {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	if len(ka) > len(kb) {
		ka, kb = kb, ka
	}
	return containsInOrder(kb, ka)
}

func containsInOrder(long, short []string) bool {
	i := 0
	for _, w := range long {
		if i < len(short) && w == short[i] {
			i++
		}
	}
	return i == len(short)
}

// sameName reports whether the profile's name has the contact's names
// ("Siti Aisyah binti Ahmad" has "Siti" and "Aisyah").
func sameName(profile, first, last string) bool {
	have := strings.FieldsFunc(strings.ToLower(profile), func(r rune) bool { return !unicode.IsLetter(r) })
	want := strings.FieldsFunc(strings.ToLower(first+" "+last), func(r rune) bool { return !unicode.IsLetter(r) })
	if len(want) == 0 {
		return false
	}
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}
