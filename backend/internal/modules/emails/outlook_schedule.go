package emails

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func scheduledAPI(row store.EmailOutlookSend) api.OutlookScheduledSend {
	return api.OutlookScheduledSend{
		Id: row.ID, MessageId: row.MessageID, Version: int(row.Version), Subject: row.Subject,
		SendAt: row.SendAt, UpdatedAt: row.UpdatedAt, Status: api.OutlookScheduledSendStatus(row.Status), Error: apiconv.OrNull(row.Error),
	}
}

func (s *Service) scheduledOutlook(ctx context.Context, p authz.Principal) (api.OutlookScheduleList, error) {
	out := api.OutlookScheduleList{Items: []api.OutlookScheduledSend{}}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).OutlookRecentSends(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			out.Items = append(out.Items, scheduledAPI(row))
		}
		return nil
	})
	return out, err
}

func validScheduledVersion(version int) bool { return version >= 0 && version < math.MaxInt32 }

func checkOutlookEnvelope(draft outlookRemoteMessage) error {
	body := draft.Body.Content
	if strings.EqualFold(draft.Body.ContentType, "html") {
		body = TextFromHTML(SanitizeHTML(body))
	}
	_, fields := NormalizeOutgoing(Outgoing{
		To: draftRecipientAddresses(draft.ToRecipients), Cc: draftRecipientAddresses(draft.CcRecipients), Bcc: draftRecipientAddresses(draft.BccRecipients),
		Subject: draft.Subject, Body: body,
	})
	return fields.Err()
}

func (s *Service) scheduledCandidate(ctx context.Context, tx pgx.Tx, p authz.Principal, box, conversation string) (*string, error) {
	link, err := store.New(tx).OutlookLink(ctx, store.OutlookLinkParams{MailboxID: box, Conversation: conversation})
	if database.IsNoRows(err) {
		return nil, nil //nolint:nilnil // no linked candidate
	}
	if err != nil {
		return nil, err
	}
	return link.CandidateID, s.canSendToOutlookCandidate(ctx, tx, p, link.CandidateID)
}

func (s *Service) canSendToOutlookCandidate(ctx context.Context, tx pgx.Tx, p authz.Principal, id *string) error {
	if id == nil {
		return nil
	}
	if _, err := s.profileAddress(ctx, tx, p, id, nil); err != nil {
		return err
	}
	return s.contactable(ctx, tx, *id)
}

func (s *Service) scheduleOutlook(ctx context.Context, p authz.Principal, id string, in api.OutlookScheduleInput) (api.OutlookScheduledSend, error) {
	var out api.OutlookScheduledSend
	now := time.Now()
	at := now
	if supplied, err := in.SendAt.Get(); err == nil {
		if !supplied.After(now) || supplied.After(now.AddDate(1, 0, 0)) {
			return out, apperr.Validation(apperr.FieldErrors{{Field: "sendAt", Message: "choose a future time within one year"}})
		}
		at = supplied.UTC()
	}
	if !validScheduledVersion(in.ExpectedVersion) {
		return out, apperr.StaleVersion("scheduled send")
	}
	expected := int32(in.ExpectedVersion) //nolint:gosec // checked above
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		active, err := activeOutlookSend(ctx, q, box.ID, row.RemoteID)
		if err != nil {
			return err
		}
		if active != nil && active.Status != "scheduled" {
			return sendInProgress()
		}
		if active == nil && expected != 0 || active != nil && active.Version != expected {
			return apperr.StaleVersion("scheduled send")
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		remote, err := s.readRemoteDraft(ctx, token, row.RemoteID)
		if err != nil {
			return outlookFailure(err, false)
		}
		if !remote.IsDraft || draftVersion(remote) != in.DraftVersion {
			return draftChanged()
		}
		if err := checkOutlookEnvelope(remote); err != nil {
			return err
		}
		candidate, err := s.scheduledCandidate(ctx, tx, p, box.ID, row.Conversation)
		if err != nil {
			return err
		}
		var saved store.EmailOutlookSend
		if active == nil {
			saved, err = q.CreateOutlookScheduled(ctx, store.CreateOutlookScheduledParams{
				MailboxID: box.ID, MessageID: id, RemoteID: row.RemoteID, DraftVersion: in.DraftVersion,
				CandidateID: candidate, Subject: truncate(remote.Subject, 300), SendAt: at,
			})
		} else {
			saved, err = q.RescheduleOutlookSend(ctx, store.RescheduleOutlookSendParams{
				ID: active.ID, MessageID: id, Version: expected, DraftVersion: in.DraftVersion,
				CandidateID: candidate, Subject: truncate(remote.Subject, 300), SendAt: at,
			})
		}
		if err != nil {
			return err
		}
		payload := api.OutlookScheduledJob{SendId: saved.ID, UserId: p.UserID, Version: int(saved.Version)}
		if _, err := jobqueue.EnqueueAt(ctx, tx, p.TenantID, api.JobKindEmailOutlookScheduled, payload, at); err != nil {
			return err
		}
		out = scheduledAPI(saved)
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.send_scheduled", EntityType: "outlook_send", EntityID: saved.ID})
	})
	return out, err
}

func (s *Service) cancelOutlookSend(ctx context.Context, p authz.Principal, id string, version int) (api.OutlookScheduledSend, error) {
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
		if row.Status != "scheduled" && row.Status != "needs_review" {
			return sendInProgress()
		}
		row, err = q.CancelOutlookSend(ctx, store.CancelOutlookSendParams{ID: id, Version: int32(version)}) //nolint:gosec // checked above
		if database.IsNoRows(err) {
			return apperr.StaleVersion("scheduled send")
		}
		if err != nil {
			return err
		}
		out = scheduledAPI(row)
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.send_cancelled", EntityType: "outlook_send", EntityID: id})
	})
	return out, err
}
