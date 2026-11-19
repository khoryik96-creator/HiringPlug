package emails

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const (
	outlookAttachmentLimit  = 20 << 20
	outlookAttachmentPage   = 50
	outlookAttachmentFields = "?$select=id,name,size,isInline"
)

var outlookAttachmentID = regexp.MustCompile(`^[A-Za-z0-9_+=/-]{1,1000}$`)

type outlookAttachment struct {
	ID, Name string
	Size     int64
	IsInline bool
	Type     string `json:"@odata.type"`
}

func (a outlookAttachment) file() bool { return a.Type == "#microsoft.graph.fileAttachment" }

func outlookAttachmentName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" || name == "/" {
		return "attachment"
	}
	// A long name is shortened before its extension, which says what the
	// file is (and whether it can be a CV).
	ext := path.Ext(name)
	if len([]rune(ext)) > 16 {
		ext = ""
	}
	return truncate(strings.TrimSuffix(name, ext), 240-len([]rune(ext))) + ext
}

// Authorize against the private local index and refresh credentials under
// the mailbox lock, then release the transaction before transferring bytes.
func (s *Service) outlookAttachmentAccess(ctx context.Context, p authz.Principal, id string) (target, token string, err error) {
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		token, err = s.mailboxAccess(ctx, box)
		if err != nil {
			return outlookFailure(err, false)
		}
		target = s.outlookMessagePath(row) + "/attachments"
		return nil
	})
	return target, token, err
}

func (s *Service) listOutlookAttachments(ctx context.Context, p authz.Principal, id string) (api.OutlookAttachmentList, error) {
	out := api.OutlookAttachmentList{Items: []api.OutlookAttachment{}}
	target, token, err := s.outlookAttachmentAccess(ctx, p, id)
	if err != nil {
		return out, err
	}
	var page outlookPage[outlookAttachment]
	if err := s.outlookRequest(ctx, token, http.MethodGet, target+outlookAttachmentFields+"&$top="+strconv.Itoa(outlookAttachmentPage), nil, &page); err != nil {
		return out, outlookFailure(err, false)
	}
	if page.Value == nil {
		return out, outlookFailure(&outlookGraphError{status: http.StatusBadGateway}, false)
	}
	out.More = page.Next != ""
	for _, attachment := range *page.Value {
		if attachment.IsInline {
			continue
		}
		if len(out.Items) == outlookAttachmentPage || !outlookAttachmentID.MatchString(attachment.ID) {
			out.More = true
			continue
		}
		downloadable := attachment.file() && attachment.Size >= 0 && attachment.Size <= outlookAttachmentLimit
		out.Items = append(out.Items, api.OutlookAttachment{
			Id:           base64.RawURLEncoding.EncodeToString([]byte(attachment.ID)),
			FileName:     outlookAttachmentName(attachment.Name),
			SizeBytes:    max(0, attachment.Size),
			Downloadable: downloadable,
			// Offered for a CV only when a candidate's files take it: never
			// fetched in full only to be refused.
			Savable: downloadable && s.deps.Documents != nil && attachment.Size <= s.deps.Documents.MaxUploadBytes(),
		})
	}
	return out, nil
}

func attachmentTooLarge() error {
	return apperr.New(api.ErrorCodeOUTLOOKATTACHMENTTOOLARGE, "This attachment is over 20 MB. Open the message in Outlook to download it.")
}

func attachmentFailure(err error) error {
	var graph *outlookGraphError
	if errors.As(err, &graph) && graph.status == http.StatusNotFound {
		return apperr.NotFound("Outlook attachment")
	}
	return outlookFailure(err, false)
}

func (s *Service) downloadOutlookAttachment(ctx context.Context, p authz.Principal, messageID, attachmentID string) ([]byte, string, error) {
	id, err := base64.RawURLEncoding.Strict().DecodeString(attachmentID)
	if err != nil || !outlookAttachmentID.Match(id) {
		return nil, "", apperr.NotFound("Outlook attachment")
	}
	target, token, err := s.outlookAttachmentAccess(ctx, p, messageID)
	if err != nil {
		return nil, "", err
	}
	target += "/" + url.PathEscape(string(id))
	var attachment outlookAttachment
	if err := s.outlookRequest(ctx, token, http.MethodGet, target+outlookAttachmentFields, nil, &attachment); err != nil {
		return nil, "", attachmentFailure(err)
	}
	if !attachment.file() || attachment.IsInline || attachment.ID != string(id) || attachment.Size < 0 {
		return nil, "", apperr.NotFound("Outlook file attachment")
	}
	if attachment.Size > outlookAttachmentLimit {
		return nil, "", attachmentTooLarge()
	}
	response, err := s.openOutlookResponse(ctx, token, http.MethodGet, target+"/$value", nil)
	if err != nil {
		return nil, "", attachmentFailure(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, "", outlookFailure(&outlookGraphError{status: http.StatusBadGateway}, false)
	}
	if response.ContentLength > outlookAttachmentLimit {
		return nil, "", attachmentTooLarge()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, outlookAttachmentLimit+1))
	if err != nil {
		return nil, "", outlookFailure(err, false)
	}
	if len(data) > outlookAttachmentLimit {
		return nil, "", attachmentTooLarge()
	}
	return data, outlookAttachmentName(attachment.Name), nil
}

func (h *HTTP) ListOutlookAttachments(ctx context.Context, r api.ListOutlookAttachmentsRequestObject) (api.ListOutlookAttachmentsResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.listOutlookAttachments(ctx, p, r.MessageId)
	if err != nil {
		return nil, err
	}
	return api.ListOutlookAttachments200JSONResponse{
		Body: out, Headers: api.ListOutlookAttachments200ResponseHeaders{CacheControl: "no-store"},
	}, nil
}

func (h *HTTP) DownloadOutlookAttachment(ctx context.Context, r api.DownloadOutlookAttachmentRequestObject) (api.DownloadOutlookAttachmentResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	data, name, err := h.svc.downloadOutlookAttachment(ctx, p, r.MessageId, r.AttachmentId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return api.DownloadOutlookAttachment200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(data), ContentLength: int64(len(data)),
		Headers: api.DownloadOutlookAttachment200ResponseHeaders{ContentDisposition: disposition, CacheControl: "no-store"},
	}, nil
}
