package emails

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// deliver handles email.send: it sends one queued email through the mail
// server, in a transaction scoped to the job's tenant.
//
// A failure worth retrying (the server is down for a moment) fails the job, so
// the queue tries again later and the email stays queued; a permanent one (a
// refused address, no mail server set up) or the last attempt marks the email
// failed with the reason. Delivery is at least once: if the server accepted
// the email but the transaction then could not commit, it is sent again.
func (s *Service) deliver(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.EmailSendJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	e, err := q.GetEmail(ctx, payload.EmailId)
	if database.IsNoRows(err) {
		logx.From(ctx).Info("email no longer exists; not sending it", "email_id", payload.EmailId)
		return nil
	}
	if err != nil {
		return err
	}
	if e.Status != string(api.EmailStatusQueued) {
		return nil // already sent, or given up on
	}
	agency, err := s.deps.Tenants.TenantName(ctx, tx, job.TenantID)
	if err != nil {
		return err
	}
	msg := mail.Message{
		FromName: e.SenderName + " (" + agency + ")", ReplyTo: e.ReplyTo,
		To: e.ToAddresses, Cc: e.CcAddresses, Bcc: e.BccAddresses, Subject: e.Subject, Body: e.Body,
	}
	if e.Calendar != nil {
		msg.Calendar = *e.Calendar
	}
	var sendErr error
	switch partsErr := s.files2parts(ctx, q, e, &msg); {
	case errors.Is(partsErr, os.ErrNotExist): // gone for good: retrying cannot help
		sendErr = fmt.Errorf("%w: a banner or attached file of this email is missing", mail.ErrPermanent)
	case partsErr != nil:
		return partsErr
	default:
		sendErr = s.sender.Send(ctx, msg)
	}
	if sendErr != nil && !errors.Is(sendErr, mail.ErrPermanent) && job.Attempts < job.MaxAttempts {
		return sendErr
	}
	status, action := api.EmailStatusSent, "email.sent"
	var reason *string
	changes := map[string]any{}
	if sendErr != nil {
		status, action = api.EmailStatusFailed, "email.failed"
		msg := truncate(sendErr.Error(), 1000)
		reason, changes["error"] = &msg, msg
	}
	if err := q.MarkEmail(ctx, store.MarkEmailParams{ID: e.ID, Status: string(status), Error: reason}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "email", EntityID: e.ID, Changes: changes})
}

// files2parts adds the formatted message with its banners, and the
// attachments, to msg.
func (s *Service) files2parts(ctx context.Context, q *store.Queries, e store.Email, msg *mail.Message) error {
	if e.BodyHtml != nil && *e.BodyHtml != "" {
		msg.HTML = WithBannerCIDs(*e.BodyHtml)
		banners, err := q.GetBanners(ctx, BannerIDs(*e.BodyHtml))
		if err != nil {
			return err
		}
		for _, b := range banners {
			data, err := s.read(b.StorageKey)
			if err != nil {
				return err
			}
			msg.Inline = append(msg.Inline, mail.Part{Name: "banner" + extension(b.ContentType), ContentType: b.ContentType, ContentID: b.ID, Data: data})
		}
	}
	files, err := q.EmailAttachments(ctx, []string{e.ID})
	if err != nil {
		return err
	}
	for _, f := range files {
		data, err := s.read(f.StorageKey)
		if err != nil {
			return err
		}
		msg.Attach = append(msg.Attach, mail.Part{Name: f.FileName, ContentType: f.ContentType, Data: data})
	}
	return nil
}

func (s *Service) read(key string) ([]byte, error) {
	f, err := s.files.Open(key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func extension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
