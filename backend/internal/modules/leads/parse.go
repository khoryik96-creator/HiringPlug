package leads

// Reading openings from what sources return, as pure functions: the job
// board feeds' JSON, and schema.org JobPosting markup in careers pages.

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WebLink is a link read from someone else's page or job board as it may be
// stored and shown: an http(s) address (one relative to base is completed),
// or nil. Anything else (javascript:, data:, a bare word) is dropped: these
// become links people click.
func WebLink(raw *string, base string) *string {
	if raw == nil {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil
	}
	if b, err := url.Parse(base); err == nil && base != "" {
		u = b.ResolveReference(u)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil
	}
	out := u.String()
	return &out
}

func str(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func when(s string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700", time.DateOnly} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return &t
		}
	}
	return nil
}

// ParseGreenhouse reads boards-api.greenhouse.io/v1/boards/<board>/jobs.
func ParseGreenhouse(body []byte) ([]Posting, error) {
	var feed struct {
		Jobs []struct {
			ID             int64  `json:"id"`
			Title          string `json:"title"`
			AbsoluteURL    string `json:"absolute_url"`
			FirstPublished string `json:"first_published"`
			UpdatedAt      string `json:"updated_at"`
			Location       struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("not a Greenhouse job board: %w", err)
	}
	out := make([]Posting, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		posted := when(j.FirstPublished)
		if posted == nil {
			posted = when(j.UpdatedAt)
		}
		out = append(out, Posting{ExternalID: strconv.FormatInt(j.ID, 10), Title: strings.TrimSpace(j.Title), Location: str(j.Location.Name), URL: str(j.AbsoluteURL), PostedAt: posted})
	}
	return out, nil
}

// ParseLever reads api.lever.co/v0/postings/<site>?mode=json.
func ParseLever(body []byte) ([]Posting, error) {
	var feed []struct {
		ID         string `json:"id"`
		Text       string `json:"text"`
		HostedURL  string `json:"hostedUrl"`
		CreatedAt  int64  `json:"createdAt"` // milliseconds
		Categories struct {
			Location string `json:"location"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("not a Lever job board: %w", err)
	}
	out := make([]Posting, 0, len(feed))
	for _, j := range feed {
		p := Posting{ExternalID: j.ID, Title: strings.TrimSpace(j.Text), Location: str(j.Categories.Location), URL: str(j.HostedURL)}
		if j.CreatedAt > 0 {
			t := time.UnixMilli(j.CreatedAt).UTC()
			p.PostedAt = &t
		}
		out = append(out, p)
	}
	return out, nil
}

// ParseAshby reads api.ashbyhq.com/posting-api/job-board/<board>.
func ParseAshby(body []byte) ([]Posting, error) {
	var feed struct {
		Jobs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Location    string `json:"location"`
			JobURL      string `json:"jobUrl"`
			PublishedAt string `json:"publishedAt"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("not an Ashby job board: %w", err)
	}
	out := make([]Posting, 0, len(feed.Jobs))
	for _, j := range feed.Jobs {
		out = append(out, Posting{ExternalID: j.ID, Title: strings.TrimSpace(j.Title), Location: str(j.Location), URL: str(j.JobURL), PostedAt: when(j.PublishedAt)})
	}
	return out, nil
}

// ParseSmartRecruiters reads api.smartrecruiters.com/v1/companies/<company>/postings.
func ParseSmartRecruiters(body []byte, company string) ([]Posting, error) {
	var feed struct {
		Content []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ReleasedDate string `json:"releasedDate"`
			Location     struct {
				City    string `json:"city"`
				Country string `json:"country"`
			} `json:"location"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("not a SmartRecruiters job board: %w", err)
	}
	out := make([]Posting, 0, len(feed.Content))
	for _, j := range feed.Content {
		loc := strings.Trim(strings.TrimSpace(j.Location.City)+", "+strings.ToUpper(strings.TrimSpace(j.Location.Country)), ", ")
		out = append(out, Posting{
			ExternalID: j.ID, Title: strings.TrimSpace(j.Name), Location: str(loc),
			URL: str("https://jobs.smartrecruiters.com/" + company + "/" + j.ID), PostedAt: when(j.ReleasedDate),
		})
	}
	return out, nil
}

var ldJSON = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// ParseJobPostings reads the schema.org JobPosting markup (JSON-LD, as for
// Google for Jobs) in a careers page; pageURL stands in for a posting
// without its own address.
func ParseJobPostings(page []byte, pageURL string) []Posting {
	var out []Posting
	for _, m := range ldJSON.FindAllSubmatch(page, -1) {
		var data any
		if json.Unmarshal([]byte(html.UnescapeString(string(m[1]))), &data) != nil {
			continue
		}
		walk(data, func(o map[string]any) {
			if p, ok := jobPosting(o, pageURL); ok {
				out = append(out, p)
			}
		})
	}
	return out
}

// walk calls fn for every object in a JSON-LD document (arrays and @graph).
func walk(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			walk(x, fn)
		}
	case map[string]any:
		fn(t)
		if g, ok := t["@graph"]; ok {
			walk(g, fn)
		}
	}
}

func text(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(html.UnescapeString(t))
	case map[string]any:
		for _, k := range []string{"name", "value", "@value"} {
			if s := text(t[k]); s != "" {
				return s
			}
		}
	case []any:
		if len(t) > 0 {
			return text(t[0])
		}
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return ""
}

func isType(o map[string]any, want string) bool {
	switch t := o["@type"].(type) {
	case string:
		return t == want
	case []any:
		for _, x := range t {
			if x == want {
				return true
			}
		}
	}
	return false
}

func jobPosting(o map[string]any, pageURL string) (Posting, bool) {
	if !isType(o, "JobPosting") {
		return Posting{}, false
	}
	p := Posting{Title: text(o["title"]), Company: text(o["hiringOrganization"]), PostedAt: when(text(o["datePosted"]))}
	if p.Title == "" {
		return p, false
	}
	p.URL = WebLink(str(text(o["url"])), pageURL)
	if p.URL == nil {
		p.URL = str(pageURL)
	}
	p.Location = str(location(o["jobLocation"]))
	switch {
	case text(o["identifier"]) != "":
		p.ExternalID = text(o["identifier"])
	case text(o["url"]) != "":
		p.ExternalID = text(o["url"])
	default:
		p.ExternalID = p.Title + "|" + location(o["jobLocation"])
	}
	return p, true
}

// location writes a JobPosting's jobLocation as "Kuala Lumpur, MY".
func location(v any) string {
	if arr, ok := v.([]any); ok && len(arr) > 0 {
		v = arr[0]
	}
	place, ok := v.(map[string]any)
	if !ok {
		return text(v)
	}
	addr, ok := place["address"].(map[string]any)
	if !ok {
		return text(place["address"])
	}
	var parts []string
	for _, k := range []string{"addressLocality", "addressRegion", "addressCountry"} {
		if s := text(addr[k]); s != "" && !containsFold(parts, s) {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
