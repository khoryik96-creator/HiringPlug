package mailcheck

// Address rules: what can be checked, before any network work.

import (
	"net/netip"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

// Syntax states.
const (
	SyntaxOK          = "ok"
	SyntaxInvalid     = "invalid"
	SyntaxUnsupported = "unsupported" // quoted or international local parts, address literals: not proven invalid
)

var (
	localRe = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+/=?^_`{|}~.-]+$")
	labelRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	ctlRe   = regexp.MustCompile(`[\x00-\x1f\x7f]`)
)

// DomainName returns a domain in its lower-case ASCII (IDNA) form, or "" when
// it is not a usable host name (an IP address, a single label, bad labels).
func DomainName(value string) string {
	domain, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return ""
	}
	domain = strings.ToLower(domain)
	labels := strings.Split(domain, ".")
	if len(domain) > 253 || len(labels) < 2 {
		return ""
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return ""
		}
	}
	if _, err := netip.ParseAddr(domain); err == nil {
		return ""
	}
	return domain
}

// AddressParts returns the canonical address (the domain in ASCII lower case,
// the local part exactly as given) and its domain, with the syntax state. It
// never corrects an address.
func AddressParts(value string) (address, domain, syntax string) {
	if len(value) > 320 {
		return "", "", SyntaxInvalid
	}
	value = strings.TrimSpace(value)
	if value == "" || ctlRe.MatchString(value) {
		return value, "", SyntaxInvalid
	}
	at := strings.LastIndex(value, "@")
	if at <= 0 {
		return value, "", SyntaxInvalid
	}
	local, rawDomain := value[:at], value[at+1:]
	if strings.HasPrefix(rawDomain, "[") && strings.HasSuffix(rawDomain, "]") {
		return value, "", SyntaxUnsupported
	}
	domain = DomainName(rawDomain)
	if domain == "" || len(local) > 64 {
		return value, domain, SyntaxInvalid
	}
	address = local + "@" + domain
	if len(address) > 254 {
		return address, domain, SyntaxInvalid
	}
	switch {
	case len(local) >= 2 && strings.HasPrefix(local, `"`) && strings.HasSuffix(local, `"`):
		return address, domain, SyntaxUnsupported
	case strings.ContainsAny(local, " \t"):
		return address, domain, SyntaxInvalid
	case !isASCII(local):
		return address, domain, SyntaxUnsupported
	case !localRe.MatchString(local) || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, ".."):
		return address, domain, SyntaxInvalid
	}
	return address, domain, SyntaxOK
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Special-purpose networks (IANA registries) that are not public internet.
var notGlobal = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
		"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "255.255.255.255/32",
		"::/128", "::1/128", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32",
		"2002::/16", "3fff::/20", "5f00::/16", "fc00::/7", "fe80::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// PublicIP reports whether ip is a public internet address: the only kind a
// check may connect to (IPv4-mapped addresses are judged as IPv4).
func PublicIP(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	for _, p := range notGlobal {
		if p.Contains(addr) {
			return false
		}
	}
	return addr.IsGlobalUnicast()
}
