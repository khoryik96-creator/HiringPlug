package emails

import (
	"context"
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

func (h *HTTP) AddOutlookDraftAttachment(ctx context.Context, r api.AddOutlookDraftAttachmentRequestObject) (api.AddOutlookDraftAttachmentResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	upload, err := readForm(r.Body, maxFileBytes, "A file can be at most 10 MB.")
	if err != nil {
		return nil, err
	}
	name := outlookAttachmentName(upload.fileName)
	contentType := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if contentType == "" {
		contentType = http.DetectContentType(upload.data)
	}
	out, err := h.svc.changeDraftAttachment(ctx, p, r.MessageId, r.Params.Version, &mail.Part{
		Name: name, ContentType: contentType, Data: upload.data,
	}, "")
	return api.AddOutlookDraftAttachment200JSONResponse(out), err
}

func (h *HTTP) RemoveOutlookDraftAttachment(ctx context.Context, r api.RemoveOutlookDraftAttachmentRequestObject) (api.RemoveOutlookDraftAttachmentResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	attachment, err := base64.RawURLEncoding.Strict().DecodeString(r.AttachmentId)
	if err != nil || !outlookGraphID.Match(attachment) {
		return nil, apperr.NotFound("Outlook attachment")
	}
	out, err := h.svc.changeDraftAttachment(ctx, p, r.MessageId, r.Params.Version, nil, string(attachment))
	return api.RemoveOutlookDraftAttachment200JSONResponse(out), err
}

// Attachment changes require review before a previously scheduled draft can
// go out. The worker and editor share the same per-mailbox advisory lock.
func (s *Service) changeDraftAttachment(ctx context.Context, p authz.Principal, id, version string, add *mail.Part, remove string) (api.OutlookDraft, error) {
	// Keep time to commit the schedule hold if a write is interrupted or its
	// response is lost, even when the browser has already disconnected.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 55*time.Second)
	defer cancel()
	remoteCtx, remoteCancel := context.WithTimeout(ctx, 45*time.Second)
	defer remoteCancel()
	var out api.OutlookDraft
	var providerErr error
	providerChanged := false
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		draft, err := s.openOutlookDraft(remoteCtx, q, p.UserID, id)
		if err != nil {
			return err
		}
		if draftVersion(draft.remote) != version {
			return draftChanged()
		}
		active, err := activeOutlookSend(ctx, q, draft.box.ID, draft.row.RemoteID)
		if err != nil {
			return err
		}
		if active != nil && active.Status != "scheduled" && active.Status != "needs_review" {
			return sendInProgress()
		}
		target := s.outlookMessagePath(draft.row)
		if remove != "" {
			if err := s.checkDraftAttachmentRemoval(remoteCtx, draft.token, target, remove); err != nil {
				return err
			}
		}
		if active != nil && active.Status == "scheduled" {
			message := "Review the draft attachments and update the schedule."
			if err := finishOutlookSend(ctx, tx, *active, "needs_review", message); err != nil {
				return err
			}
			active.Status, active.Error = "needs_review", &message
		}
		auditAction := "outlook.draft_attachment_added"
		if add != nil {
			err = s.putMicrosoftAttachment(remoteCtx, s.endpoint(ProviderMicrosoft), draft.token, target, *add, true)
		} else {
			auditAction = "outlook.draft_attachment_removed"
			err = s.outlookRequest(remoteCtx, draft.token, http.MethodDelete, target+"/attachments/"+url.PathEscape(remove), nil, nil)
		}
		if err != nil {
			// Commit the hold even when Outlook could have applied the change
			// before its response was lost.
			providerErr = outlookDraftWriteFailure(err)
			return nil
		}
		providerChanged = true
		remote, err := s.readRemoteDraft(remoteCtx, draft.token, draft.row.RemoteID)
		if err != nil || !remote.IsDraft {
			providerErr = outlookFailure(err, true)
			return nil
		}
		row, err := saveOutlookDraftIndex(ctx, q, draft.box.ID, draft.row.FolderID, remote)
		if err != nil {
			return err
		}
		out = draftView(row, remote, active)
		return audit.Record(ctx, tx, audit.Entry{Action: auditAction, EntityType: "outlook_message", EntityID: row.ID})
	})
	if err != nil {
		if providerChanged {
			return out, outlookFailure(err, true)
		}
		return out, err
	}
	return out, providerErr
}

func (s *Service) checkDraftAttachmentRemoval(ctx context.Context, token, target, id string) error {
	var file outlookAttachment
	if err := s.outlookRequest(ctx, token, http.MethodGet, target+"/attachments/"+url.PathEscape(id)+outlookAttachmentFields, nil, &file); err != nil {
		return attachmentFailure(err)
	}
	if file.IsInline || file.ID != id {
		return apperr.NotFound("Outlook attachment")
	}
	return nil
}
