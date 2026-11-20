package emails

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) forwardOutlook(ctx context.Context, p authz.Principal, id string, in api.OutlookForwardInput) (api.OutlookDraft, error) {
	// Outlook copies the attachments and appends the quoted body to the comment.
	body := map[string]any{"message": draftPatch(api.OutlookDraftInput{To: &in.To})}
	if note := strings.TrimSpace(deref(in.Note)); note != "" {
		body["comment"] = note
	}
	return s.createOutlookThreadDraft(ctx, p, id, "createForward", body)
}

func (s *Service) createOutlookThreadDraft(ctx context.Context, p authz.Principal, id, operation string, body any) (api.OutlookDraft, error) {
	var draft api.OutlookDraft
	created := false
	reply := operation != "createForward"
	auditAction := "outlook.forward_drafted"
	if reply {
		auditAction = "outlook.reply_drafted"
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, source, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		if source.IsDraft {
			return apperr.Validation(apperr.FieldErrors{{Field: "message", Message: "choose a received or sent message"}})
		}
		if reply {
			if err := s.checkOutlookReplyCandidate(ctx, tx, p, box.ID, source.Conversation); err != nil {
				return err
			}
		}
		folder, err := outlookDraftFolderForCreate(ctx, q, box.ID)
		if err != nil {
			return err
		}
		token, err := s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		var remote outlookRemoteMessage
		if err := s.outlookRequest(ctx, token, http.MethodPost, s.outlookMessagePath(source)+"/"+operation, body, &remote); err != nil {
			return outlookFailure(err, true)
		}
		created = true
		if remote.ID == source.RemoteID || !outlookGraphID.MatchString(remote.ID) || reply && remote.ConversationID != source.Conversation {
			return outlookFailure(nil, true)
		}
		draft, err = indexNewOutlookDraft(ctx, q, box.ID, folder.ID, remote)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: auditAction, EntityType: "outlook_message", EntityID: draft.MessageId})
	})
	if err != nil && created {
		return draft, outlookFailure(err, true)
	}
	return draft, err
}
