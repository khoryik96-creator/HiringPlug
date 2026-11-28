// Package entra signs people in with Microsoft (Entra ID, OpenID Connect):
// the address that sends them to Microsoft (PKCE S256, state and nonce),
// exchanging the code Microsoft sends back for an ID token, and checking that
// token fully (idtoken.go): an RS256 signature by one of Microsoft's
// published keys (kept for a day, fetched again for a key not seen yet), the
// issuer of the token's own organisation, the audience, its times, the nonce,
// and the organisation. Personal Microsoft accounts are always refused.
//
// Microsoft's addresses are code-owned; only tests point Login elsewhere.
// Nothing Microsoft answers is ever passed on to the browser or the logs.
package entra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultLogin is Microsoft's sign-in service.
	DefaultLogin = "https://login.microsoftonline.com"
	// ConsumersTenant is the organisation of personal Microsoft accounts
	// (outlook.com, hotmail.com): never accepted.
	ConsumersTenant = "9188040d-6c67-4c5b-b112-36a304b66dad"
	// Organizations is the authority for work and school accounts of any
	// organisation (the organisation is then checked against the company's).
	Organizations = "organizations"
	// Scope asks only who the person is.
	Scope = "openid profile email"
	// OutlookScope also asks for the person's Outlook mail and calendar
	// (Microsoft Graph, delegated), with a refresh token: one consent for
	// signing in, the mailbox (Settings > Email connection) and the calendar.
	OutlookScope = Scope + " offline_access User.Read Mail.Send Mail.ReadWrite Calendars.ReadWrite"
)

var guidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeID lower-cases and trims an organisation or object ID.
func NormalizeID(id string) string { return strings.ToLower(strings.TrimSpace(id)) }

// IsID reports whether id is a GUID in normal (lower-case) form.
func IsID(id string) bool { return guidRe.MatchString(id) }

// IsOrganisation reports whether id can be a company's organisation: a GUID,
// and not the one of personal accounts.
func IsOrganisation(id string) bool { return IsID(id) && id != ConsumersTenant }

// Issuer is the issuer of ID tokens for an organisation (v2.0 endpoints).
// It is Microsoft's real address even when tests point Login elsewhere.
func Issuer(tenantID string) string { return DefaultLogin + "/" + tenantID + "/v2.0" }

// Client talks to Microsoft's sign-in service.
type Client struct {
	login string
	http  *http.Client
	now   func() time.Time

	mu       sync.Mutex
	keys     map[string]keySet // by organisation
	fetching singleflight.Group
}

// New returns a client for the sign-in service at login (DefaultLogin when
// empty). httpClient nil: one with a 15-second timeout that follows no
// redirects.
func New(login string, httpClient *http.Client) *Client {
	if login == "" {
		login = DefaultLogin
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Client{login: strings.TrimRight(login, "/"), http: httpClient, now: time.Now, keys: map[string]keySet{}}
}

// UseClock replaces the clock (tests).
func (c *Client) UseClock(now func() time.Time) { c.now = now }

// validAuthority reports whether a is "organizations" or an organisation ID:
// it goes into Microsoft's addresses, so nothing else may.
func validAuthority(a string) bool { return a == Organizations || IsOrganisation(a) }

// Authorize is what the address that sends someone to Microsoft carries.
type Authorize struct {
	Authority, ClientID, RedirectURI string
	State, Nonce, Challenge          string // Challenge: base64url SHA-256 of the PKCE verifier
	LoginHint                        string // optional: the email they typed
	Scope                            string // Scope (the default) or OutlookScope
}

// AuthorizeURL is the address that sends someone to Microsoft to sign in.
func (c *Client) AuthorizeURL(a Authorize) (string, error) {
	if !validAuthority(a.Authority) {
		return "", errors.New("entra: invalid authority")
	}
	q := url.Values{
		"client_id": {a.ClientID}, "response_type": {"code"}, "response_mode": {"query"}, "redirect_uri": {a.RedirectURI},
		"scope": {scopeOr(a.Scope)}, "state": {a.State}, "nonce": {a.Nonce},
		"code_challenge": {a.Challenge}, "code_challenge_method": {"S256"}, "prompt": {"select_account"},
	}
	if a.LoginHint != "" {
		q.Set("login_hint", a.LoginHint)
	}
	return c.login + "/" + a.Authority + "/oauth2/v2.0/authorize?" + q.Encode(), nil
}

// Exchange is the code exchange: what the app and the browser's sign-in give.
type Exchange struct {
	Authority, ClientID, ClientSecret string
	Code, RedirectURI, Verifier       string
	Scope                             string // what the sign-in asked for (Scope when empty)
}

// Tokens are what Microsoft gives for a code: the ID token (not yet checked:
// see Validate) and, when Outlook was asked for too, Graph tokens.
type Tokens struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	ExpiresIn    int    // seconds
	Scope        string // what was granted
}

func scopeOr(s string) string {
	if s == "" {
		return Scope
	}
	return s
}

// maxAnswer bounds what is read from Microsoft.
const maxAnswer = 1 << 20

// IDToken exchanges the code Microsoft sent back for an ID token (not yet
// checked: see Validate). Microsoft's error text is never returned.
func (c *Client) IDToken(ctx context.Context, e Exchange) (string, error) {
	t, err := c.Tokens(ctx, e)
	return t.IDToken, err
}

// Tokens exchanges the code Microsoft sent back for its tokens (the ID token
// not yet checked: see Validate). Microsoft's error text is never returned.
func (c *Client) Tokens(ctx context.Context, e Exchange) (Tokens, error) {
	if !validAuthority(e.Authority) {
		return Tokens{}, errors.New("entra: invalid authority")
	}
	form := url.Values{
		"client_id": {e.ClientID}, "client_secret": {e.ClientSecret}, "grant_type": {"authorization_code"},
		"code": {e.Code}, "redirect_uri": {e.RedirectURI}, "code_verifier": {e.Verifier}, "scope": {scopeOr(e.Scope)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.login+"/"+e.Authority+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return Tokens{}, errors.New("entra: Microsoft's token service could not be reached")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxAnswer))
		return Tokens{}, fmt.Errorf("entra: Microsoft's token service answered %d", res.StatusCode)
	}
	var out struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(&out); err != nil || out.IDToken == "" {
		return Tokens{}, errors.New("entra: Microsoft's token service sent no ID token")
	}
	return Tokens{IDToken: out.IDToken, AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresIn: out.ExpiresIn, Scope: out.Scope}, nil
}
