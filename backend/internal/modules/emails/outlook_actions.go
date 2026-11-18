package emails

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) outlookMessagePath(row store.EmailOutlookMessage) string {
	return s.outlookRoot() + "/messages/" + url.PathEscape(row.RemoteID)
}

func outlookFailure(err error, write bool) error {
	var graph *outlookGraphError
	if errors.As(err, &graph) {
		switch graph.status {
		case 404:
			return apperr.NotFound("Outlook message")
		case 401, 403:
			return apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, "Outlook denied mailbox access. Reconnect in Email connection.")
		case 400, 405, 409, 412:
			return apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, "Outlook refused the change. Refresh the mailbox and try again.")
		}
	}
	message := "Could not reach Outlook. Check the mailbox connection and try again."
	if write {
		message = "Outlook did not confirm the change. Check Outlook before trying again."
	}
	return apperr.New(api.ErrorCodeOUTLOOKUNAVAILABLE, message)
}

func (s *Service) outlookBody(ctx context.Context, p authz.Principal, id string) (api.OutlookMessageBody, error) {
	var out api.OutlookMessageBody
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		var remote outlookRemoteMessage
		if err := s.outlookRequest(ctx, token, http.MethodGet, s.outlookMessagePath(row)+"?$select=body", nil, &remote); err != nil {
			return outlookFailure(err, false)
		}
		body := remote.Body.Content
		if strings.EqualFold(remote.Body.ContentType, "html") {
			body = TextFromHTML(SanitizeHTML(body))
		}
		out.Text = truncate(body, 40000)
		out.Truncated = out.Text != body
		return nil
	})
	return out, err
}

func (s *Service) changeOutlook(ctx context.Context, p authz.Principal, id string, in api.OutlookMessageChange) error {
	if (in.IsRead == nil) == (in.FolderId == nil) {
		return apperr.Validation(apperr.FieldErrors{{Field: "message", Message: "choose a read-status change or a destination folder"}})
	}
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		path := s.outlookMessagePath(row)
		var remote outlookRemoteMessage
		if in.FolderId == nil {
			if err := s.outlookRequest(ctx, token, http.MethodPatch, path, map[string]bool{"isRead": *in.IsRead}, &remote); err != nil {
				return outlookFailure(err, true)
			}
			if err := q.SetOutlookRead(ctx, store.SetOutlookReadParams{ID: id, IsRead: *in.IsRead, ChangedAt: remote.LastModifiedDateTime}); err != nil {
				return err
			}
		} else {
			folder, err := q.OutlookFolder(ctx, store.OutlookFolderParams{ID: *in.FolderId, MailboxID: box.ID})
			if database.IsNoRows(err) {
				return apperr.NotFound("Outlook folder")
			}
			if err != nil {
				return err
			}
			if row.FolderID == folder.ID {
				return nil
			}
			if err := s.outlookRequest(ctx, token, http.MethodPost, path+"/move", map[string]string{"destinationId": folder.RemoteID}, &remote); err != nil {
				return outlookFailure(err, true)
			}
			if remote.ID != row.RemoteID {
				return outlookFailure(nil, true)
			}
			if err := q.MoveOutlookMessage(ctx, store.MoveOutlookMessageParams{ID: id, FolderID: folder.ID, ChangedAt: remote.LastModifiedDateTime}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.message_changed", EntityType: "outlook_message", EntityID: id})
	})
}

func (s *Service) replyOutlook(ctx context.Context, p authz.Principal, id string, in api.OutlookReplyInput) error {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return apperr.Validation(apperr.FieldErrors{{Field: "text", Message: "enter a reply"}})
	}
	accepted := false
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		if row.IsDraft {
			return apperr.Validation(apperr.FieldErrors{{Field: "message", Message: "open the draft in Outlook to edit it"}})
		}
		link, err := q.OutlookLink(ctx, store.OutlookLinkParams{MailboxID: box.ID, Conversation: row.Conversation})
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		if link.CandidateID != nil {
			if _, err := s.profileAddress(ctx, tx, p, link.CandidateID, nil); err != nil {
				return err
			}
			if err := s.contactable(ctx, tx, *link.CandidateID); err != nil {
				return err
			}
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		operation, action := "reply", "outlook.replied"
		if in.Mode == api.OutlookReplyInputModeReplyAll {
			operation = "replyAll"
		}
		if in.SaveDraft {
			action = "outlook.reply_drafted"
			if operation == "reply" {
				operation = "createReply"
			} else {
				operation = "createReplyAll"
			}
		}
		if err := s.outlookRequest(ctx, token, http.MethodPost, s.outlookMessagePath(row)+"/"+operation, map[string]string{"comment": text}, nil); err != nil {
			return outlookFailure(err, true)
		}
		accepted = true
		return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "outlook_message", EntityID: id})
	})
	if accepted && err != nil {
		return outlookFailure(err, true)
	}
	return err
}

func (s *Service) linkOutlook(ctx context.Context, p authz.Principal, id string, in *api.OutlookConversationLink) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		if in == nil {
			if err := q.RemoveOutlookLink(ctx, store.RemoveOutlookLinkParams{MailboxID: box.ID, Conversation: row.Conversation}); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "outlook.conversation_unlinked", EntityType: "outlook_message", EntityID: id})
		}
		address, err := s.profileAddress(ctx, tx, p, in.CandidateId, in.ContactId)
		if err != nil {
			return err
		}
		match, err := q.OutlookThreadHasAddress(ctx, store.OutlookThreadHasAddressParams{MailboxID: box.ID, Conversation: row.Conversation, Address: strings.ToLower(strings.TrimSpace(deref(address)))})
		if err != nil {
			return err
		}
		if !match {
			return apperr.Validation(apperr.FieldErrors{{Field: "message", Message: "this conversation does not include the profile’s email address"}})
		}
		if err := q.SaveOutlookLink(ctx, store.SaveOutlookLinkParams{MailboxID: box.ID, Conversation: row.Conversation, CandidateID: in.CandidateId, ContactID: in.ContactId}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.conversation_linked", EntityType: "outlook_message", EntityID: id})
	})
}
