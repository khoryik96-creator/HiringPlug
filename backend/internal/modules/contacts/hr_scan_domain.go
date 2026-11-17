package contacts

// Find HR: the rules, as pure functions. A company's HR people are read from
// LinkedIn's people search (by the recruiter's companion) a page at a time;
// each page is its own address, so a failed page is simply read again.

import (
	"regexp"
	"sort"
	"strings"
)

// HRCountry is a country LinkedIn's people search can be narrowed to: its
// name as LinkedIn shows it in the filter, and LinkedIn's number for it.
type HRCountry struct{ Name, Geo string }

// HRCountries are the countries Find HR offers (the companion confirms the
// filter LinkedIn shows matches Name before anyone is read).
var HRCountries = map[string]HRCountry{
	"MY": {"Malaysia", "106808692"}, "SG": {"Singapore", "102454443"}, "ID": {"Indonesia", "102478259"},
	"PH": {"Philippines", "103121230"}, "TH": {"Thailand", "105146118"}, "VN": {"Vietnam", "104195383"},
	"HK": {"Hong Kong SAR", "103291313"}, "TW": {"Taiwan", "104187078"}, "CN": {"China", "102890883"},
	"JP": {"Japan", "101355337"}, "KR": {"South Korea", "105149562"}, "IN": {"India", "102713980"},
	"AU": {"Australia", "101452733"}, "NZ": {"New Zealand", "105490917"}, "AE": {"United Arab Emirates", "104305776"},
	"GB": {"United Kingdom", "101165590"}, "US": {"United States", "103644278"},
}

// HRDefaultKeywords are the job titles searched for when none are given.
var HRDefaultKeywords = []string{"talent acquisition", "recruiter", "recruitment", "human resources", "HR", "people and culture"}

// hrPerQuery: LinkedIn returns nothing for long ORs (CV Studio found it at
// eleven terms); three per search stays well inside.
const hrPerQuery = 3

// hrMaxPeople bounds what one scan keeps.
const hrMaxPeople = 400

// HRQueries are the searches a scan runs: a few keywords each, quoted.
func HRQueries(keywords []string) []string {
	var terms []string
	seen := map[string]bool{}
	for _, k := range keywords {
		k = strings.Join(strings.Fields(strings.ReplaceAll(k, `"`, "")), " ")
		if k != "" && !seen[strings.ToLower(k)] {
			seen[strings.ToLower(k)] = true
			terms = append(terms, k)
		}
	}
	var out []string
	for i := 0; i < len(terms); i += hrPerQuery {
		group := terms[i:min(i+hrPerQuery, len(terms))]
		quoted := make([]string, len(group))
		for j, t := range group {
			quoted[j] = `"` + t + `"`
		}
		out = append(out, strings.Join(quoted, " OR "))
	}
	return out
}

var (
	hrTierA  = regexp.MustCompile(`(?i)talent acquisition|recruit|head of (hr|human|people)|(hr|human resources?|people) (director|lead|head)|chief (people|human)|\bchro\b|vp,? (hr|human|people)|people (and|&) culture (director|lead|head|manager)`)
	hrTierB  = regexp.MustCompile(`(?i)(hr|human resources?|people)( business)? (manager|partner|generalist)|hrbp|people (and|&) culture|talent (manager|partner|management)`)
	hrFormer = regexp.MustCompile(`(?i)\b(ex[- ]|former|formerly|previously|retired)\b`)
	hrIntern = regexp.MustCompile(`(?i)\b(intern|internship|trainee|student|graduate)\b`)
)

// HRTier says how likely someone hires, from their title: A recruitment and
// HR leads, B HR managers and partners, C other HR roles.
func HRTier(title string) string {
	switch {
	case hrTierA.MatchString(title):
		return "A"
	case hrTierB.MatchString(title):
		return "B"
	}
	return "C"
}

// HRHidden is why someone is left out of the list by default, or "".
func HRHidden(title string) string {
	switch {
	case hrFormer.MatchString(title):
		return "no longer there, by their title"
	case hrIntern.MatchString(title):
		return "an intern or trainee"
	}
	return ""
}

// HRPerson is someone a scan found.
type HRPerson struct {
	URL      string  `json:"url"`
	Name     string  `json:"name"`
	Title    string  `json:"title"`
	Location string  `json:"location"`
	Joined   *string `json:"joined,omitempty"`
	Left     *string `json:"left,omitempty"`
	Current  *bool   `json:"current,omitempty"`
}

// MergeHRPeople adds a page's people to those found (each profile once, at most hrMaxPeople).
func MergeHRPeople(found, page []HRPerson) []HRPerson {
	seen := map[string]bool{}
	for _, p := range found {
		seen[p.URL] = true
	}
	for _, p := range page {
		if !seen[p.URL] && len(found) < hrMaxPeople {
			seen[p.URL] = true
			found = append(found, p)
		}
	}
	return found
}

// SortHRPeople puts the most likely hirers first, then by name.
func SortHRPeople(people []HRPerson) {
	sort.SliceStable(people, func(i, j int) bool {
		ti, tj := HRTier(people[i].Title), HRTier(people[j].Title)
		if ti != tj {
			return ti < tj
		}
		return strings.ToLower(people[i].Name) < strings.ToLower(people[j].Name)
	})
}

// HRNext is where a scan goes after reading page of query: the next page of
// the same search while LinkedIn has one (up to pages), else the next search;
// done after the last.
func HRNext(query, page, pages, queries int, hasNext bool) (nextQuery, nextPage int, done bool) {
	if hasNext && page < pages {
		return query, page + 1, false
	}
	if query+1 < queries {
		return query + 1, 1, false
	}
	return query, page, true
}
