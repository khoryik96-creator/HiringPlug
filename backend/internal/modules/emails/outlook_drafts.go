package emails

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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
)

const outlookDraftFields = "id,changeKey,isDraft,subject,body,toRecipients,ccRecipients,bccRecipients,conversationId,parentFolderId,from,sender,isRead,webLink,hasAttachments,receivedDateTime,createdDateTime,lastModifiedDateTime,sentDateTime"

func (s *Service) readRemoteDraft(ctx context.Context, token, remoteID string) (outlookRemoteMessage, error) {
	var remote outlookRemoteMessage
	path := s.outlookRoot() + "/messages/" + url.PathEscape(remoteID) + "?" + url.Values{"$select": {outlookDraftFields}}.Encode()
	err := s.outlookRequest(ctx, token, http.MethodGet, path, nil, &remote)
	if err == nil && (remote.ID != remoteID || draftVersion(remote) == "") {
		err = &outlookGraphError{status: 502}
	}
	return remote, err
}

func draftVersion(m outlookRemoteMessage) string {
	key := m.ChangeKey
	if key == "" {
		key = m.ETag
	}
	if key == "" && !m.LastModifiedDateTime.IsZero() {
		key = m.LastModifiedDateTime.UTC().Format(time.RFC3339Nano)
	}
	if key == "" {
		return ""
	}
	return stateHash(m.ID + "\n" + key)
}

func draftChanged() error {
	return apperr.New(api.ErrorCodeOUTLOOKDRAFTCHANGED, "This draft changed in Outlook. Reopen it and review the latest version.")
}
func sendInProgress() error {
	return apperr.New(api.ErrorCodeOUTLOOKSENDINPROGRESS, "This draft is being sent or has an unconfirmed outcome. Check the Scheduled list and Outlook.")
}
func draftRecipientAddresses(people []outlookRecipient) []string {
	out := make([]string, 0, len(people))
	for _, person := range people {
		out = append(out, person.EmailAddress.Address)
	}
	return out
}

func draftPatch(in api.OutlookDraftInput) map[string]any {
	out := map[string]any{}
	if in.Subject != nil {
		out["subject"] = strings.TrimSpace(*in.Subject)
	}
	if in.Body != nil {
		out["body"] = map[string]string{"contentType": "text", "content": *in.Body}
	}
	for _, pair := range []struct {
		field     string
		addresses *[]openapi_types.Email
	}{
		{"toRecipients", in.To}, {"ccRecipients", in.Cc}, {"bccRecipients", in.Bcc},
	} {
		if pair.addresses == nil {
			continue
		}
		people := make([]outlookRecipient, 0, len(*pair.addresses))
		for _, address := range *pair.addresses {
			var person outlookRecipient
			person.EmailAddress.Address = strings.TrimSpace(string(address))
			people = append(people, person)
		}
		out[pair.field] = people
	}
	return out
}

func draftView(row store.EmailOutlookMessage, remote outlookRemoteMessage, active *store.EmailOutlookSend) api.OutlookDraft {
	body := remote.Body.Content
	if strings.EqualFold(remote.Body.ContentType, "html") {
		body = TextFromHTML(SanitizeHTML(body))
	}
	shown := truncate(body, 40000)
	var schedule *api.OutlookScheduledSend
	if active != nil {
		v := scheduledAPI(*active)
		schedule = &v
	}
	return api.OutlookDraft{
		MessageId: row.ID, Version: draftVersion(remote), Subject: remote.Subject, Body: shown, BodyTruncated: shown != body,
		To: draftRecipientAddresses(remote.ToRecipients), Cc: draftRecipientAddresses(remote.CcRecipients), Bcc: draftRecipientAddresses(remote.BccRecipients),
		HasAttachments: remote.HasAttachments, WebUrl: apiconv.OrNull(outlookWebLink(remote.WebLink)), Schedule: apiconv.OrNull(schedule),
		ScheduleNeedsReview: active != nil && active.Status == "scheduled" && active.DraftVersion != draftVersion(remote),
	}
}

func activeOutlookSend(ctx context.Context, q *store.Queries, mailbox, remote string) (*store.EmailOutlookSend, error) {
	row, err := q.OutlookActiveSend(ctx, store.OutlookActiveSendParams{MailboxID: mailbox, RemoteID: remote})
	if database.IsNoRows(err) {
		return nil, nil //nolint:nilnil // this draft has no schedule
	}
	return &row, err
}

func (s *Service) writeOutlookDraft(ctx context.Context, p authz.Principal, id, version string, in api.OutlookDraftInput, discard bool) (api.OutlookDraft, error) {
	var out api.OutlookDraft
	providerChanged := false
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		if id == "" {
			draft, accepted, err := s.newOutlookDraft(ctx, q, p, in)
			out, providerChanged = draft, accepted
			if err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "outlook.draft_created", EntityType: "outlook_message", EntityID: out.MessageId})
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
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
		if !remote.IsDraft {
			return draftChanged()
		}
		active, err := activeOutlookSend(ctx, q, box.ID, row.RemoteID)
		if err != nil {
			return err
		}
		// No version means open the draft, without changes.
		if version == "" {
			out = draftView(row, remote, active)
			return nil
		}
		if draftVersion(remote) != version {
			return draftChanged()
		}
		if active != nil && (discard || active.Status != "scheduled") {
			return sendInProgress()
		}
		path := s.outlookMessagePath(row)
		if discard {
			if err := s.outlookConditional(ctx, token, http.MethodDelete, path, nil, nil, remote.ETag); err != nil {
				return outlookDraftWriteFailure(err)
			}
			providerChanged = true
			if err := q.RemoveOutlookMessage(ctx, store.RemoveOutlookMessageParams{FolderID: row.FolderID, RemoteID: row.RemoteID}); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "outlook.draft_discarded", EntityType: "outlook_message", EntityID: id})
		}
		patch := draftPatch(in)
		if len(patch) == 0 {
			out = draftView(row, remote, active)
			return nil
		}
		var updated outlookRemoteMessage
		if err := s.outlookConditional(ctx, token, http.MethodPatch, path, patch, &updated, remote.ETag); err != nil {
			return outlookDraftWriteFailure(err)
		}
		providerChanged = true
		// Bind the schedule to the PATCH receipt so an unrelated edit made
		// immediately afterward never becomes the reviewed version.
		if updated.ID != row.RemoteID || !updated.IsDraft || draftVersion(updated) == "" {
			return draftChanged()
		}
		row, err = saveOutlookDraftIndex(ctx, q, box.ID, row.FolderID, updated)
		if err != nil {
			return err
		}
		if active != nil && active.DraftVersion == version {
			if err := q.ReviewOutlookScheduled(ctx, store.ReviewOutlookScheduledParams{ID: active.ID, DraftVersion: draftVersion(updated), Subject: row.Subject, PreviousDraftVersion: version}); err != nil {
				return err
			}
			active.Subject = row.Subject
			active.DraftVersion = draftVersion(updated)
		}
		out = draftView(row, updated, active)
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.draft_updated", EntityType: "outlook_message", EntityID: id})
	})
	if providerChanged && err != nil {
		return out, outlookFailure(err, true)
	}
	return out, err
}

func outlookDraftWriteFailure(err error) error {
	var graph *outlookGraphError
	if errors.As(err, &graph) && (graph.status == 409 || graph.status == 412) {
		return draftChanged()
	}
	return outlookFailure(err, true)
}

func (s *Service) newOutlookDraft(ctx context.Context, q *store.Queries, p authz.Principal, in api.OutlookDraftInput) (api.OutlookDraft, bool, error) {
	var zero api.OutlookDraft
	box, err := q.MailboxForDelivery(ctx, p.UserID)
	if database.IsNoRows(err) || err == nil && box.Provider != ProviderMicrosoft {
		return zero, false, outlookDisconnected()
	}
	if err != nil {
		return zero, false, err
	}
	folder, err := q.OutlookDraftFolder(ctx, box.ID)
	if database.IsNoRows(err) {
		return zero, false, apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, "Enable Outlook sync and wait for the Drafts folder to finish loading.")
	}
	if err != nil {
		return zero, false, err
	}
	token, err := s.mailboxAccess(ctx, box)
	if err != nil {
		return zero, false, outlookFailure(err, false)
	}
	var remote outlookRemoteMessage
	err = s.outlookRequest(ctx, token, http.MethodPost, s.outlookRoot()+"/messages", draftPatch(in), &remote)
	if err != nil {
		return zero, false, outlookFailure(err, true)
	}
	if !remote.IsDraft || remote.ID == "" || draftVersion(remote) == "" {
		return zero, true, outlookFailure(nil, true)
	}
	row, err := saveOutlookDraftIndex(ctx, q, box.ID, folder.ID, remote)
	if err != nil {
		return zero, true, err
	}
	return draftView(row, remote, nil), true, nil
}

func saveOutlookDraftIndex(ctx context.Context, q *store.Queries, box, folder string, remote outlookRemoteMessage) (store.EmailOutlookMessage, error) {
	conversation := remote.ConversationID
	if conversation == "" {
		conversation = remote.ID
	}
	date := remote.ReceivedDateTime
	if date.IsZero() {
		date = remote.CreatedDateTime
	}
	if date.IsZero() {
		date = time.Now()
	}
	modified := remote.LastModifiedDateTime
	if modified.IsZero() {
		modified = time.Now()
	}
	err := q.SaveOutlookMessage(ctx, store.SaveOutlookMessageParams{
		MailboxID: box, FolderID: folder, RemoteID: remote.ID, Conversation: conversation,
		Subject: truncate(remote.Subject, 300), Sender: truncate(remote.sender(), 254), Participants: outlookParticipants(remote),
		ReceivedAt: date, ChangedAt: modified, IsRead: remote.IsRead, IsDraft: remote.IsDraft, HasAttachments: remote.HasAttachments,
		WebUrl: outlookWebLink(remote.WebLink),
	})
	if err != nil {
		return store.EmailOutlookMessage{}, err
	}
	return q.OutlookByRemoteID(ctx, store.OutlookByRemoteIDParams{MailboxID: box, RemoteID: remote.ID})
}
