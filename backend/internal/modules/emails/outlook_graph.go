package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type outlookGraphError struct {
	status int
	retry  time.Time
}

var outlookGraphID = regexp.MustCompile(`^[A-Za-z0-9_+=/-]{1,1000}$`)

func (e *outlookGraphError) Error() string {
	return fmt.Sprintf("outlook mail request failed (HTTP %d)", e.status)
}

func (s *Service) outlookRoot() string {
	return strings.TrimSuffix(s.endpoint(ProviderMicrosoft).SendURL, "/sendMail")
}

func (s *Service) outlookRequest(ctx context.Context, token, method, target string, body any, out any) error {
	return s.outlookConditional(ctx, token, method, target, body, out, "")
}

func (s *Service) outlookConditional(ctx context.Context, token, method, target string, body any, out any, etag string) error {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	response, err := s.openOutlookResponse(ctx, token, method, target, bytes.NewReader(data), etag)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if out == nil {
		return nil
	}
	const limit = 6 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(raw) > limit || json.Unmarshal(raw, out) != nil {
		return &outlookGraphError{status: 502}
	}
	return nil
}

func (s *Service) openOutlookResponse(ctx context.Context, token, method, target string, body io.Reader, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, &outlookGraphError{}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", `IdType="ImmutableId", outlook.body-content-type="text", odata.maxpagesize=50`)
	client := *s.connections.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, &outlookGraphError{}
	}
	if response.StatusCode/100 != 2 {
		_ = response.Body.Close()
		retry, _ := http.ParseTime(response.Header.Get("Retry-After"))
		if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 && seconds <= 86400 {
			retry = time.Now().Add(time.Duration(seconds) * time.Second)
		}
		return nil, &outlookGraphError{status: response.StatusCode, retry: retry}
	}
	return response, nil
}

func outlookContinuation(initial, next string) bool {
	base, e1 := url.Parse(initial)
	target, e2 := url.Parse(next)
	if e1 != nil || e2 != nil || target.User != nil || target.Fragment != "" || target.Scheme != base.Scheme || target.Host != base.Host {
		return false
	}
	return outlookFolderPath(base.Path) == outlookFolderPath(target.Path)
}

// Graph may return either /mailFolders/ID or /mailfolders('ID').
// Normalize route segments while preserving the case of opaque folder IDs.
func outlookFolderPath(path string) string {
	lower := strings.ToLower(path)
	marker := "/mailfolders"
	start := strings.Index(lower, marker)
	if start < 0 {
		return path
	}
	start += len(marker)
	tail := path[start:]
	if strings.HasPrefix(tail, "('") {
		end := strings.Index(tail, "')")
		if end > 2 {
			return lower[:start] + "/" + tail[2:end] + strings.ToLower(tail[end+2:])
		}
	}
	if !strings.HasPrefix(tail, "/") {
		return lower[:start] + tail
	}
	for _, suffix := range []string{"/messages/delta", "/childfolders", "/messagerules"} {
		if strings.HasSuffix(strings.ToLower(tail), suffix) {
			return lower[:start] + tail[:len(tail)-len(suffix)] + suffix
		}
	}
	return lower[:start] + tail
}

type outlookRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}
type outlookRemoteMessage struct {
	ID, ConversationID, Subject, WebLink, ParentFolderID, ChangeKey string
	ETag                                                            string `json:"@odata.etag"`
	SentDateTime                                                    time.Time
	From, Sender                                                    outlookRecipient
	ToRecipients, CcRecipients, BccRecipients, ReplyTo              []outlookRecipient
	ReceivedDateTime, LastModifiedDateTime, CreatedDateTime         time.Time
	IsRead, IsDraft, HasAttachments                                 bool
	Removed                                                         json.RawMessage `json:"@removed"`
	Body                                                            struct{ ContentType, Content string }
}

func (m outlookRemoteMessage) sender() string {
	if m.From.EmailAddress.Address != "" {
		return m.From.EmailAddress.Address
	}
	return m.Sender.EmailAddress.Address
}

type outlookRemoteFolder struct {
	ID, DisplayName, ParentFolderID string
	ChildFolderCount                int
}
type outlookPage[T any] struct {
	Value *[]T   `json:"value"`
	Next  string `json:"@odata.nextLink"`
	Delta string `json:"@odata.deltaLink"`
}

func (s *Service) outlookFolders(ctx context.Context, token string) (map[string]outlookRemoteFolder, map[string]string, error) {
	root := s.outlookRoot() + "/mailFolders"
	folders := map[string]outlookRemoteFolder{}
	pending := []string{root}
	seenPages := map[string]bool{}
	for len(pending) > 0 {
		initial := pending[0]
		pending = pending[1:]
		for next := initial; next != ""; {
			if !outlookContinuation(initial, next) || seenPages[next] || len(seenPages) >= 200 {
				return nil, nil, &outlookGraphError{status: 502}
			}
			seenPages[next] = true
			var page outlookPage[outlookRemoteFolder]
			if err := s.outlookRequest(ctx, token, http.MethodGet, next, nil, &page); err != nil {
				return nil, nil, err
			}
			if page.Value == nil {
				return nil, nil, &outlookGraphError{status: 502}
			}
			for _, folder := range *page.Value {
				if folder.ID == "" || len(folder.ID) > 1024 || len(folders) >= 200 {
					return nil, nil, &outlookGraphError{status: 502}
				}
				if _, ok := folders[folder.ID]; ok {
					continue
				}
				folders[folder.ID] = folder
				if folder.ChildFolderCount > 0 {
					pending = append(pending, root+"/"+url.PathEscape(folder.ID)+"/childFolders")
				}
			}
			next = page.Next
		}
	}
	kinds := map[string]string{}
	for _, kind := range []string{"inbox", "sentitems", "drafts", "archive"} {
		var folder outlookRemoteFolder
		err := s.outlookRequest(ctx, token, http.MethodGet, root+"/"+kind, nil, &folder)
		var graph *outlookGraphError
		if kind == "archive" && errors.As(err, &graph) && graph.status == 404 {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if folder.ID == "" || len(folder.ID) > 1024 {
			return nil, nil, &outlookGraphError{status: 502}
		}
		folders[folder.ID] = folder
		kinds[folder.ID] = kind
	}
	return folders, kinds, nil
}
