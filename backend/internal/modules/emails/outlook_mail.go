package emails

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func outlookDisconnected() error {
	return apperr.New(api.ErrorCodeOUTLOOKNOTCONNECTED, "Connect a Microsoft mailbox in Email connection and enable Outlook sync.")
}

func (s *Service) requestOutlook(ctx context.Context, p authz.Principal, disable bool) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		if disable {
			if err := q.CancelOutlookSendsForMailbox(ctx); err != nil {
				return err
			}
			if err := q.RemoveOutlookSync(ctx); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "outlook.sync_disabled", EntityType: "user", EntityID: p.UserID})
		}
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) {
			return outlookDisconnected()
		}
		if err != nil {
			return err
		}
		if box.Provider != ProviderMicrosoft {
			return outlookDisconnected()
		}
		state, err := q.RequestOutlookSync(ctx, store.RequestOutlookSyncParams{MailboxID: box.ID, Generation: randomMailboxToken()})
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.NudgeOutlookFolders(ctx, box.ID); err != nil {
			return err
		}
		if err := enqueueOutlook(ctx, tx, api.OutlookMailSyncJob{MailboxId: box.ID, UserId: p.UserID, Generation: state.Generation}, p.TenantID, time.Time{}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "outlook.sync_requested", EntityType: "user", EntityID: p.UserID})
	})
}

func (s *Service) profileAddress(ctx context.Context, tx pgx.Tx, p authz.Principal, candidate, contact *string) (*string, error) {
	if candidate != nil && contact == nil {
		if !p.Can(api.PermissionCandidatesRead) {
			return nil, apperr.NotFound("candidate")
		}
		people, err := s.deps.Candidates.Briefs(ctx, tx, []string{*candidate})
		if err != nil {
			return nil, err
		}
		person, found := people[*candidate]
		if !found {
			return nil, apperr.NotFound("candidate")
		}
		return person.Email, nil
	}
	if contact != nil && candidate == nil {
		if !p.Can(api.PermissionContactsRead) {
			return nil, apperr.NotFound("contact")
		}
		person, err := s.deps.Contacts.Contact(ctx, tx, *contact)
		return person.Email, err
	}
	return nil, apperr.Validation(apperr.FieldErrors{{Field: "candidateId", Message: "choose one candidate or contact"}})
}

func (s *Service) listOutlook(ctx context.Context, p authz.Principal, params api.ListOutlookMailParams) (api.OutlookMailPage, error) {
	out := api.OutlookMailPage{
		Folders: []api.OutlookMailFolder{}, Items: []api.OutlookMessage{},
		Error: apiconv.OrNull[string](nil), LastSyncAt: apiconv.OrNull[time.Time](nil), NextOffset: apiconv.OrNull[int](nil),
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if params.CandidateId != nil || params.ContactId != nil {
			if _, err := s.profileAddress(ctx, tx, p, params.CandidateId, params.ContactId); err != nil {
				return err
			}
		}
		q := store.New(tx)
		box, err := q.MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if box.Provider != ProviderMicrosoft {
			return nil
		}
		out.Connected = true
		state, err := q.OutlookSync(ctx, box.ID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		out.Enabled = true
		out.Error = apiconv.OrNull(state.SyncError)
		out.LastSyncAt = apiconv.OrNull(state.LastSyncAt)
		out.Syncing = state.LastSyncAt == nil && state.SyncError == nil
		folders, err := q.OutlookFolders(ctx, box.ID)
		if err != nil {
			return err
		}
		localFolderIds := make(map[string]string, len(folders))
		for _, folder := range folders {
			localFolderIds[folder.RemoteID] = folder.ID
		}
		counts, err := q.OutlookUnreadCounts(ctx, box.ID)
		if err != nil {
			return err
		}
		unread := make(map[string]int, len(counts))
		for _, c := range counts {
			unread[c.FolderID] = int(c.Unread)
		}
		for _, folder := range folders {
			parent := orNil(localFolderIds[deref(folder.ParentID)])
			out.Folders = append(out.Folders, api.OutlookMailFolder{Id: folder.ID, Name: folder.Name, Kind: folder.Kind, ParentId: apiconv.OrNull(parent), Unread: unread[folder.ID]})
			out.Syncing = out.Syncing || (!folder.Complete && state.SyncError == nil)
		}
		var offset int
		if params.Offset != nil {
			offset = *params.Offset
		}
		if offset < 0 || offset > 100000 {
			return apperr.Validation(apperr.FieldErrors{{Field: "offset", Message: "select a smaller offset"}})
		}
		folderID := deref(params.Folder)
		if folderID == "" && params.CandidateId == nil && params.ContactId == nil && params.Conversation == nil && deref(params.Search) == "" {
			for _, folder := range folders {
				if folder.Kind == "inbox" {
					folderID = folder.ID
					break
				}
			}
		}
		rows, err := q.ListOutlookMessages(ctx, store.ListOutlookMessagesParams{MailboxID: box.ID, Folder: folderID, Thread: deref(params.Conversation), Candidate: deref(params.CandidateId), Contact: deref(params.ContactId), Search: deref(params.Search), Skip: int32(offset)}) //nolint:gosec // range checked above
		if err != nil {
			return err
		}
		if len(rows) > 50 && offset < 100000 {
			next := offset + 50
			out.NextOffset = apiconv.OrNull(&next)
		}
		for _, row := range rows[:min(50, len(rows))] {
			out.Items = append(out.Items, api.OutlookMessage{Id: row.ID, FolderId: row.FolderID, Conversation: row.Conversation, Subject: row.Subject, Sender: row.Sender, Participants: row.Participants, ReceivedAt: row.ReceivedAt, IsRead: row.IsRead, IsDraft: row.IsDraft, HasAttachments: row.HasAttachments, WebUrl: apiconv.OrNull(row.WebUrl),
				SenderName: row.SenderName, Flagged: row.Flagged})
		}
		return nil
	})
	return out, err
}

func loadOutlookMessage(ctx context.Context, q *store.Queries, user, id string) (store.EmailMailbox, store.EmailOutlookMessage, error) {
	box, err := q.MailboxForDelivery(ctx, user)
	if database.IsNoRows(err) {
		return box, store.EmailOutlookMessage{}, outlookDisconnected()
	}
	if err != nil {
		return box, store.EmailOutlookMessage{}, err
	}
	if box.Provider != ProviderMicrosoft {
		return box, store.EmailOutlookMessage{}, outlookDisconnected()
	}
	row, err := q.OutlookMessage(ctx, store.OutlookMessageParams{MailboxID: box.ID, ID: id})
	if database.IsNoRows(err) {
		err = apperr.NotFound("Outlook message")
	}
	return box, row, err
}
