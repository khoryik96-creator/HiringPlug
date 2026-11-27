package emails

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

func (m *Module) OutsideJobHandlers() jobqueue.OutsideHandlers {
	return jobqueue.OutsideHandlers{
		api.JobKindEmailOutlookScheduled:  m.service.submitOutlook,
		api.JobKindEmailSend:              m.service.deliverOutside,
		api.JobKindEmailInterviewReminder: m.service.deliverOutside,
	}
}

// DeadJobHandlers end what a sending job was for when it ends without
// running (cancelled after a restore, or stopped for good), and keep a
// mailbox's Outlook sync going (each run queues the next).
func (m *Module) DeadJobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{
		api.JobKindEmailOutlookSync:       outlookSyncDied,
		api.JobKindEmailOutlookScheduled:  outlookSendDead,
		api.JobKindEmailSend:              m.service.sendDied,
		api.JobKindEmailInterviewReminder: m.service.sendDied,
	}
}

// Why an email whose job ended was not sent.
const (
	notSentRestore = "Not sent: cancelled after the server was restored (it was waiting in the backup)."
	notSentStopped = "Not sent: sending it stopped before it was tried. Send it again if it is still needed."
)

// sendDied marks an email whose sending job ended without trying it as not
// sent: queued for ever otherwise. One whose delivery was attempted may
// have gone, and is left as it is (the job itself decided it).
func (s *Service) sendDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.EmailSendJob
	if err := job.Decode(&payload); err != nil {
		return nil //nolint:nilerr // an unreadable job names no email to update
	}
	why := notSentStopped
	if jobqueue.WasCancelledAfterRestore(job) {
		why = notSentRestore
	}
	row, err := store.New(tx).FailUnsentEmail(ctx, store.FailUnsentEmailParams{ID: payload.EmailId, Error: &why})
	if database.IsNoRows(err) {
		return nil // sent, failed, tried, or gone
	}
	if err != nil {
		return err
	}
	if row.FeesRestricted && s.deps.Invoices != nil { // an invoice's email: the invoice shows it was not sent
		if err := s.deps.Invoices.Delivered(ctx, tx, row.ID, true); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "email.failed", EntityType: "email", EntityID: row.ID, Changes: map[string]any{"error": why}})
}

func outlookSendDead(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.OutlookScheduledJob
	if err := job.Decode(&payload); err != nil {
		return nil //nolint:nilerr // The invalid job has no matching send to update.
	}
	row, err := store.New(tx).OutlookScheduled(ctx, payload.SendId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.UserID != payload.UserId || int(row.Version) != payload.Version {
		return nil
	}
	switch row.Status {
	case "scheduled":
		return finishOutlookSend(ctx, tx, row, "needs_review", "HyPlug could not process the scheduled send. Review it before scheduling again.")
	case "confirming":
		return finishOutlookSend(ctx, tx, row, "uncertain", "HyPlug could not confirm the send. Check Outlook Drafts and Sent Items.")
	default:
		return nil
	}
}

const confirmingWindow = 15 * time.Minute

// A dedicated connection holds the same advisory lock that draft edits and
// mailbox replacement take. The dispatch claim commits before Outlook is
// called. A retried job reconciles a prior claim and never repeats the POST.
func (s *Service) submitOutlook(ctx context.Context, job jobqueue.Job) error {
	var payload api.OutlookScheduledJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	work, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(work, s.pool.Config().ConnConfig.Copy())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	locked, err := store.New(conn).TryLockOutlookSendMailbox(work, store.TryLockOutlookSendMailboxParams{TenantID: job.TenantID, UserID: payload.UserId})
	if err != nil {
		return err
	}
	if !locked {
		return jobqueue.Later(time.Now().Add(15 * time.Second))
	}
	return s.submitOutlookLocked(work, conn, job.TenantID, payload)
}

func (s *Service) submitOutlookLocked(ctx context.Context, conn *pgx.Conn, tenant string, payload api.OutlookScheduledJob) error {
	system := authz.System(tenant)
	var active bool
	var p authz.Principal
	var row store.EmailOutlookSend
	err := database.InTx(ctx, conn, system, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).OutlookScheduled(ctx, payload.SendId)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.UserID != payload.UserId || int(row.Version) != payload.Version || row.Status != "scheduled" && row.Status != "confirming" {
			row.ID = ""
			return nil
		}
		p, active, err = s.deps.Users.PrincipalFor(ctx, tx, row.UserID)
		return err
	})
	if err != nil || row.ID == "" {
		return err
	}
	if !active || !p.Can(api.PermissionEmailsSend) {
		status := "needs_review"
		if row.Status == "confirming" {
			status = "uncertain"
		}
		return settleOutlook(ctx, conn, system, row, status, "Mailbox access changed. Check the draft in Outlook.")
	}
	if row.Status == "scheduled" && time.Now().Before(row.SendAt) {
		return jobqueue.Later(row.SendAt)
	}
	if row.Status == "scheduled" && time.Since(row.SendAt) > 24*time.Hour {
		return settleOutlook(ctx, conn, p, row, "needs_review", "The scheduled time passed over a day ago. Review the draft before sending.")
	}
	var token string
	var box store.EmailMailbox
	var invalid string
	err = database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		box, err = q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) || err == nil && (box.ID != row.MailboxID || box.Provider != ProviderMicrosoft) {
			invalid = "The original Outlook mailbox is no longer connected."
			return nil
		}
		if err != nil {
			return err
		}

		token, err = s.mailboxAccess(ctx, box)
		if err != nil && errors.Is(err, mail.ErrPermanent) {
			invalid = "Reconnect the original Outlook mailbox to use this draft."
			return nil
		}
		return err
	})
	if err != nil {
		if row.Status == "confirming" {
			return waitOutlookReceipt(ctx, conn, p, row)
		}
		return jobqueue.Later(time.Now().Add(time.Minute))
	}
	if invalid != "" {
		status := "needs_review"
		if row.Status == "confirming" {
			status = "uncertain"
		}
		return settleOutlook(ctx, conn, p, row, status, invalid)
	}
	remote, err := s.readRemoteDraft(ctx, token, row.RemoteID)
	if err == nil && !remote.IsDraft && !remote.SentDateTime.IsZero() {
		return settleOutlook(ctx, conn, p, row, "sent", "")
	}
	if row.Status == "confirming" {
		return waitOutlookReceipt(ctx, conn, p, row)
	}
	if err != nil {
		var graph *outlookGraphError
		if errors.As(err, &graph) && (graph.status == 401 || graph.status == 403 || graph.status == 404) {
			return settleOutlook(ctx, conn, p, row, "needs_review", "Outlook could not find or open the original draft. Review it before sending.")
		}
		return retryOutlook(err)
	}
	if !remote.IsDraft || draftVersion(remote) != row.DraftVersion {
		return settleOutlook(ctx, conn, p, row, "needs_review", "The draft changed in Outlook after it was scheduled. Review it before sending.")
	}

	err = database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		if err := s.checkOutlookEnvelope(ctx, tx, p.TenantID, draftEnvelope(remote, nil)); err != nil {
			return err
		}
		// Consent is rechecked right before the claim, using the send's pinned
		// record even if the user unlinked the Outlook conversation meanwhile.
		if err := s.canSendToOutlookCandidate(ctx, tx, p, row.CandidateID); err != nil {
			return err
		}
		if _, err := s.scheduledCandidate(ctx, tx, p, box.ID, remote.ConversationID); err != nil {
			return err
		}
		n, err := store.New(tx).ClaimOutlookSend(ctx, store.ClaimOutlookSendParams{ID: row.ID, Version: row.Version})
		if err != nil {
			return err
		}
		if n != 1 {
			return apperr.StaleVersion("scheduled send")
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.send_started", EntityType: "outlook_send", EntityID: row.ID})
	})
	if err != nil {
		if problem, safe := apperr.As(err); safe {
			if problem.Code == api.ErrorCodeEMAILDONOTCONTACT {
				return settleOutlook(ctx, conn, p, row, "needs_review", "A recipient opted out of being contacted. Review the recipients before sending.")
			}
			return settleOutlook(ctx, conn, p, row, "needs_review", "The scheduled send needs review before it can go out.")
		}
		return err
	}
	row.Status = "confirming"
	now := time.Now()
	row.AttemptedAt = &now
	target := s.outlookRoot() + "/messages/" + url.PathEscape(row.RemoteID) + "/send"
	sendErr := s.outlookConditional(ctx, token, http.MethodPost, target, nil, nil, remote.ETag)
	var graph *outlookGraphError
	if errors.As(sendErr, &graph) {
		switch graph.status {
		case 429:
			// Graph explicitly refused this attempt before accepting the message.
			if err := settleOutlook(ctx, conn, p, row, "scheduled", "Outlook is throttling sends. HyPlug will retry."); err != nil {
				return err
			}
			return retryOutlook(sendErr)
		case 400, 401, 403, 405, 409, 412, 413:
			return settleOutlook(ctx, conn, p, row, "needs_review", "Outlook refused the send. Open the draft and review it.")
		}
	}
	// 202, an interrupted connection, or an ambiguous server error: verify the
	// same immutable ID rather than trusting the response or posting again.
	sent, err := s.readRemoteDraft(ctx, token, row.RemoteID)
	if err == nil && !sent.IsDraft && !sent.SentDateTime.IsZero() {
		return settleOutlook(ctx, conn, p, row, "sent", "")
	}
	return waitOutlookReceipt(ctx, conn, p, row)
}

func retryOutlook(err error) error {
	at := time.Now().Add(time.Minute)
	var graph *outlookGraphError
	if errors.As(err, &graph) && graph.retry.After(at) {
		at = graph.retry
	}
	return jobqueue.Later(at)
}
func waitOutlookReceipt(ctx context.Context, conn *pgx.Conn, p authz.Principal, row store.EmailOutlookSend) error {
	if row.AttemptedAt != nil && time.Since(*row.AttemptedAt) < confirmingWindow {
		return jobqueue.Later(time.Now().Add(30 * time.Second))
	}
	return settleOutlook(ctx, conn, p, row, "uncertain", "Outlook has not confirmed delivery. Check Drafts and Sent Items before taking action.")
}
func settleOutlook(ctx context.Context, conn *pgx.Conn, p authz.Principal, row store.EmailOutlookSend, status, message string) error {
	return database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		return finishOutlookSend(ctx, tx, row, status, message)
	})
}

func finishOutlookSend(ctx context.Context, tx pgx.Tx, row store.EmailOutlookSend, status, message string) error {
	n, err := store.New(tx).FinishOutlookSend(ctx, store.FinishOutlookSendParams{
		ID: row.ID, Version: row.Version, PreviousStatus: row.Status, Status: status, Error: orNil(message),
	})
	if err != nil || n == 0 {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "outlook.send_status", EntityType: "outlook_send", EntityID: row.ID, Changes: map[string]any{"status": status}})
}
