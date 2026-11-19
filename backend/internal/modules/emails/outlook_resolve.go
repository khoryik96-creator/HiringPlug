package emails

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Resolving an unknown outcome reads the original immutable ID. An existing
// draft can be reviewed again, but this operation never sends or queues it.
func (s *Service) resolveOutlookSend(ctx context.Context, p authz.Principal, id string, version int) (api.OutlookScheduledSend, error) {
	var out api.OutlookScheduledSend
	if !validScheduledVersion(version) {
		return out, apperr.StaleVersion("scheduled send")
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		row, err := q.OutlookScheduled(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("scheduled send")
		}
		if err != nil {
			return err
		}
		if row.Status != "uncertain" || int(row.Version) != version {
			return apperr.StaleVersion("scheduled send")
		}
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) || err == nil && (box.ID != row.MailboxID || box.Provider != ProviderMicrosoft) {
			return apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, "Connect the original Outlook mailbox to check this send.")
		}
		if err != nil {
			return err
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		remote, err := s.readRemoteDraft(ctx, token, row.RemoteID)
		if err != nil {
			return outlookFailure(err, false)
		}
		status, message := "sent", ""
		if remote.IsDraft {
			status = "needs_review"
			message = "Outlook still has this draft. Check Sent Items before scheduling it again."
		} else if remote.SentDateTime.IsZero() {
			return sendInProgress()
		}
		resolved, err := q.ResolveOutlookSend(ctx, store.ResolveOutlookSendParams{
			ID: row.ID, Version: row.Version, Status: status, Error: orNil(message),
		})
		if database.IsNoRows(err) {
			return apperr.StaleVersion("scheduled send")
		}
		if err != nil {
			return err
		}
		out = scheduledAPI(resolved)
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.send_resolved", EntityType: "outlook_send", EntityID: id,
			Changes: map[string]any{"status": status}})
	})
	return out, err
}
