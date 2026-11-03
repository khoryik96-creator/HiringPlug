// Package robots reads robots.txt, so the app reads only the pages sites
// allow (careers pages for leads, company websites for auto-fill). Rules for
// "HiringPlugBot" by name come first, then those for every robot.
package robots

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

// UserAgent names the app to the sites it reads.
const UserAgent = "HiringPlugBot/1.0 (+respects robots.txt)"

// Site is one site's robots.txt, read once for the pages read from it.
type Site struct{ rules string }

// Read fetches a site's robots.txt (none, or a 4xx answer, allows
// everything).
func Read(ctx context.Context, client *http.Client, pageURL string) (Site, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return Site{}, err
	}
	body, err := safehttp.Get(ctx, client, u.Scheme+"://"+u.Host+"/robots.txt", UserAgent, "text/plain", 512<<10)
	var status safehttp.StatusError
	switch {
	case errors.As(err, &status) && status >= 400 && status < 500:
		return Site{}, nil
	case err != nil:
		return Site{}, err
	}
	return Site{rules: string(body)}, nil
}

// Allows reports whether the site allows reading a page of it.
func (s Site) Allows(pageURL string) bool {
	u, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return Allows(s.rules, path)
}

// Allows reports whether robots.txt lets any robot (or ours, by name:
// HiringPlugBot) fetch path. The most specific (longest) matching rule wins; Allow wins a
// tie.
func Allows(robots, path string) bool {
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
