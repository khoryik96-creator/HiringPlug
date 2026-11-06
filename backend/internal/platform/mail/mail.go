// Package mail sends email through an SMTP server (the company's own, or a
// service such as Amazon SES, SendGrid or Mailgun via their SMTP endpoints).
//
// Messages are UTF-8: plain text, or HTML with a plain-text version beside it,
// inline images (a banner) and attachments. Addresses, the subject and file
// names are checked for line breaks, so nothing typed into an email can add
// headers. Bcc addresses get the message but never appear in it.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
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
	Bcc      []string // receive it, but are not listed in it
	Subject  string
	Body     string // plain text (always: the version for mail apps that show no HTML)
	HTML     string // optional formatted version; images in it refer to Inline parts by cid:
	Inline   []Part // images the HTML shows (e.g. a banner)
	Attach   []Part // attachments
	// Calendar is an iCalendar invitation (RFC 5546: METHOD:REQUEST or
	// CANCEL). It goes in as another version of the message, which is how
	// Gmail, Outlook and Apple Mail recognise an invitation, and as an
	// invite.ics attachment for the others.
	Calendar string
}

// Part is a file in a message.
type Part struct {
	Name        string // file name
	ContentType string // e.g. image/png, application/pdf
	ContentID   string // for inline parts: what cid: in the HTML refers to
	Data        []byte
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
	for _, rcpt := range append(append(append([]string{}, m.To...), m.Cc...), m.Bcc...) {
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
	values := append(append(append([]string{from, m.ReplyTo, m.Subject, m.FromName}, m.To...), m.Cc...), m.Bcc...)
	for _, p := range append(append([]Part{}, m.Inline...), m.Attach...) {
		values = append(values, p.Name, p.ContentType, p.ContentID)
	}
	for _, v := range values {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("line breaks are not allowed in addresses, the subject or file names")
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
	contentType, body, err := content(m)
	if err != nil {
		return nil, err
	}
	header("Content-Type", contentType)
	if !strings.HasPrefix(contentType, "multipart/") {
		header("Content-Transfer-Encoding", "quoted-printable")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes(), nil
}

// content is the message's body and its type: plain text alone, or the
// text and HTML versions as alternatives, with inline images and attachments
// wrapped around them as needed.
func content(m Message) (string, []byte, error) {
	text, err := quoted(m.Body)
	if err != nil {
		return "", nil, err
	}
	ctype, body := `text/plain; charset="utf-8"`, text
	if m.Calendar != "" {
		cal, method, err := calendarPart(m.Calendar)
		if err != nil {
			return "", nil, err
		}
		alt := []part{{header: qpHeader(`text/plain; charset="utf-8"`), body: text}}
		if m.HTML != "" {
			html, err := quoted(m.HTML)
			if err != nil {
				return "", nil, err
			}
			alt = append(alt, part{header: qpHeader(`text/html; charset="utf-8"`), body: html})
		}
		alt = append(alt, cal)
		if ctype, body, err = multipartOf("alternative", alt); err != nil {
			return "", nil, err
		}
		parts := []part{{header: textproto.MIMEHeader{"Content-Type": {ctype}}, body: body}}
		parts = append(parts, filePart(Part{Name: "invite.ics", ContentType: "application/ics; method=" + method, Data: []byte(m.Calendar)}, "attachment"))
		for _, p := range m.Attach {
			parts = append(parts, filePart(p, "attachment"))
		}
		return multipartOf("mixed", parts)
	}
	if m.HTML != "" {
		html, err := quoted(m.HTML)
		if err != nil {
			return "", nil, err
		}
		ctype, body, err = multipartOf("alternative", []part{
			{header: qpHeader(`text/plain; charset="utf-8"`), body: text},
			{header: qpHeader(`text/html; charset="utf-8"`), body: html},
		})
		if err != nil {
			return "", nil, err
		}
		if len(m.Inline) > 0 {
			parts := []part{{header: textproto.MIMEHeader{"Content-Type": {ctype}}, body: body}}
			for _, p := range m.Inline {
				parts = append(parts, filePart(p, "inline"))
			}
			if ctype, body, err = multipartOf("related", parts); err != nil {
				return "", nil, err
			}
		}
	}
	if len(m.Attach) > 0 {
		parts := []part{{header: textproto.MIMEHeader{"Content-Type": {ctype}}, body: body}}
		if !strings.HasPrefix(ctype, "multipart/") {
			parts[0].header.Set("Content-Transfer-Encoding", "quoted-printable")
		}
		for _, p := range m.Attach {
			parts = append(parts, filePart(p, "attachment"))
		}
		return multipartOf("mixed", parts)
	}
	return ctype, body, nil
}

// calendarPart is the invitation as a text/calendar part, with the METHOD
// it declares (REQUEST, CANCEL...) in its content type, as RFC 6047 asks.
func calendarPart(ics string) (part, string, error) {
	method := ""
	for _, line := range strings.Split(ics, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "METHOD:"); ok {
			method = strings.ToUpper(v)
			break
		}
	}
	if method == "" {
		return part{}, "", errors.New("mail: a calendar invitation needs a METHOD")
	}
	h := textproto.MIMEHeader{
		"Content-Type":              {`text/calendar; charset="utf-8"; method=` + method},
		"Content-Transfer-Encoding": {"base64"},
	}
	enc := base64.StdEncoding.EncodeToString([]byte(ics))
	var lines bytes.Buffer
	for len(enc) > 76 {
		lines.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	lines.WriteString(enc + "\r\n")
	return part{header: h, body: lines.Bytes()}, method, nil
}

type part struct {
	header textproto.MIMEHeader
	body   []byte
}

func qpHeader(ctype string) textproto.MIMEHeader {
	return textproto.MIMEHeader{"Content-Type": {ctype}, "Content-Transfer-Encoding": {"quoted-printable"}}
}

// filePart is a base64 part for a file, inline (with its Content-ID) or attached.
func filePart(p Part, disposition string) part {
	h := textproto.MIMEHeader{
		"Content-Type":              {p.ContentType},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {mime.FormatMediaType(disposition, map[string]string{"filename": p.Name})},
	}
	if p.ContentID != "" {
		h.Set("Content-ID", "<"+p.ContentID+">")
	}
	enc := base64.StdEncoding.EncodeToString(p.Data)
	var lines bytes.Buffer
	for len(enc) > 76 {
		lines.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	lines.WriteString(enc + "\r\n")
	return part{header: h, body: lines.Bytes()}
}

func multipartOf(kind string, parts []part) (string, []byte, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for _, p := range parts {
		pw, err := w.CreatePart(p.header)
		if err != nil {
			return "", nil, err
		}
		if _, err := pw.Write(p.body); err != nil {
			return "", nil, err
		}
	}
	if err := w.Close(); err != nil {
		return "", nil, err
	}
	return "multipart/" + kind + `; boundary="` + w.Boundary() + `"`, b.Bytes(), nil
}

// quoted is text with CRLF line ends, quoted-printable.
func quoted(text string) ([]byte, error) {
	var b bytes.Buffer
	qp := quotedprintable.NewWriter(&b)
	if _, err := qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n"))); err != nil {
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
