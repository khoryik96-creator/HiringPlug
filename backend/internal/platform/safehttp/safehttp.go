// Package safehttp makes HTTP requests to addresses users typed (webhooks,
// careers pages): never to the server's own or private networks (checked on
// the address actually connected to, so a name that resolves to one is
// refused too), never through a proxy (the check must see the real address),
// never following redirects elsewhere unchecked, with time and size limits.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// Blocked reports whether an address is the server's own or a private
// network one (unless allowed: local testing only).
func Blocked(ip net.IP, allowPrivate bool) bool {
	if allowPrivate {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsUnspecified() || addr.IsMulticast() || addr.IsInterfaceLocalMulticast() ||
		netip.MustParsePrefix("100.64.0.0/10").Contains(addr) // carrier-grade NAT
}

// ErrRedirect is returned when a response redirects and redirects are off.
var ErrRedirect = errors.New("the address redirected (redirects are not followed)")

// Options configure a client.
type Options struct {
	AllowPrivate   bool          // local testing only
	Timeout        time.Duration // the whole request
	FollowRedirect bool          // follow up to 5 redirects (each one checked)
}

// Client returns an HTTP client with the checks above.
func Client(o Options) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if Blocked(net.ParseIP(host), o.AllowPrivate) {
			return fmt.Errorf("%s is a private or local network address", host)
		}
		return nil
	}}
	return &http.Client{
		Timeout:   o.Timeout,
		Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if !o.FollowRedirect {
				return ErrRedirect
			}
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil // the next connection is checked by the dialer again
		},
	}
}

// StatusError is an answer other than 2xx.
type StatusError int

func (e StatusError) Error() string {
	if e == http.StatusNotFound {
		return "not found (HTTP 404): check the address"
	}
	return fmt.Sprintf("the site answered HTTP %d", int(e))
}

// Get fetches an address (at most max bytes), as userAgent; an answer other
// than 2xx is a StatusError.
func Get(ctx context.Context, client *http.Client, address, userAgent, accept string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode/100 != 2 {
		return nil, StatusError(res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxBytes))
}
