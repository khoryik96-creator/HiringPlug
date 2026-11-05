// Package linkedin tidies LinkedIn profile addresses, for contacts and
// candidates alike.
package linkedin

import (
	"net/url"
	"strings"
)

// Normalize turns "my.linkedin.com/in/siti/" into
// "https://www.linkedin.com/in/siti"; lnkd.in short links are kept as they
// are (as Kano reads them). ok is false for anything else.
func Normalize(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return raw, false
	}
	host := strings.ToLower(u.Hostname())
	path := strings.Trim(u.Path, "/")
	switch {
	case path == "":
		return raw, false
	case host == "lnkd.in":
		return "https://lnkd.in/" + path, true
	case host != "linkedin.com" && !strings.HasSuffix(host, ".linkedin.com"):
		return raw, false
	}
	u.Scheme, u.Host, u.RawQuery, u.Fragment = "https", "www.linkedin.com", "", ""
	return strings.TrimSuffix(u.String(), "/"), true
}
