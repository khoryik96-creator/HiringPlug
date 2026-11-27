package claims

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The storage providers: Google Drive, OneDrive (Microsoft Graph) and
// Dropbox, each signed in with OAuth (authorization code + PKCE) and then
// called with the account's access token.

const (
	GoogleDrive = "google_drive"
	OneDrive    = "onedrive"
	Dropbox     = "dropbox"
)

// Endpoints are the providers' addresses (tests point them at a fake).
type Endpoints struct {
	MicrosoftAuthorize, MicrosoftToken, Graph                               string
	GoogleAuthorize, GoogleToken, GoogleUserInfo, GoogleDrive, GoogleUpload string
	DropboxAuthorize, DropboxToken, DropboxAPI, DropboxContent              string
}

// RealEndpoints are the providers' public addresses (not credentials).
var RealEndpoints = Endpoints{ //nolint:gosec // public OAuth endpoints, not credentials
	MicrosoftAuthorize: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	MicrosoftToken:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	Graph:              "https://graph.microsoft.com/v1.0",
	GoogleAuthorize:    "https://accounts.google.com/o/oauth2/v2/auth",
	GoogleToken:        "https://oauth2.googleapis.com/token",
	GoogleUserInfo:     "https://www.googleapis.com/oauth2/v3/userinfo",
	GoogleDrive:        "https://www.googleapis.com/drive/v3",
	GoogleUpload:       "https://www.googleapis.com/upload/drive/v3",
	DropboxAuthorize:   "https://www.dropbox.com/oauth2/authorize",
	DropboxToken:       "https://api.dropboxapi.com/oauth2/token",
	DropboxAPI:         "https://api.dropboxapi.com/2",
	DropboxContent:     "https://content.dropboxapi.com/2",
}

// oauthSpec is how a provider is signed in to.
type oauthSpec struct {
	authorize, token, scope string
	extra                   url.Values // more sign-in parameters
}

func (s *Storage) spec(provider string) oauthSpec {
	e := s.config.Endpoints
	switch provider {
	case GoogleDrive:
		// drive.file: only the folders and files HyPlug makes, nothing else in the drive.
		return oauthSpec{e.GoogleAuthorize, e.GoogleToken, "openid email https://www.googleapis.com/auth/drive.file",
			url.Values{"access_type": {"offline"}, "prompt": {"consent select_account"}}}
	case Dropbox:
		return oauthSpec{e.DropboxAuthorize, e.DropboxToken, "account_info.read files.content.write",
			url.Values{"token_access_type": {"offline"}, "force_reapprove": {"true"}}}
	}
	return oauthSpec{e.MicrosoftAuthorize, e.MicrosoftToken, "openid offline_access User.Read Files.ReadWrite",
		url.Values{"prompt": {"select_account"}}}
}

// ProviderName names a provider for people.
func ProviderName(provider string) string {
	switch provider {
	case GoogleDrive:
		return "Google Drive"
	case Dropbox:
		return "Dropbox"
	}
	return "OneDrive"
}

type tokens struct {
	AccessToken, RefreshToken string
	ExpiresAt                 time.Time
}

// providerError is a provider's refusal (Status 0: it could not be reached).
type providerError struct {
	Provider string
	Status   int
	Reason   string // the provider's short error code, when it gave one
}

func (e *providerError) Error() string {
	name := ProviderName(e.Provider)
	switch {
	case e.Status == 0:
		return name + " could not be reached; the upload will be tried again"
	case e.reconnect():
		return name + " no longer accepts HyPlug's access: connect the account again in Settings > Claims storage"
	case e.Status == 507 || e.Reason == "insufficient_space" || e.Reason == "storageQuotaExceeded":
		return name + " is full: free some space, then try the uploads again"
	}
	return fmt.Sprintf("%s refused the upload (HTTP %d)", name, e.Status)
}

// reconnect: the account's access was withdrawn or expired for good.
func (e *providerError) reconnect() bool {
	return e.Status == 401 || e.Reason == "invalid_grant" || e.Reason == "expired_access_token" || e.Reason == "invalid_access_token"
}

// transient: worth trying again later.
func (e *providerError) transient() bool {
	return e.Status == 0 || e.Status == 408 || e.Status == 429 || (e.Status >= 500 && e.Status != 507)
}

type call struct {
	method, target, token, contentType string
	body                               []byte
	header                             map[string]string
}

// do sends a request to a provider and returns its answer (at most 1 MiB:
// the answers are small JSON) and headers; anything but a 2xx is a
// *providerError.
func (s *Storage) do(ctx context.Context, provider string, c call) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, c.method, c.target, bytes.NewReader(c.body))
	if err != nil {
		return nil, nil, errors.New("the storage request could not be prepared")
	}
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	res, err := s.config.HTTP.Do(req)
	if err != nil {
		return nil, nil, &providerError{Provider: provider}
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if res.StatusCode/100 != 2 {
		return nil, res.Header, &providerError{Provider: provider, Status: res.StatusCode, Reason: reasonOf(raw)}
	}
	if err != nil || len(raw) > 1<<20 {
		return nil, nil, errors.New(ProviderName(provider) + "'s answer could not be read")
	}
	return raw, res.Header, nil
}

// reasonOf is a provider's short error code: OAuth's "error", Graph's
// error.code, Google's error.errors[0].reason, Dropbox's error_summary.
func reasonOf(raw []byte) string {
	var e struct {
		Error   json.RawMessage `json:"error"`
		Summary string          `json:"error_summary"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	if e.Summary != "" {
		return strings.SplitN(e.Summary, "/", 2)[0]
	}
	var code string
	if json.Unmarshal(e.Error, &code) == nil {
		return code
	}
	var nested struct {
		Code   any `json:"code"`
		Errors []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(e.Error, &nested)
	if len(nested.Errors) > 0 {
		return nested.Errors[0].Reason
	}
	if c, ok := nested.Code.(string); ok {
		return c
	}
	return ""
}

// exchange trades a code (or a refresh token) for tokens.
func (s *Storage) exchange(ctx context.Context, provider string, app App, form url.Values) (tokens, error) {
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	raw, _, err := s.do(ctx, provider, call{method: "POST", target: s.spec(provider).token, contentType: "application/x-www-form-urlencoded", body: []byte(form.Encode())})
	if err != nil {
		return tokens{}, err
	}
	var p struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Type    string `json:"token_type"`
		Expires int    `json:"expires_in"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Access == "" || !strings.EqualFold(p.Type, "Bearer") || p.Expires < 1 || p.Expires > 86400 {
		return tokens{}, errors.New(ProviderName(provider) + " returned an invalid authorization")
	}
	return tokens{AccessToken: p.Access, RefreshToken: p.Refresh, ExpiresAt: s.now().Add(time.Duration(p.Expires) * time.Second)}, nil
}

// identity is the signed-in account's email address.
func (s *Storage) identity(ctx context.Context, provider, token string) (string, error) {
	e := s.config.Endpoints
	var raw []byte
	var err error
	switch provider {
	case GoogleDrive:
		raw, _, err = s.do(ctx, provider, call{method: "GET", target: e.GoogleUserInfo, token: token})
	case Dropbox:
		raw, _, err = s.do(ctx, provider, call{method: "POST", target: e.DropboxAPI + "/users/get_current_account", token: token, contentType: "application/json", body: []byte("null")})
	default:
		raw, _, err = s.do(ctx, provider, call{method: "GET", target: e.Graph + "/me?$select=mail,userPrincipalName", token: token})
		if err == nil { // the account must have a OneDrive
			_, _, err = s.do(ctx, provider, call{method: "GET", target: e.Graph + "/me/drive?$select=id", token: token})
		}
	}
	if err != nil {
		return "", err
	}
	var p struct{ Email, Mail, UserPrincipalName string }
	_ = json.Unmarshal(raw, &p)
	account := strings.TrimSpace(strings.ToLower(firstOf(p.Email, p.Mail, p.UserPrincipalName)))
	if account == "" || len(account) > 320 || strings.ContainsAny(account, "\r\n") {
		return "", errors.New(ProviderName(provider) + " did not say which account signed in")
	}
	return account, nil
}

func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
