package emails

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const outlookReadingFields = "?$select=body,from,sender,toRecipients,ccRecipients,sentDateTime,importance"

// outlookBody is a message as the reading pane shows it: who it is from and
// to, its formatting made safe, and its plain text. Credentials are checked
// under the mailbox lock; Outlook is read after it is released.
func (s *Service) outlookBody(ctx context.Context, p authz.Principal, id string) (api.OutlookMessageBody, error) {
	out := api.OutlookMessageBody{To: []api.OutlookPerson{}, Cc: []api.OutlookPerson{}, Importance: api.OutlookMessageBodyImportanceNormal,
		Html: apiconv.OrNull[string](nil), From: apiconv.OrNull[api.OutlookPerson](nil), SentAt: apiconv.OrNull[time.Time](nil)}
	var path, token string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, row, err := loadOutlookMessage(ctx, q, p.UserID, id)
		if err != nil {
			return err
		}
		if token, err = s.mailboxAccess(ctx, box); err != nil {
			return outlookFailure(err, false)
		}
		path = s.outlookMessagePath(row)
		return nil
	})
	if err != nil {
		return out, err
	}
	var remote outlookRemoteMessage
	if err := s.outlookRequest(ctx, token, http.MethodGet, path+outlookReadingFields, nil, &remote); err != nil {
		return out, outlookFailure(err, false)
	}
	from := remote.From
	if from.EmailAddress.Address == "" {
		from = remote.Sender
	}
	if from.EmailAddress.Address != "" {
		sender := person(from)
		out.From = apiconv.OrNull(&sender)
	}
	for _, r := range remote.ToRecipients {
		out.To = append(out.To, person(r))
	}
	for _, r := range remote.CcRecipients {
		out.Cc = append(out.Cc, person(r))
	}
	if !remote.SentDateTime.IsZero() {
		out.SentAt = apiconv.OrNull(&remote.SentDateTime)
	}
	switch strings.ToLower(remote.Importance) {
	case "high":
		out.Importance = api.OutlookMessageBodyImportanceHigh
	case "low":
		out.Importance = api.OutlookMessageBodyImportanceLow
	}
	body := remote.Body.Content
	if strings.EqualFold(remote.Body.ContentType, "html") {
		if len(body) <= readingHTMLLimit {
			shown, blocked := readableHTML(body, s.inlineImages(ctx, token, path, contentIDs(body)))
			if shown != "" {
				out.Html = apiconv.OrNull(&shown)
				out.BlockedImages = blocked
			}
		}
		body = TextFromHTML(SanitizeHTML(body))
	}
	out.Text = truncate(body, 40000)
	out.Truncated = out.Text != body && out.Html.IsNull()
	return out, nil
}

// pictureLike: a file that may be a picture, by its type or its name.
func pictureLike(contentType, name string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return true
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		return true
	}
	return false
}

func person(r outlookRecipient) api.OutlookPerson {
	return api.OutlookPerson{Name: truncate(strings.TrimSpace(r.EmailAddress.Name), 300), Address: truncate(strings.TrimSpace(r.EmailAddress.Address), 320)}
}

// inlineImages are the pictures sent with a message that its HTML shows
// (content id -> data: URI), within limits; one that cannot be had is left
// out (the message shows its alt text).
func (s *Service) inlineImages(ctx context.Context, token, path string, wanted map[string]bool) map[string]string {
	out := map[string]string{}
	if len(wanted) == 0 {
		return out
	}
	var page outlookPage[outlookAttachment]
	if s.outlookRequest(ctx, token, http.MethodGet, path+"/attachments?$select=id,name,size,isInline,contentType&$top=50", nil, &page) != nil || page.Value == nil {
		return out
	}
	total, tried := 0, 0
	for _, a := range *page.Value {
		// Some mail apps send a signature picture without marking it inline,
		// or as a plain file type: any picture-like file may be the one shown.
		if !a.file() || a.Size <= 0 || a.Size > inlineImageLimit || len(out) >= len(wanted) || tried >= inlineImagesAtMost || !outlookGraphID.MatchString(a.ID) ||
			(!a.IsInline && !pictureLike(a.ContentType, a.Name)) {
			continue
		}
		tried++
		var file struct {
			ContentID, ContentType, ContentBytes string
		}
		if s.outlookRequest(ctx, token, http.MethodGet, path+"/attachments/"+url.PathEscape(a.ID), nil, &file) != nil {
			continue
		}
		id := contentID(file.ContentID)
		if !wanted[id] {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(file.ContentBytes)
		if err != nil || len(data) > inlineImageLimit || total+len(data) > inlineImagesLimit {
			continue
		}
		// The type the bytes are, whatever the file says it is.
		kind := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
		if !imageTypes[kind] {
			continue
		}
		total += len(data)
		out[id] = "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	return out
}
