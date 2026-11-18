package main

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var objectHost = regexp.MustCompile(`^(?:[a-z0-9.-]+\.s3(?:[.-][a-z0-9-]+)?\.amazonaws\.com|s3(?:[.-][a-z0-9-]+)?\.amazonaws\.com|[a-z0-9-]+\.blob\.core\.windows\.net)$`)

// Native downloads can be signed redirects to JobAdder's object storage.
// Never forward the bearer token, cookies or referer to a different host.
func nativeHTTP(base *http.Client, apiHost string) *http.Client {
	h := *base
	h.Timeout = 10 * time.Minute
	h.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return errors.New("too many native-file redirects")
		}
		u := req.URL
		host := strings.ToLower(u.Hostname())
		if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || u.Fragment != "" {
			return errors.New("unsafe native-file redirect")
		}
		if u.Host != apiHost && !apiHostPattern(host) && !objectHost.MatchString(host) {
			return errors.New("unknown native storage host; file requires review")
		}
		if u.Host != apiHost || via[len(via)-1].URL.Host != apiHost {
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			req.Header.Del("Referer")
		}
		req.Header.Del("Accept")
		return nil
	}
	return &h
}
func apiHostPattern(host string) bool { return apiHost.MatchString(host) }
