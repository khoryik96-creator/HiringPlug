package mailcheck

// One SMTP session with a mail server, asking only whether it takes mail for
// the address and for a made-up address beside it. Never DATA, AUTH, VRFY
// or EXPN: nothing is sent and no one is emailed.

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Dialer opens the connection to a mail server (a fake in tests).
type Dialer func(ctx context.Context, address string) (net.Conn, error)

const (
	commandCap   = 4 * time.Second
	maxReplyLine = 4096
	maxReplyRows = 16
	ehloName     = "[127.0.0.1]"
)

// observation is what one session established.
type observation struct {
	Status    string // accepted, catch_all, invalid, unknown
	Reason    string
	Outcome   string
	Retryable bool
	Target    string // accepted, rejected, unconfirmed
}

var (
	missing5_1_1 = regexp.MustCompile(`^\s*5\.1\.1\b`)
	dbebDenied   = regexp.MustCompile(`(?i)^\s*5\.4\.1 Recipient address rejected: Access denied\.?(?:\s|$)`)
	mailboxFull  = regexp.MustCompile(`^\s*[45]\.2\.2\b`)
)

// Provider names a mail service from its server's name (not a verification feed).
func Provider(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	switch {
	case strings.HasSuffix(host, ".mail.protection.outlook.com"):
		return "Microsoft 365"
	case host == "smtp.google.com" || host == "aspmx.l.google.com" || strings.HasSuffix(host, ".aspmx.l.google.com") || strings.HasSuffix(host, ".aspmx.l.googlemail.com"):
		return "Google Workspace"
	}
	return "Other mail server"
}

// missingRecipient reports whether a reply says the mailbox does not exist:
// 550/551/553 with 5.1.1, or Microsoft 365's directory-based rejection on an
// actual Microsoft 365 server. A generic 550 or "access denied" is not.
func missingRecipient(host string, code int, text string) bool {
	if code != 550 && code != 551 && code != 553 {
		return false
	}
	if missing5_1_1.MatchString(text) {
		return true
	}
	return code == 550 && Provider(host) == "Microsoft 365" && dbebDenied.MatchString(text)
}

type session struct {
	conn     net.Conn
	r        *bufio.Reader
	deadline time.Time
	host     string
}

func (s *session) budget() error {
	remaining := time.Until(s.deadline)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	return s.conn.SetDeadline(time.Now().Add(min(commandCap, remaining)))
}

// reply reads one reply: at most 16 lines of 4 KB, within the deadline.
func (s *session) reply() (int, string, error) {
	var lines []string
	for range maxReplyRows {
		if err := s.budget(); err != nil {
			return 0, "", err
		}
		var line []byte
		for {
			chunk, err := s.r.ReadSlice('\n')
			line = append(line, chunk...)
			if len(line) > maxReplyLine {
				return 0, "", errors.New("reply too long")
			}
			if err == nil {
				break
			}
			if !errors.Is(err, bufio.ErrBufferFull) {
				return 0, "", err
			}
		}
		if len(line) < 4 {
			return 0, "", errors.New("unusable reply")
		}
		code, err := strconv.Atoi(string(line[:3]))
		if err != nil {
			return 0, "", errors.New("unusable reply")
		}
		lines = append(lines, strings.TrimSpace(string(line[4:])))
		if line[3] != '-' {
			return code, strings.Join(lines, "\n"), nil
		}
	}
	return 0, "", errors.New("too many reply lines")
}

func (s *session) cmd(line string) (int, string, error) {
	if err := s.budget(); err != nil {
		return 0, "", err
	}
	if _, err := s.conn.Write([]byte(line + "\r\n")); err != nil {
		return 0, "", err
	}
	return s.reply()
}

// probe runs one session with host at ip (a checked public address) for
// address, with the check's made-up control mailbox controlLocal.
func probe(ctx context.Context, dial Dialer, host, ip, address, controlLocal string, deadline time.Time) observation {
	done := func(status, reason, outcome string, retryable bool, target string) observation {
		return observation{Status: status, Reason: reason, Outcome: outcome, Retryable: retryable, Target: target}
	}
	if !PublicIP(ip) || DomainName(host) == "" {
		return done("unknown", "The mail server's network address is restricted. It was not contacted.", "check_unavailable", false, "unconfirmed")
	}
	tempOr := func(code int, other string) (string, bool) {
		if code >= 400 && code < 500 {
			return "temporary", true
		}
		return other, false
	}
	phase := "connection"
	target := "unconfirmed"
	connCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	dialCtx, cancelDial := context.WithTimeout(connCtx, min(commandCap, time.Until(deadline)))
	conn, err := dial(dialCtx, net.JoinHostPort(ip, "25"))
	cancelDial()
	if err != nil {
		return networkFailure(err, phase, target)
	}
	defer func() { _ = conn.Close() }() // close, not QUIT: never wait on a silent server
	s := &session{conn: conn, r: bufio.NewReaderSize(conn, maxReplyLine), deadline: deadline, host: host}
	declined := func(code int, text string, control bool) observation {
		what := "this mailbox."
		if control {
			what = "its catch-all policy."
		}
		switch {
		case mailboxFull.MatchString(text):
			return done("unknown", "The mailbox is full. This is not evidence of a missing address.", "mailbox_full", false, target)
		case code >= 400 && code < 500:
			check := "mailbox check."
			if control {
				check = "catch-all check."
			}
			return done("unknown", "The server temporarily deferred the "+check+" A later recheck may help.", "temporary", true, target)
		}
		return done("unknown", "The server did not confirm "+what+" It may restrict verification.", "policy", false, target)
	}

	code, _, err := s.reply()
	if err != nil {
		return networkFailure(err, phase, target)
	}
	if code != 220 {
		outcome, retry := tempOr(code, "policy")
		return done("unknown", "The mail server did not accept the connection.", outcome, retry, target)
	}
	phase = "session"
	code, text, err := s.cmd("EHLO " + ehloName)
	if err == nil && code != 250 {
		code, text, err = s.cmd("HELO " + ehloName)
	}
	if err != nil {
		return networkFailure(err, phase, target)
	}
	if code != 250 {
		outcome, retry := tempOr(code, "policy")
		return done("unknown", "The mail server declined the verification session.", outcome, retry, target)
	}
	if hasExtension(text, "STARTTLS") {
		if code, _, err = s.cmd("STARTTLS"); err != nil {
			return networkFailure(err, phase, target)
		}
		if code != 220 {
			return done("unknown", "The mail server declined the secure verification session.", "policy", false, target)
		}
		// The connection stays on the checked address; the certificate must be the server's name.
		tc := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := s.budget(); err != nil {
			return networkFailure(err, phase, target)
		}
		if err := tc.HandshakeContext(connCtx); err != nil {
			var verify *tls.CertificateVerificationError
			var unknownAuth x509.UnknownAuthorityError
			var hostname x509.HostnameError
			if errors.As(err, &verify) || errors.As(err, &unknownAuth) || errors.As(err, &hostname) {
				return done("unknown", "The mail server's secure connection could not be verified. The mailbox remains unconfirmed.", "tls", false, target)
			}
			return networkFailure(err, phase, target)
		}
		s.conn, s.r = tc, bufio.NewReaderSize(tc, maxReplyLine)
		if code, _, err = s.cmd("EHLO " + ehloName); err != nil {
			return networkFailure(err, phase, target)
		}
		if code != 250 {
			return done("unknown", "The mail server declined the secure verification session.", "policy", false, target)
		}
	}
	senderOK := func() (observation, bool) {
		code, _, err := s.cmd("MAIL FROM:<>")
		if err != nil {
			return networkFailure(err, phase, target), false
		}
		if code != 250 {
			outcome, retry := tempOr(code, "sender_policy")
			reason := "The server declined a verification-only sender. No mailbox conclusion is possible."
			if target == "accepted" {
				reason = "The mailbox was accepted, but the catch-all check could not be completed."
			}
			return done("unknown", reason, outcome, retry, target), false
		}
		return observation{}, true
	}
	if o, ok := senderOK(); !ok {
		return o
	}
	phase = "mailbox"
	code, text, err = s.cmd("RCPT TO:<" + address + ">")
	if err != nil {
		return networkFailure(err, phase, target)
	}
	if missingRecipient(host, code, text) {
		return done("invalid", "The mail server explicitly reported that this mailbox does not exist.", "recipient_missing", false, "rejected")
	}
	if code != 250 && code != 251 {
		return declined(code, text, false)
	}
	target = "accepted"
	if code, _, err = s.cmd("RSET"); err != nil || code != 250 {
		if err != nil {
			return networkFailure(err, phase, target)
		}
		return done("unknown", "The mailbox was accepted, but the catch-all check could not be completed.", "policy", false, target)
	}
	if o, ok := senderOK(); !ok {
		return o
	}
	control := controlLocal + "@" + address[strings.LastIndex(address, "@")+1:]
	code, text, err = s.cmd("RCPT TO:<" + control + ">")
	if err != nil {
		return networkFailure(err, phase, target)
	}
	switch {
	case code == 250 || code == 251:
		return done("catch_all", "The server accepts made-up addresses too. A real mailbox can exist on this domain, but this check cannot confirm it.", "catch_all", false, target)
	case missingRecipient(host, code, text):
		return done("accepted", "The server accepted this mailbox and rejected a made-up one. Delivery is still not guaranteed.", "recipient_supported", false, target)
	}
	return declined(code, text, true)
}

func networkFailure(err error, phase, target string) observation {
	var ne net.Error
	timeout := errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout())
	o := observation{Status: "unknown", Retryable: true, Target: target}
	switch {
	case timeout && phase == "connection":
		o.Outcome, o.Reason = "connection", "The mail server connection timed out on port 25. This server's network or the mail server may block verification. This does not mean the email is invalid."
	case timeout && phase == "mailbox":
		o.Outcome, o.Reason = "timeout", "The mailbox response timed out. The mailbox remains unconfirmed; it has not been marked invalid."
	case timeout:
		o.Outcome, o.Reason = "timeout", "The mail server's verification session timed out before a mailbox answer. The mailbox remains unconfirmed."
	case errors.Is(err, syscall.ECONNREFUSED):
		o.Outcome, o.Reason = "connection", "The mail server connection was refused. The mailbox remains unconfirmed; it has not been marked invalid."
	default:
		o.Outcome, o.Reason = "connection", "The mail server connection was interrupted or unavailable. The mailbox remains unconfirmed; it has not been marked invalid."
	}
	return o
}

func hasExtension(ehlo, name string) bool {
	for _, line := range strings.Split(ehlo, "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.EqualFold(f[0], name) {
			return true
		}
	}
	return false
}

// PublicDialer connects only to public addresses on port 25, rechecking the
// address actually dialled (no name is resolved here).
func PublicDialer() Dialer {
	d := &net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "25" || !PublicIP(host) {
			return fmt.Errorf("%s is not a public mail server address", address)
		}
		return nil
	}}
	return func(ctx context.Context, address string) (net.Conn, error) {
		return d.DialContext(ctx, "tcp", address)
	}
}
