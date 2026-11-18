package emails

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

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

func draftEnvelope(draft outlookRemoteMessage, changes *api.OutlookDraftInput) Outgoing {
	body := draft.Body.Content
	if strings.EqualFold(draft.Body.ContentType, "html") {
		body = TextFromHTML(SanitizeHTML(body))
	}
	out := Outgoing{
		To: draftRecipientAddresses(draft.ToRecipients), Cc: draftRecipientAddresses(draft.CcRecipients), Bcc: draftRecipientAddresses(draft.BccRecipients),
		Subject: draft.Subject, Body: body,
	}
	if changes == nil {
		return out
	}
	if changes.Subject != nil {
		out.Subject = *changes.Subject
	}
	if changes.Body != nil {
		out.Body = *changes.Body
	}
	for _, field := range []struct {
		change *[]openapi_types.Email
		target *[]string
	}{{changes.To, &out.To}, {changes.Cc, &out.Cc}, {changes.Bcc, &out.Bcc}} {
		if field.change == nil {
			continue
		}
		addresses := make([]string, 0, len(*field.change))
		for _, address := range *field.change {
			addresses = append(addresses, string(address))
		}
		*field.target = addresses
	}
	return out
}

func (s *Service) checkOutlookEnvelope(ctx context.Context, tx pgx.Tx, tenant string, envelope Outgoing) error {
	clean, fields := NormalizeOutgoing(envelope)
	if err := fields.Err(); err != nil {
		return err
	}
	if s.deps.Consent == nil || s.deps.Candidates == nil {
		return nil
	}
	// A recruiter may know an address without having access to the candidate
	// record. Enforce consent across the tenant without revealing that record.
	return database.AsSystem(ctx, tx, tenant, func() error {
		ids, err := s.deps.Candidates.IDsByEmails(ctx, tx, slices.Concat(clean.To, clean.Cc, clean.Bcc))
		if err != nil {
			return err
		}
		for _, id := range ids {
			withdrawn, err := s.deps.Consent.DoNotContact(ctx, tx, id)
			if err != nil {
				return err
			}
			if withdrawn {
				return apperr.New(api.ErrorCodeEMAILDONOTCONTACT, "A recipient opted out of being contacted. Check the email addresses before sending.")
			}
		}
		return nil
	})
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
		draft, err := s.openOutlookDraft(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		box, row, remote := draft.box, draft.row, draft.remote
		active, err := activeOutlookSend(ctx, q, box.ID, row.RemoteID)
		if err != nil {
			return err
		}
		if active != nil && active.Status != "scheduled" && active.Status != "needs_review" {
			return sendInProgress()
		}
		if active == nil && expected != 0 || active != nil && active.Version != expected {
			return apperr.StaleVersion("scheduled send")
		}
		if draftVersion(remote) != in.DraftVersion {
			return draftChanged()
		}
		if err := s.checkOutlookEnvelope(ctx, tx, p.TenantID, draftEnvelope(remote, in.Changes)); err != nil {
			return err
		}
		candidate, err := s.scheduledCandidate(ctx, tx, p, box.ID, remote.ConversationID)
		if err != nil {
			return err
		}
		if active != nil && active.CandidateID != nil {
			if err := s.canSendToOutlookCandidate(ctx, tx, p, active.CandidateID); err != nil {
				return err
			}
			if candidate == nil {
				candidate = active.CandidateID
			}
		}
		if in.Changes != nil {
			patch := draftPatch(*in.Changes)
			if len(patch) > 0 {
				// The mailbox lock spans the Graph patch and the new schedule
				// commit. The old job can never claim the revised content.
				remote, err = s.patchRemoteDraft(ctx, draft, patch)
				if err != nil {
					return err
				}
				if err := s.checkOutlookEnvelope(ctx, tx, p.TenantID, draftEnvelope(remote, nil)); err != nil {
					return err
				}
				row, err = saveOutlookDraftIndex(ctx, q, box.ID, row.FolderID, remote)
				if err != nil {
					return err
				}
			}
		}
		var saved store.EmailOutlookSend
		if active == nil {
			saved, err = q.CreateOutlookScheduled(ctx, store.CreateOutlookScheduledParams{
				MailboxID: box.ID, MessageID: row.ID, RemoteID: row.RemoteID, DraftVersion: draftVersion(remote),
				CandidateID: candidate, Subject: truncate(remote.Subject, 300), SendAt: at,
			})
		} else {
			saved, err = q.RescheduleOutlookSend(ctx, store.RescheduleOutlookSendParams{
				ID: active.ID, MessageID: row.ID, Version: expected, DraftVersion: draftVersion(remote),
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
