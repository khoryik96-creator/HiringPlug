// Package mail sends email through an SMTP server (the company's own, or a
// service such as Amazon SES, SendGrid or Mailgun via their SMTP endpoints).
//
// Messages are plain text, UTF-8, quoted-printable. Addresses and the subject
// are checked for line breaks, so nothing typed into an email can add headers.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// Message is one email.
type Message struct {
	FromName string   // shown as the sender, e.g. "Aina (Contoh Recruitment)"
	ReplyTo  string   // where replies go: the person who sent it
	To, Cc   []string // addresses
	Subject  string
	Body     string // plain text
}

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrPermanent marks failures that retrying cannot fix (a refused address, a
// misconfigured server); other errors are worth retrying.
var ErrPermanent = errors.New("permanent")

// Config is how to reach the mail server.
type Config struct {
	Addr     string // host:port, e.g. smtp.example.com:587
	Username string
	Password string
	From     string // the envelope sender and From address, e.g. no-reply@example.com
	TLS      string // starttls (default), tls (implicit, port 465) or none (local catchers only)
}

// SMTP sends through an SMTP server.
type SMTP struct{ cfg Config }

// NewSMTP returns a sender for cfg.
func NewSMTP(cfg Config) *SMTP { return &SMTP{cfg: cfg} }

// Send delivers m.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	raw, err := Build(s.cfg.From, m, time.Now())
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	host, _, err := net.SplitHostPort(s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("%w: mail server address %q: %w", ErrPermanent, s.cfg.Addr, err)
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if s.cfg.TLS == "tls" {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", s.cfg.Addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", s.cfg.Addr)
	}
	if err != nil {
		return err // the server may be down for a moment: retry
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if s.cfg.TLS == "" || s.cfg.TLS == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%w: the mail server does not offer STARTTLS (set HP_SMTP_TLS)", ErrPermanent)
		}
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)); err != nil {
			return fmt.Errorf("%w: signing in to the mail server: %w", ErrPermanent, err)
		}
	}
	if err := c.Mail(s.cfg.From); err != nil {
		return classify(err)
	}
	for _, rcpt := range append(append([]string{}, m.To...), m.Cc...) {
		if err := c.Rcpt(rcpt); err != nil {
			return classify(err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return classify(err)
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return classify(err)
	}
	return c.Quit()
}

// classify marks SMTP 5xx replies (refused for good) as permanent.
func classify(err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) && reply.Code >= 500 {
		return fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return err
}

// Build renders m as an RFC 5322 message.
func Build(from string, m Message, now time.Time) ([]byte, error) {
	for _, v := range append(append([]string{from, m.ReplyTo, m.Subject, m.FromName}, m.To...), m.Cc...) {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("line breaks are not allowed in addresses or the subject")
		}
	}
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("from address: %w", err)
	}
	if m.FromName != "" {
		fromAddr.Name = m.FromName
	}
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", fromAddr.String())
	if m.ReplyTo != "" {
		header("Reply-To", (&mail.Address{Address: m.ReplyTo}).String())
	}
	header("To", strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		header("Cc", strings.Join(m.Cc, ", "))
	}
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+randomID()+"@"+domainOf(fromAddr.Address)+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `text/plain; charset="utf-8"`)
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n"))); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return "hiringplug.local"
}

// Unconfigured is the sender when no mail server is set: every message fails
// with a clear reason, rather than vanishing.
type Unconfigured struct{}

// Send always fails, permanently.
func (Unconfigured) Send(context.Context, Message) error {
	return fmt.Errorf("%w: no mail server is set up (HP_SMTP_HOST)", ErrPermanent)
}
