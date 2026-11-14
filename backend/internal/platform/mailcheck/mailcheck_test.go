package mailcheck

// Ported from CV Studio's email verifier tests: fake DNS, and fake SMTP
// servers on loopback reached through an injected dialer, so no real mail
// server is ever contacted.

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDNS struct {
	mx    map[string][]*net.MX
	ips   map[string][]net.IP // "ip4 host." / "ip6 host."
	fail  map[string]error
	calls atomic.Int32
}

func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (f *fakeDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	f.calls.Add(1)
	if err := f.fail["mx "+name]; err != nil {
		return nil, err
	}
	if mx, ok := f.mx[name]; ok {
		return mx, nil
	}
	return nil, notFound(name)
}

func (f *fakeDNS) LookupIP(_ context.Context, network, host string) ([]net.IP, error) {
	f.calls.Add(1)
	if err := f.fail[network+" "+host]; err != nil {
		return nil, err
	}
	if ips, ok := f.ips[network+" "+host]; ok {
		return ips, nil
	}
	return nil, notFound(host)
}

// server is a scripted SMTP server: reply(cmd) answers each command.
type server struct {
	ln       net.Listener
	mu       sync.Mutex
	commands []string
}

func newServer(t *testing.T, greeting string, reply func(cmd string) string) *server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = c.Write([]byte(greeting + "\r\n"))
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.TrimSpace(line)
					s.mu.Lock()
					s.commands = append(s.commands, cmd)
					s.mu.Unlock()
					out := reply(cmd)
					if out == "" {
						return // hang up
					}
					_, _ = c.Write([]byte(out + "\r\n"))
				}
			}()
		}
	}()
	return s
}

func (s *server) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

// script answers like a normal server; rcpt decides recipients.
func script(rcpt func(addr string) string) func(string) string {
	return func(cmd string) string {
		up := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(up, "EHLO"):
			return "250-mx.test\r\n250 PIPELINING"
		case strings.HasPrefix(up, "MAIL FROM"), up == "RSET":
			return "250 OK"
		case strings.HasPrefix(up, "RCPT TO:<"):
			return rcpt(strings.TrimSuffix(cmd[len("RCPT TO:<"):], ">"))
		}
		return "502 not here"
	}
}

func dialTo(routes map[string]*server) Dialer {
	return func(ctx context.Context, address string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(address)
		s, ok := routes[host]
		if !ok {
			return nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp", s.ln.Addr().String())
	}
}

func oneMX(host string, ips ...string) *fakeDNS {
	f := &fakeDNS{mx: map[string][]*net.MX{"example.com.": {{Host: host + ".", Pref: 10}}}, ips: map[string][]net.IP{}}
	for _, ip := range ips {
		p := net.ParseIP(ip)
		key := "ip4 " + host + "."
		if p.To4() == nil {
			key = "ip6 " + host + "."
		}
		f.ips[key] = append(f.ips[key], p)
	}
	return f
}

const control = "0123456789abcdef0123456789abcdef"

func TestSyntaxIsConservativeAndNeverRewritesTheMailbox(t *testing.T) {
	for in, want := range map[string]string{
		"Ana.Lim+hr@Example.COM": SyntaxOK, "a@b": SyntaxInvalid, "no-at-sign": SyntaxInvalid,
		"a..b@example.com": SyntaxInvalid, ".a@example.com": SyntaxInvalid, `"quoted"@example.com`: SyntaxUnsupported,
		"ünï@example.com": SyntaxUnsupported, "x@[192.0.2.1]": SyntaxUnsupported, "x@192.0.2.1": SyntaxInvalid,
		"a b@example.com": SyntaxInvalid, "a@example.com\r\nRCPT": SyntaxInvalid, strings.Repeat("a", 65) + "@example.com": SyntaxInvalid,
	} {
		if _, _, got := AddressParts(in); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
	if addr, domain, _ := AddressParts("Ana.Lim+hr@Example.COM"); addr != "Ana.Lim+hr@example.com" || domain != "example.com" {
		t.Errorf("got %s %s: the local part keeps its case, the domain is lower case", addr, domain)
	}
	if _, domain, _ := AddressParts("a@bücher.de"); domain != "xn--bcher-kva.de" {
		t.Errorf("international domain: %s", domain)
	}
}

func TestOnlyPublicAddressesAreEverContacted(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"224.0.0.1", "192.0.2.10", "198.51.100.1", "203.0.113.5", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "2001:db8::1", "64:ff9b::7f00:1", "bad"} {
		if PublicIP(ip) {
			t.Errorf("%s counted as public", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2607:f8b0:4004:800::200e", "::ffff:8.8.8.8"} {
		if !PublicIP(ip) {
			t.Errorf("%s not counted as public", ip)
		}
	}
	// A mail server with any private answer is not contacted at all.
	dns := oneMX("mx.example.com", "8.8.8.8", "10.0.0.5")
	dialled := false
	v := &Verifier{DNS: dns, Dial: func(context.Context, string) (net.Conn, error) { dialled = true; return nil, errors.New("no") }}
	r, _ := v.Check(context.Background(), "a@example.com", Full, control)
	if dialled || r.Status != Unknown || r.Outcome != "check_unavailable" {
		t.Fatalf("got %+v (dialled %v)", r, dialled)
	}
}

func TestDomainFailuresAreInvalidOnlyWhenTheDomainSaysSo(t *testing.T) {
	v := &Verifier{DNS: &fakeDNS{mx: map[string][]*net.MX{"example.com.": {{Host: ".", Pref: 0}}}}, Dial: dialTo(nil)}
	if r, _ := v.Check(context.Background(), "a@example.com", Full, control); r.Status != Invalid || r.Outcome != "no_mail" {
		t.Errorf("null MX: %+v", r)
	}
	v.DNS = &fakeDNS{}
	if r, _ := v.Check(context.Background(), "a@example.com", Full, control); r.Status != Invalid || r.Domain != DomainNoServer {
		t.Errorf("no domain: %+v", r)
	}
	v.DNS = &fakeDNS{fail: map[string]error{"mx example.com.": &net.DNSError{Err: "timeout", IsTimeout: true}}}
	if r, _ := v.Check(context.Background(), "a@example.com", Full, control); r.Status != Unknown || !r.Retryable || r.Outcome != "dns_unavailable" {
		t.Errorf("DNS timeout must stay unknown: %+v", r)
	}
	// An advertised MX with no address is unknown, not invalid.
	v.DNS = &fakeDNS{mx: map[string][]*net.MX{"example.com.": {{Host: "mx.example.com.", Pref: 10}}}}
	if r, _ := v.Check(context.Background(), "a@example.com", Full, control); r.Status != Unknown || r.Outcome != "no_server" {
		t.Errorf("MX without address: %+v", r)
	}
}

func TestNoMXFallsBackToTheDomainItself(t *testing.T) {
	srv := newServer(t, "220 hi", script(func(a string) string {
		if strings.HasPrefix(a, ControlPrefix) {
			return "550 5.1.1 no such user"
		}
		return "250 OK"
	}))
	dns := &fakeDNS{ips: map[string][]net.IP{"ip4 example.com.": {net.ParseIP("8.8.4.4")}}}
	v := &Verifier{DNS: dns, Dial: dialTo(map[string]*server{"8.8.4.4": srv})}
	r, _ := v.Check(context.Background(), "a@example.com", Full, control)
	if r.Status != Accepted || r.Domain != DomainImplicit {
		t.Fatalf("got %+v", r)
	}
}

func TestVerdictsComeOnlyFromRecipientSpecificAnswers(t *testing.T) {
	cases := []struct {
		name, target, made, status, outcome string
	}{
		{"accepted, made-up refused", "250 OK", "550 5.1.1 unknown user", Accepted, "recipient_supported"},
		{"both accepted", "250 OK", "250 OK", CatchAll, "catch_all"},
		{"mailbox missing", "550 5.1.1 The email account that you tried to reach does not exist", "", Invalid, "recipient_missing"},
		{"generic 550 is policy", "550 Rejected by policy", "", Unknown, "policy"},
		{"embedded code is not evidence", "550 rejected (see 5.1.1)", "", Unknown, "policy"},
		{"greylisted", "451 4.7.1 try later", "", Unknown, "temporary"},
		{"mailbox full", "552 5.2.2 mailbox full", "", Unknown, "mailbox_full"},
		{"made-up answer inconclusive", "250 OK", "450 4.2.0 later", Unknown, "temporary"},
		{"Access denied on another server", "550 5.4.1 Recipient address rejected: Access denied", "", Unknown, "policy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newServer(t, "220 mx ready", script(func(a string) string {
				if strings.HasPrefix(a, ControlPrefix) {
					return c.made
				}
				return c.target
			}))
			v := &Verifier{DNS: oneMX("mx.example.com", "8.8.8.8"), Dial: dialTo(map[string]*server{"8.8.8.8": srv})}
			r, err := v.Check(context.Background(), "Ana@example.com", Full, control)
			if err != nil || r.Status != c.status || r.Outcome != c.outcome {
				t.Fatalf("got %+v %v, want %s/%s", r, err, c.status, c.outcome)
			}
			for _, cmd := range srv.sent() {
				up := strings.ToUpper(cmd)
				if up == "DATA" || strings.HasPrefix(up, "AUTH") || strings.HasPrefix(up, "VRFY") || strings.HasPrefix(up, "EXPN") {
					t.Fatalf("sent %q", cmd)
				}
			}
			if sent := srv.sent(); !contains(sent, "MAIL FROM:<>") || !contains(sent, "RCPT TO:<Ana@example.com>") {
				t.Fatalf("commands %v", sent)
			}
			if c.target == "250 OK" && !contains(srv.sent(), "RCPT TO:<"+ControlPrefix+control+"@example.com>") {
				t.Fatalf("made-up mailbox not the check's own: %v", srv.sent())
			}
		})
	}
}

func TestMicrosoftDirectoryRejectionCountsOnlyOnMicrosoftServers(t *testing.T) {
	ms := "contoso-com.mail.protection.outlook.com"
	if !missingRecipient(ms, 550, "5.4.1 Recipient address rejected: Access denied. AS(201806281)") {
		t.Error("Microsoft 365's directory rejection not recognised")
	}
	for _, c := range []struct {
		host string
		code int
		text string
	}{
		{"mail.protection.outlook.com.evil.test", 550, "5.4.1 Recipient address rejected: Access denied"},
		{ms, 550, "5.4.1 Access denied"},
		{ms, 554, "5.4.1 Recipient address rejected: Access denied"},
		{ms, 550, "5.4.10 Recipient address rejected: Access denied"},
		{"mx.example.com", 550, "5.1.10 not found"},
	} {
		if missingRecipient(c.host, c.code, c.text) {
			t.Errorf("%+v counted as a missing mailbox", c)
		}
	}
}

func TestNetworkFailuresAreNeverInvalid(t *testing.T) {
	v := &Verifier{DNS: oneMX("mx.example.com", "8.8.8.8"), Dial: dialTo(nil)} // refused
	r, _ := v.Check(context.Background(), "a@example.com", Full, control)
	if r.Status != Unknown || r.Outcome != "connection" || !r.Retryable || r.Mailbox != "unconfirmed" {
		t.Fatalf("refused: %+v", r)
	}
	hang := newServer(t, "220 hi", script(func(string) string { return "" })) // hangs up after EHLO
	v.Dial = dialTo(map[string]*server{"8.8.8.8": hang})
	if r, _ = v.Check(context.Background(), "a@example.com", Full, control); r.Status != Unknown {
		t.Fatalf("hang up: %+v", r)
	}
	// A server that never answers is cut off at the deadline.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			time.Sleep(3 * time.Second)
			_ = c.Close()
		}
	}()
	v.Dial = func(ctx context.Context, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", ln.Addr().String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, _ = v.Check(ctx, "a@example.com", Full, control)
	if r.Status != Unknown || time.Since(start) > 2*time.Second {
		t.Fatalf("silent server: %+v after %s", r, time.Since(start))
	}
}

func TestRepliesAreBounded(t *testing.T) {
	long := newServer(t, "220 "+strings.Repeat("x", 5000), script(func(string) string { return "250 OK" }))
	many := newServer(t, strings.Repeat("220-hi\r\n", 20)+"220 hi", script(func(string) string { return "250 OK" }))
	for name, s := range map[string]*server{"long line": long, "many lines": many} {
		v := &Verifier{DNS: oneMX("mx.example.com", "8.8.8.8"), Dial: dialTo(map[string]*server{"8.8.8.8": s})}
		if r, _ := v.Check(context.Background(), "a@example.com", Full, control); r.Status != Unknown {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestDeepCheckComparesServersAndReportsConflicts(t *testing.T) {
	yes := newServer(t, "220 a", script(func(a string) string {
		if strings.HasPrefix(a, ControlPrefix) {
			return "550 5.1.1 no"
		}
		return "250 OK"
	}))
	no := newServer(t, "220 b", script(func(string) string { return "550 5.1.1 no such user" }))
	dns := &fakeDNS{
		mx:  map[string][]*net.MX{"example.com.": {{Host: "mx1.example.com.", Pref: 10}, {Host: "mx2.example.com.", Pref: 20}}},
		ips: map[string][]net.IP{"ip4 mx1.example.com.": {net.ParseIP("8.8.8.8")}, "ip4 mx2.example.com.": {net.ParseIP("1.1.1.1")}},
	}
	v := &Verifier{DNS: dns, Dial: dialTo(map[string]*server{"8.8.8.8": yes, "1.1.1.1": no})}
	r, _ := v.Check(context.Background(), "a@example.com", Deep, control)
	if r.Status != Unknown || r.Outcome != "conflicting_servers" || r.Attempts != 2 || r.Mailbox != "conflicting" {
		t.Fatalf("deep: %+v", r)
	}
	// Full mode stops at the first clear answer.
	if r, _ = v.Check(context.Background(), "a@example.com", Full, control); r.Status != Accepted || r.Attempts != 1 {
		t.Fatalf("full: %+v", r)
	}
}

func TestABlockedIPv4RouteTriesTheIPv6Address(t *testing.T) {
	ok := newServer(t, "220 v6", script(func(a string) string {
		if strings.HasPrefix(a, ControlPrefix) {
			return "550 5.1.1 no"
		}
		return "250 OK"
	}))
	dns := oneMX("mx.example.com", "8.8.8.8", "8.8.4.4", "2607:f8b0::1")
	v := &Verifier{DNS: dns, Dial: dialTo(map[string]*server{"2607:f8b0::1": ok})}
	r, _ := v.Check(context.Background(), "a@example.com", Full, control)
	if r.Status != Accepted || r.Attempts != 2 {
		t.Fatalf("got %+v", r)
	}
}

func TestChecksAreBoundedToTwoAtATimeAndOnePerDomainWithoutWaiting(t *testing.T) {
	var running, most atomic.Int32
	slow := func(ctx context.Context, _ string) (net.Conn, error) {
		n := running.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		running.Add(-1)
		return nil, &net.OpError{Op: "dial", Err: errors.New("refused")}
	}
	dns := &fakeDNS{mx: map[string][]*net.MX{}, ips: map[string][]net.IP{}}
	for _, d := range []string{"a.com", "b.com", "c.com", "d.com"} {
		dns.mx[d+"."] = []*net.MX{{Host: "mx." + d + ".", Pref: 1}}
		dns.ips["ip4 mx."+d+"."] = []net.IP{net.ParseIP("8.8.8.8")}
	}
	v := &Verifier{DNS: dns, Dial: slow}
	var wg sync.WaitGroup
	var busy, done atomic.Int32
	for _, d := range []string{"a.com", "b.com", "c.com", "d.com", "a.com", "a.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := v.Check(context.Background(), "x@"+d, Full, control)
			switch {
			case errors.Is(err, ErrBusy):
				busy.Add(1)
			case err == nil:
				done.Add(1)
			}
		}()
	}
	wg.Wait()
	if most.Load() > 2 || busy.Load() < 4 || busy.Load()+done.Load() != 6 {
		t.Fatalf("%d at once, %d busy, %d done", most.Load(), busy.Load(), done.Load())
	}
	if len(v.domains) != 0 || len(v.slots) != 0 {
		t.Fatal("slots or domains not released")
	}
	// The same domain twice at once: the second is told to come back.
	unlock, ok := v.lockDomain("a.com")
	if !ok {
		t.Fatal("domain not free")
	}
	if _, err := v.Check(context.Background(), "y@a.com", Full, control); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	unlock()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
