package leads

// The lead rules, as plain functions on plain values: no database, no HTTP.

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Source kinds.
const (
	Greenhouse      = "greenhouse"
	Lever           = "lever"
	Ashby           = "ashby"
	SmartRecruiters = "smartrecruiters"
	CareersPage     = "careers_page"
)

// CheckEvery is how often sources are checked.
const CheckEvery = 24 * time.Hour

var boardName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// NormalizeSource checks a source. A job board's identifier is its board
// name; a full board address is accepted and its name taken from it (e.g.
// https://boards.greenhouse.io/acme -> acme). A careers page is an http(s)
// address. Job boards need the company's name; a careers page without one is
// named after its site until its markup names the company.
func NormalizeSource(kind, identifier, name string) (string, string, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	identifier = strings.TrimSpace(identifier)
	name = strings.Join(strings.Fields(name), " ")
	if kind == CareersPage {
		u, err := url.Parse(identifier)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			errs.Add("identifier", "must be the careers page address, e.g. https://example.com/careers")
			return identifier, name, errs
		}
		if name == "" {
			name = strings.TrimPrefix(u.Hostname(), "www.")
		}
		return u.String(), name, errs
	}
	if u, err := url.Parse(identifier); err == nil && u.Host != "" {
		identifier = boardFromPath(strings.Split(strings.Trim(u.Path, "/"), "/"))
	}
	if !boardName.MatchString(identifier) {
		errs.Add("identifier", "must be the board name, e.g. acme (from boards.greenhouse.io/acme)")
	}
	if name == "" {
		errs.Add("name", "enter the company's name")
	}
	return identifier, name, errs
}

// boardFromPath picks the board name from a board address: the segment after
// boards/companies/job-board in an API address
// (boards-api.greenhouse.io/v1/boards/acme/jobs), else the first segment
// (boards.greenhouse.io/acme, jobs.lever.co/acme, jobs.ashbyhq.com/acme,
// careers.smartrecruiters.com/Acme).
func boardFromPath(parts []string) string {
	for i, p := range parts {
		if (p == "boards" || p == "companies" || p == "job-board" || p == "postings") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return parts[0]
}

// Posting is one opening found at a source.
type Posting struct {
	ExternalID string
	Title      string
	Company    string // empty when the source does not name it
	Location   *string
	URL        *string
	PostedAt   *time.Time
}

// Existing is a lead already known for a source.
type Existing struct {
	ID         string
	ExternalID string
	Closed     bool
}

// Sync is what a check changes: openings to add, leads seen again (with their
// latest details), and leads no longer listed.
type Sync struct {
	New    []Posting
	Seen   map[string]Posting // lead id -> posting
	Closed []string           // lead ids
}

// Diff compares a source's known leads with what it lists now.
func Diff(existing []Existing, postings []Posting) Sync {
	byExternal := map[string]Existing{}
	for _, e := range existing {
		byExternal[e.ExternalID] = e
	}
	out := Sync{Seen: map[string]Posting{}}
	listed := map[string]bool{}
	for _, p := range postings {
		if p.ExternalID == "" || p.Title == "" || listed[p.ExternalID] {
			continue
		}
		listed[p.ExternalID] = true
		if e, ok := byExternal[p.ExternalID]; ok {
			out.Seen[e.ID] = p
		} else {
			out.New = append(out.New, p)
		}
	}
	for _, e := range existing {
		if !listed[e.ExternalID] && !e.Closed {
			out.Closed = append(out.Closed, e.ID)
		}
	}
	return out
}

// RobotsAllows reports whether robots.txt lets any robot (or ours, by name)
// fetch path. The most specific (longest) matching rule wins; Allow wins a
// tie.
func RobotsAllows(robots, path string) bool {
	type rule struct {
		allow  bool
		prefix string
	}
	var mine, any []rule
	var groups []string // user agents of the current group
	inRules := false
	for _, line := range strings.Split(robots, "\n") {
		line, _, _ = strings.Cut(line, "#")
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if inRules {
				groups, inRules = nil, false
			}
			groups = append(groups, strings.ToLower(value))
		case "allow", "disallow":
			inRules = true
			if value == "" {
				continue // "Disallow:" with nothing allows everything
			}
			r := rule{allow: key == "allow", prefix: value}
			for _, g := range groups {
				switch {
				case strings.Contains(g, "hiringplug"):
					mine = append(mine, r)
				case g == "*":
					any = append(any, r)
				}
			}
		}
	}
	rules := any
	if len(mine) > 0 {
		rules = mine
	}
	best, allowed := -1, true
	for _, r := range rules {
		if strings.HasPrefix(path, strings.TrimSuffix(r.prefix, "*")) && (len(r.prefix) > best || (len(r.prefix) == best && r.allow)) {
			best, allowed = len(r.prefix), r.allow
		}
	}
	return allowed
}
