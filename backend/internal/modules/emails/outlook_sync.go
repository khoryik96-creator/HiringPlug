package emails

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func enqueueOutlook(ctx context.Context, tx pgx.Tx, payload api.OutlookMailSyncJob, tenant string, at time.Time) error {
	_, err := jobqueue.EnqueueAt(ctx, tx, tenant, api.JobKindEmailOutlookSync, payload, at)
	return err
}

func (s *Service) syncOutlook(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.OutlookMailSyncJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	p, active, err := s.deps.Users.PrincipalFor(ctx, tx, payload.UserId)
	if err != nil || !active || !p.Can(api.PermissionEmailsSend) {
		return err
	}

	// Keep the mailbox checkpoint independent of queue acknowledgement. A
	// generation prevents a stale job from polling again after it has advanced.
	conn, err := pgx.ConnectConfig(ctx, s.pool.Config().ConnConfig.Copy())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	return database.InTx(ctx, conn, p, func(t pgx.Tx) error {
		q := store.New(t)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if box.Provider != ProviderMicrosoft || box.ID != payload.MailboxId {
			return nil
		}
		state, err := q.OutlookSync(ctx, box.ID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if state.Generation != payload.Generation {
			return nil
		}
		work, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		token, accessErr := s.mailboxAccess(work, box)
		changed := false
		var graph *outlookGraphError
		next := time.Now().Add(time.Minute)
		var last *time.Time
		var failure *string
		if accessErr != nil {
			message := "Could not access this mailbox. Check the Microsoft connection."
			failure = &message
			next = time.Now().Add(5 * time.Minute)
		} else {
			changed, err = s.pullOutlook(work, q, state, token)
			if err == nil && changed {
				now := time.Now()
				last = &now
				next = now.Add(20 * time.Second)
			} else if errors.As(err, &graph) {
				message := "Outlook could not sync. HiringPlug will retry."
				if graph.status == 401 || graph.status == 403 {
					message = "Outlook denied mailbox access. Reconnect in Email connection."
				}
				failure = &message
				next = time.Now().Add(2 * time.Minute)
				if graph.retry.After(next) {
					next = graph.retry
				}
			} else if err != nil {
				return err
			}
		}
		generation := randomMailboxToken()
		if err := q.MarkOutlookSync(ctx, store.MarkOutlookSyncParams{MailboxID: box.ID, Generation: generation, LastSyncAt: last, SyncError: failure}); err != nil {
			return err
		}
		payload.Generation = generation
		return enqueueOutlook(ctx, t, payload, p.TenantID, next)
	})
}

func (s *Service) pullOutlook(ctx context.Context, q *store.Queries, state store.EmailOutlookSync, token string) (bool, error) {
	if state.FoldersAt == nil || time.Since(*state.FoldersAt) > 30*time.Minute {
		folders, kinds, err := s.outlookFolders(ctx, token)
		if err != nil {
			return false, err
		}
		remoteIDs := make([]string, 0, len(folders))
		for id, f := range folders {
			remoteIDs = append(remoteIDs, id)
			if err := q.SaveOutlookFolder(ctx, store.SaveOutlookFolderParams{MailboxID: state.MailboxID, RemoteID: id, Name: truncate(f.DisplayName, 200), Kind: kinds[id], ParentID: orNil(f.ParentFolderID)}); err != nil {
				return false, err
			}
		}
		if err := q.RemoveMissingOutlookFolders(ctx, store.RemoveMissingOutlookFoldersParams{MailboxID: state.MailboxID, RemoteIds: remoteIDs}); err != nil {
			return false, err
		}
		if err := q.MarkOutlookFolders(ctx, state.MailboxID); err != nil {
			return false, err
		}
	}
	due, err := q.OutlookDueFolders(ctx, state.MailboxID)
	if err != nil {
		return false, err
	}
	// Collect pages before making changes so one provider failure cannot mark
	// a partially handled page as seen.
	for _, folder := range due {
		if err := s.pullOutlookFolder(ctx, q, token, folder); err != nil {
			return false, err
		}
	}
	return len(due) > 0, nil
}

func (s *Service) pullOutlookFolder(ctx context.Context, q *store.Queries, token string, folder store.EmailOutlookFolder) error {
	path := s.outlookRoot() + "/mailFolders/" + url.PathEscape(folder.RemoteID) + "/messages/delta"
	target := path
	if folder.CursorUrl != nil {
		target = *folder.CursorUrl
		if !outlookContinuation(path, target) {
			return &outlookGraphError{status: 502}
		}
	} else {
		fields := "id,parentFolderId,conversationId,subject,from,sender,toRecipients,ccRecipients,replyTo,receivedDateTime,createdDateTime,lastModifiedDateTime,isRead,isDraft,hasAttachments,webLink"
		query := url.Values{"$select": {fields}}
		if folder.Kind != "drafts" {
			query.Set("$filter", "receivedDateTime ge "+time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339))
		}
		target += "?" + query.Encode()
	}
	var page outlookPage[outlookRemoteMessage]
	err := s.outlookRequest(ctx, token, http.MethodGet, target, nil, &page)
	var graph *outlookGraphError
	if errors.As(err, &graph) && graph.status == 410 && folder.CursorUrl != nil {
		if err := q.ClearOutlookFolder(ctx, folder.ID); err != nil {
			return err
		}
		return q.SaveOutlookCursor(ctx, store.SaveOutlookCursorParams{ID: folder.ID})
	}
	if err != nil {
		return err
	}
	cursor, complete := page.Next, false
	if cursor == "" {
		cursor, complete = page.Delta, true
	}
	if page.Value == nil || len(cursor) > 16384 || !outlookContinuation(path, cursor) || (!complete && cursor == target) {
		return &outlookGraphError{status: 502}
	}
	for _, item := range *page.Value {
		if item.ID == "" || len(item.ID) > 1024 {
			return &outlookGraphError{status: 502}
		}
		if len(item.Removed) > 0 && string(item.Removed) != "null" {
			if err := q.RemoveOutlookMessage(ctx, store.RemoveOutlookMessageParams{FolderID: folder.ID, RemoteID: item.ID}); err != nil {
				return err
			}
			continue
		}
		if item.ParentFolderID != "" && item.ParentFolderID != folder.RemoteID {
			continue
		}
		if item.ReceivedDateTime.IsZero() {
			item.ReceivedDateTime = item.CreatedDateTime
		}
		if item.ReceivedDateTime.IsZero() || item.LastModifiedDateTime.IsZero() {
			return &outlookGraphError{status: 502}
		}
		conversation := item.ConversationID
		if conversation == "" {
			conversation = item.ID
		}
		if len(conversation) > 1024 {
			return &outlookGraphError{status: 502}
		}
		if err := q.SaveOutlookMessage(ctx, store.SaveOutlookMessageParams{MailboxID: folder.MailboxID, FolderID: folder.ID, RemoteID: item.ID, Conversation: conversation, Subject: truncate(item.Subject, 300), Sender: truncate(item.sender(), 254), Participants: outlookParticipants(item), ReceivedAt: item.ReceivedDateTime, ChangedAt: item.LastModifiedDateTime, IsRead: item.IsRead, IsDraft: item.IsDraft, HasAttachments: item.HasAttachments, WebUrl: outlookWebLink(item.WebLink)}); err != nil {
			return err
		}
	}
	now := time.Now()
	return q.SaveOutlookCursor(ctx, store.SaveOutlookCursorParams{ID: folder.ID, CursorUrl: &cursor, Complete: complete, CheckedAt: &now})
}

func outlookParticipants(item outlookRemoteMessage) []string {
	values := []string{}
	seen := map[string]bool{}
	add := func(address string) {
		address = strings.ToLower(strings.TrimSpace(address))
		if address != "" && len(address) <= 254 && !seen[address] {
			seen[address] = true
			values = append(values, address)
		}
	}
	add(item.sender())
	for _, list := range [][]outlookRecipient{item.ToRecipients, item.CcRecipients, item.ReplyTo} {
		for _, recipient := range list {
			add(recipient.EmailAddress.Address)
		}
	}
	return values
}

func outlookWebLink(raw string) *string {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "https" || target.User != nil {
		return nil
	}
	switch strings.ToLower(target.Host) {
	case "outlook.office.com", "outlook.office365.com", "outlook.live.com", "outlook.cloud.microsoft":
		return &raw
	}
	return nil
}
