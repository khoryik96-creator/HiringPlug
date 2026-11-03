// Package safehttp makes HTTP requests to addresses users typed (webhooks,
// careers pages): never to the server's own or private networks (checked on
// the address actually connected to, so a name that resolves to one is
// refused too), never through a proxy (the check must see the real address),
// never following redirects elsewhere unchecked, with time and size limits.
package safehttp

import (
	"errors"
	"fmt"
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
