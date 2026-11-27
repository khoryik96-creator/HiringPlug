package emails

// Download all: a message's file attachments in one .zip, as Outlook's
// "Download all" does.

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// outlookZipLimit is the most the files of one message may weigh together.
const outlookZipLimit = 60 << 20

// zipNames gives each file a name of its own in the zip: a second "CV.pdf"
// is "CV (2).pdf" (names compared ignoring case, as Windows does).
type zipNames map[string]bool

func (n zipNames) take(name string) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	out := name
	for i := 2; n[strings.ToLower(out)]; i++ {
		out = stem + " (" + strconv.Itoa(i) + ")" + ext
	}
	n[strings.ToLower(out)] = true
	return out
}

func (s *Service) downloadAllOutlookAttachments(ctx context.Context, p authz.Principal, messageID string) ([]byte, error) {
	target, token, err := s.outlookAttachmentAccess(ctx, p, messageID)
	if err != nil {
		return nil, err
	}
	var page outlookPage[outlookAttachment]
	if err := s.outlookRequest(ctx, token, http.MethodGet, target+outlookAttachmentFields+"&$top="+strconv.Itoa(outlookAttachmentPage), nil, &page); err != nil {
		return nil, outlookFailure(err, false)
	}
	if page.Value == nil {
		return nil, outlookFailure(&outlookGraphError{status: http.StatusBadGateway}, false)
	}
	var files []outlookAttachment
	var leftOut []string
	var total int64
	for _, a := range *page.Value {
		if a.IsInline || !a.file() || !outlookGraphID.MatchString(a.ID) {
			continue
		}
		if a.Size < 0 || a.Size > outlookAttachmentLimit {
			leftOut = append(leftOut, outlookAttachmentName(a.Name)+": over 20 MB; download it from Outlook")
			continue
		}
		files = append(files, a)
		total += a.Size
	}
	if len(files) == 0 {
		return nil, apperr.NotFound("Outlook file attachment")
	}
	if total > outlookZipLimit {
		return nil, apperr.New(api.ErrorCodeOUTLOOKATTACHMENTTOOLARGE,
			"These attachments are over 60 MB together. Download them one by one, or from Outlook.")
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	names := zipNames{}
	now := time.Now()
	for _, a := range files {
		data, err := s.attachmentBytes(ctx, token, target+"/"+url.PathEscape(a.ID))
		if err != nil {
			return nil, err
		}
		w, err := archive.CreateHeader(&zip.FileHeader{Name: names.take(outlookAttachmentName(a.Name)), Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}
	if page.Next != "" {
		leftOut = append(leftOut, "more attachments than shown here: open the message in Outlook for the rest")
	}
	if len(leftOut) > 0 {
		w, err := archive.CreateHeader(&zip.FileHeader{Name: names.take("Left out.txt"), Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(w, "Not in this zip:\r\n- %s\r\n", strings.Join(leftOut, "\r\n- ")); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DownloadAllOutlookAttachments handles GET /v1/outlook/messages/{messageId}/attachments/zip.
func (h *HTTP) DownloadAllOutlookAttachments(ctx context.Context, r api.DownloadAllOutlookAttachmentsRequestObject) (api.DownloadAllOutlookAttachmentsResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	data, err := h.svc.downloadAllOutlookAttachments(ctx, p, r.MessageId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": "Attachments.zip"})
	return api.DownloadAllOutlookAttachments200ApplicationzipResponse{
		Body: bytes.NewReader(data), ContentLength: int64(len(data)),
		Headers: api.DownloadAllOutlookAttachments200ResponseHeaders{ContentDisposition: disposition, CacheControl: "no-store"},
	}, nil
}
