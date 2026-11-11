package emails

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

var errPrivateMailboxSMTP = errors.New("SMTP server resolves to a private network address")

func (s *Service) deliverThroughMailbox(ctx context.Context, tx pgx.Tx, email store.Email, msg mail.Message) error {
	if email.MailboxRef == nil {
		mode := s.serverMode()
		if mode == "unconfigured" {
			mode = "unknown"
		}
		if email.DeliveryMode != "unknown" && mode != email.DeliveryMode {
			return fmt.Errorf("%w: the server's delivery mode changed; review and send the email again", mail.ErrPermanent)
		}
		return s.sender.Send(ctx, msg)
	}
	row, err := store.New(tx).MailboxForDelivery(ctx, email.SentBy)
	if database.IsNoRows(err) || err == nil && row.ID != *email.MailboxRef {
		return fmt.Errorf("%w: the selected mailbox was disconnected or replaced; review and send the email again", mail.ErrPermanent)
	}
	if err != nil {
		return err
	}
	if row.Provider != "smtp" {
		return s.sendOAuthMailbox(ctx, row, msg)
	}
	var secret mailboxSecret
	if err = s.openMailbox(row.TenantID, row.UserID, row.Provider, row.SealedSecret, &secret); err != nil {
		return fmt.Errorf("%w: mailbox credentials cannot be opened; reconnect the mailbox", mail.ErrPermanent)
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if safehttp.Blocked(net.ParseIP(host), s.connections.AllowPrivate) {
			return fmt.Errorf("%w: %w", mail.ErrPermanent, errPrivateMailboxSMTP)
		}
		return nil
	}}
	sender := mail.NewSMTP(mail.Config{Addr: smtpAddress(row.Host, row.Port), Username: row.Username, Password: secret.Password, From: row.Address, TLS: row.Tls, DialContext: dialer.DialContext})
	msg.ReplyTo = row.Address
	return mailboxSMTPError(sender.Send(ctx, msg))
}

// A remote SMTP response can contain arbitrary text, including message data or
// credentials. Keep only status codes and our own fixed diagnostic messages.
func mailboxSMTPError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errPrivateMailboxSMTP) {
		return fmt.Errorf("%w: the SMTP server resolves to a private network address", mail.ErrPermanent)
	}
	var reply *textproto.Error
	if errors.As(err, &reply) {
		if errors.Is(err, mail.ErrPermanent) {
			return fmt.Errorf("%w: SMTP refused delivery (code %d); check credentials and sender permissions", mail.ErrPermanent, reply.Code)
		}
		return fmt.Errorf("SMTP temporarily refused delivery (code %d)", reply.Code)
	}
	if errors.Is(err, mail.ErrPermanent) {
		return fmt.Errorf("%w: SMTP settings or authorization were refused; check the connection", mail.ErrPermanent)
	}
	return errors.New("SMTP connection or delivery temporarily failed; check the server and security settings")
}
