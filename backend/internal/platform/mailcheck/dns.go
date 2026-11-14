package mailcheck

// The domain's mail servers and their addresses, from DNS.

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"time"
)

// Resolver is the DNS the checks use (net.Resolver; a fake in tests).
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
}

// unavailable is a check that could not establish an answer: never a
// negative mailbox verdict.
type unavailable struct {
	reason    string
	transient bool // a DNS hiccup: worth trying again later
}

func (u *unavailable) Error() string { return u.reason }

var (
	errDNSTimeout = &unavailable{"The DNS check timed out. Try this address again later.", true}
	errDNSFailed  = &unavailable{"The domain's DNS could not be read. This does not mean the address is invalid.", true}
)

// Domain states.
const (
	DomainMX         = "mx"
	DomainImplicit   = "implicit" // no MX: the domain itself takes mail (RFC 5321)
	DomainNone       = "no_domain"
	DomainNoMail     = "no_mail" // a null MX: the domain says it takes no mail
	DomainNoServer   = "no_server"
	DomainNotChecked = "not_checked"
)

const dnsQueryCap = 3 * time.Second

func query[T any](ctx context.Context, deadline time.Time, fn func(context.Context) (T, error)) (T, bool, error) {
	var zero T
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return zero, false, errDNSTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, min(dnsQueryCap, remaining))
	defer cancel()
	v, err := fn(ctx)
	if err == nil {
		return v, true, nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return zero, false, nil // no such name, or no records of this type
	}
	if ctx.Err() != nil || (errors.As(err, &dnsErr) && dnsErr.IsTimeout) {
		return zero, false, errDNSTimeout
	}
	return zero, false, errDNSFailed
}

// hosts returns the domain's state and up to two mail server names, best first.
func hosts(ctx context.Context, r Resolver, domain string, deadline time.Time) (string, []string, error) {
	mx, found, err := query(ctx, deadline, func(ctx context.Context) ([]*net.MX, error) { return r.LookupMX(ctx, domain+".") })
	if err != nil {
		return "", nil, err
	}
	if found && len(mx) > 0 {
		if len(mx) == 1 && (mx[0].Pref == 0 && strings.Trim(mx[0].Host, ".") == "") {
			return DomainNoMail, nil, nil
		}
		sort.SliceStable(mx, func(i, j int) bool { return mx[i].Pref < mx[j].Pref })
		var names []string
		seen := map[string]bool{}
		for _, m := range mx {
			host := DomainName(strings.TrimSuffix(m.Host, "."))
			if host == "" {
				return "", nil, &unavailable{"The domain publishes an unusable mail-server record.", false}
			}
			if !seen[host] {
				seen[host] = true
				names = append(names, host)
			}
		}
		return DomainMX, names[:min(2, len(names))], nil
	}
	// No MX: the domain itself, if it exists, is the mail server.
	return DomainImplicit, []string{domain}, nil
}

// addresses returns up to two public addresses of a mail server, one of each
// family when both are advertised. Every answer is checked first: a server
// with any private or special address is not contacted at all.
func addresses(ctx context.Context, r Resolver, host string, deadline time.Time) ([]string, error) {
	var all []string
	var failure error
	for _, network := range []string{"ip4", "ip6"} {
		ips, _, err := query(ctx, deadline, func(ctx context.Context) ([]net.IP, error) { return r.LookupIP(ctx, network, host+".") })
		if err != nil {
			if failure == nil {
				failure = err
			}
			continue // one failed family must not hide the other's answer
		}
		for _, ip := range ips {
			all = append(all, ip.String())
		}
	}
	for _, ip := range all {
		if !PublicIP(ip) {
			return nil, &unavailable{"The mail server resolves to a restricted network address. It was not contacted.", false}
		}
	}
	if len(all) == 0 && failure != nil {
		return nil, failure // inconclusive is not an empty answer
	}
	var v4, v6 []string
	seen := map[string]bool{}
	for _, ip := range all {
		if seen[ip] {
			continue
		}
		seen[ip] = true
		if strings.Contains(ip, ":") {
			v6 = append(v6, ip)
		} else {
			v4 = append(v4, ip)
		}
	}
	if len(v4) > 0 && len(v6) > 0 {
		return []string{v4[0], v6[0]}, nil
	}
	both := append(v4, v6...)
	return both[:min(2, len(both))], nil
}
