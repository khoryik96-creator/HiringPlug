package emails

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Large CVs use Graph's draft/upload-session flow. Direct sendMail requests
// cannot carry the ATS's full 20 MB attachment allowance.
func (s *Service) sendLargeMicrosoftMail(ctx context.Context, endpoint OAuthProvider, token string, msg mail.Message) error {
	root := strings.TrimSuffix(endpoint.SendURL, "/sendMail") + "/messages"
	recipients := func(addresses []string) []map[string]any {
		rows := make([]map[string]any, len(addresses))
		for i, address := range addresses {
			rows[i] = map[string]any{"emailAddress": map[string]string{"address": address}}
		}
		return rows
	}
	content, kind := msg.HTML, "HTML"
	if content == "" {
		content, kind = msg.Body, "Text"
	}
	draft := map[string]any{"subject": msg.Subject, "body": map[string]string{"contentType": kind, "content": content}, "toRecipients": recipients(msg.To), "ccRecipients": recipients(msg.Cc), "bccRecipients": recipients(msg.Bcc)}
	if msg.ReplyTo != "" {
		draft["replyTo"] = recipients([]string{msg.ReplyTo})
	}
	body, _ := json.Marshal(draft)
	raw, err := s.providerRequest(ctx, "POST", root, "application/json", token, body)
	if err != nil {
		return err
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &created) != nil || created.ID == "" {
		return errors.New("mail provider did not create a draft")
	}
	target := root + "/" + url.PathEscape(created.ID)
	sent := false
	defer func() {
		if !sent {
			_, _ = s.providerRequest(ctx, "DELETE", target, "application/json", token, nil)
		}
	}()
	parts := append(append([]mail.Part{}, msg.Attach...), msg.Inline...)
	if msg.Calendar != "" {
		parts = append(parts, mail.Part{Name: "invite.ics", ContentType: "text/calendar", Data: []byte(msg.Calendar)})
	}
	for _, part := range parts {
		if len(part.Data) < 3<<20 {
			attachment := map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "name": part.Name, "contentType": part.ContentType, "contentBytes": base64.StdEncoding.EncodeToString(part.Data)}
			if part.ContentID != "" {
				attachment["isInline"] = true
				attachment["contentId"] = part.ContentID
			}
			body, _ := json.Marshal(attachment)
			if _, err = s.providerRequest(ctx, "POST", target+"/attachments", "application/json", token, body); err != nil {
				return err
			}
			continue
		}
		item := map[string]any{"attachmentType": "file", "name": part.Name, "size": len(part.Data), "contentType": part.ContentType}
		if part.ContentID != "" {
			item["isInline"] = true
			item["contentId"] = part.ContentID
		}
		body, _ := json.Marshal(map[string]any{"AttachmentItem": item})
		raw, err := s.providerRequest(ctx, "POST", target+"/attachments/createUploadSession", "application/json", token, body)
		if err != nil {
			return err
		}
		var session struct {
			UploadURL string `json:"uploadUrl"`
		}
		if json.Unmarshal(raw, &session) != nil || !s.trustedUploadURL(session.UploadURL, endpoint.SendURL) {
			return errors.New("mail provider returned an invalid attachment upload address")
		}
		if err = s.uploadMicrosoftPart(ctx, session.UploadURL, part.Data); err != nil {
			return err
		}
	}
	if _, err = s.providerRequest(ctx, "POST", target+"/send", "application/json", token, nil); err != nil {
		return err
	}
	sent = true
	return nil
}
func (s *Service) trustedUploadURL(address, sendURL string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "https" && (host == "outlook.office.com" || host == "outlook.office365.com" || strings.HasSuffix(host, ".outlook.office365.com")) {
		return true
	}
	// The only exception is an in-process provider stand-in, never browser input.
	origin, err := url.Parse(sendURL)
	return s.connections.AllowPrivate && err == nil && origin.Host == parsed.Host && origin.Scheme == parsed.Scheme
}
func (s *Service) uploadMicrosoftPart(ctx context.Context, address string, data []byte) error {
	const chunk = 10 * 320 * 1024
	for start := 0; start < len(data); start += chunk {
		end := min(start+chunk, len(data))
		req, err := http.NewRequestWithContext(ctx, "PUT", address, strings.NewReader(string(data[start:end])))
		if err != nil {
			return errors.New("attachment upload could not be created")
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, len(data)))
		// uploadUrl is already authorized; a bearer token must never be sent there.
		response, err := s.connections.HTTPClient.Do(req)
		if err != nil {
			return errors.New("attachment upload could not reach the mail provider")
		}
		_ = response.Body.Close()
		if response.StatusCode/100 != 2 {
			return fmt.Errorf("attachment upload was refused (HTTP %d)", response.StatusCode)
		}
	}
	return nil
}
