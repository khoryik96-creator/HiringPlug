package contacts

// The rules of LinkedIn checks, as pure functions: which addresses can be
// checked, and which answers make sense (from CV Studio's companion, which
// applies the same rules to what it reads).

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	profilePathRe = regexp.MustCompile(`^/in/([A-Za-z0-9_%~-]+)/?$`)
	companyPathRe = regexp.MustCompile(`^/company/([A-Za-z0-9_-]+)(?:/.*)?$`)
	yearMonthRe   = regexp.MustCompile(`^(?:19|20)\d{2}(?:-(?:0[1-9]|1[0-2]))?$`)
	// LinkedIn's number for a company.
	linkedinCompanyIDRe = regexp.MustCompile(`^[0-9]{1,24}$`)
)

func linkedinURL(raw string) (*url.URL, bool) {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Port() != "" {
		return nil, false
	}
	host := strings.ToLower(u.Hostname())
	if host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com") {
		return nil, false
	}
	return u, true
}

// ProfileURL returns a contact's LinkedIn profile as the companion opens it,
// https://www.linkedin.com/in/<name>, or "" when it is not a profile address
// (an lnkd.in short link, a public /pub/ address).
func ProfileURL(raw string) string {
	u, ok := linkedinURL(raw)
	if !ok {
		return ""
	}
	m := profilePathRe.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return ""
	}
	return "https://www.linkedin.com/in/" + m[1]
}

// CompanyPageURL returns a company's LinkedIn page as the companion compares
// it, https://www.linkedin.com/company/<name>/, or "".
func CompanyPageURL(raw string) string {
	u, ok := linkedinURL(raw)
	if !ok {
		return ""
	}
	m := companyPathRe.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return ""
	}
	return "https://www.linkedin.com/company/" + m[1] + "/"
}

// ValidEmployment reports whether what a profile shows of someone's time at
// a company makes sense: current (joined, no end), former (joined and left,
// in order) or unknown (neither). Dates are a year or a year and month, as
// precise as the profile shows: a year is never turned into a month.
func ValidEmployment(joined, left string, current *bool) bool {
	for _, d := range []string{joined, left} {
		if d != "" && !yearMonthRe.MatchString(d) {
			return false
		}
	}
	switch {
	case current == nil:
		return joined == "" && left == ""
	case *current:
		return joined != "" && left == ""
	}
	if joined == "" || left == "" {
		return false
	}
	// Earliest possible start against latest possible end.
	start, end := joined, left
	if len(start) == 4 {
		start += "-01"
	}
	if len(end) == 4 {
		end += "-12"
	}
	return start <= end
}
