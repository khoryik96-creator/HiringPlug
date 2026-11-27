package calendars

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const calendarScope = "openid offline_access User.Read Calendars.ReadWrite"

type tokens struct {
	AccessToken, RefreshToken, Scope string
	ExpiresAt                        time.Time
}
type providerError struct {
	Status int
	Retry  time.Duration
}

func (e *providerError) Error() string {
	if e.Status == 0 {
		return "Outlook could not be reached; the sync will retry"
	}
	if e.Status == 401 || e.Status == 403 {
		return "Outlook did not allow calendar access; reconnect and approve calendar permissions"
	}
	if e.Status == 412 {
		return "Outlook changed this event; review it before keeping the HyPlug schedule"
	}
	return fmt.Sprintf("Outlook calendar request failed (HTTP %d)", e.Status)
}
func (s *Service) request(ctx context.Context, method, target, token string, body []byte, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("calendar request could not be prepared")
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Prefer", `outlook.timezone="UTC", IdType="ImmutableId", outlook.body-content-type="text"`)
	if key != "" {
		req.Header.Set("If-Match", key)
	}
	response, err := s.config.HTTP.Do(req)
	if err != nil {
		return nil, &providerError{Status: 0}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode/100 != 2 {
		delay := time.Duration(0)
		if raw := response.Header.Get("Retry-After"); raw != "" {
			if seconds, e := time.ParseDuration(raw + "s"); e == nil && seconds > 0 {
				delay = seconds
			} else if at, e := http.ParseTime(raw); e == nil && at.After(time.Now()) {
				delay = time.Until(at)
			}
		}
		return nil, &providerError{Status: response.StatusCode, Retry: delay}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (5<<20)+1))
	if err != nil || len(raw) > 5<<20 {
		return nil, errors.New("outlook response could not be read within the calendar limit")
	}
	return raw, nil
}
func (s *Service) exchange(ctx context.Context, app Application, form url.Values) (tokens, error) {
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	raw, err := s.request(ctx, "POST", s.config.Provider.TokenURL, "", []byte(form.Encode()), "")
	if err != nil {
		return tokens{}, err
	}
	var p struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Type    string `json:"token_type"`
		Expires int    `json:"expires_in"`
		Scope   string `json:"scope"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Access == "" || !strings.EqualFold(p.Type, "Bearer") || p.Expires < 1 || p.Expires > 86400 {
		return tokens{}, errors.New("microsoft returned an invalid calendar authorization")
	}
	return tokens{AccessToken: p.Access, RefreshToken: p.Refresh, ExpiresAt: s.now().Add(time.Duration(p.Expires) * time.Second), Scope: p.Scope}, nil
}
func writeScope(scope string) bool {
	for _, v := range strings.Fields(scope) {
		if strings.EqualFold(v, "Calendars.ReadWrite") || strings.EqualFold(v, "https://graph.microsoft.com/Calendars.ReadWrite") {
			return true
		}
	}
	return false
}

type outlookIdentity struct {
	ID, Address, CalendarID, CalendarName string
	Teams                                 bool
}

func (s *Service) profile(ctx context.Context, token string) (outlookIdentity, error) {
	var out outlookIdentity
	raw, err := s.request(ctx, "GET", s.graph("/me?$select=id,mail,userPrincipalName"), token, nil, "")
	if err != nil {
		return out, err
	}
	var p struct{ ID, Mail, UserPrincipalName string }
	if json.Unmarshal(raw, &p) != nil || p.ID == "" {
		return out, errors.New("microsoft did not return a calendar account identity")
	}
	address := p.Mail
	if address == "" {
		address = p.UserPrincipalName
	}
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
		return out, errors.New("this Microsoft account has no usable Outlook mailbox address")
	}
	raw, err = s.request(ctx, "GET", s.graph("/me/calendar?$select=id,name,canEdit,allowedOnlineMeetingProviders"), token, nil, "")
	if err != nil {
		return out, err
	}
	var cal struct {
		ID, Name                      string
		CanEdit                       bool
		AllowedOnlineMeetingProviders []string
	}
	if json.Unmarshal(raw, &cal) != nil || cal.ID == "" || !cal.CanEdit {
		return out, errors.New("outlook did not provide an editable default calendar")
	}
	out = outlookIdentity{ID: p.ID, Address: strings.ToLower(address), CalendarID: cal.ID, CalendarName: cal.Name}
	for _, provider := range cal.AllowedOnlineMeetingProviders {
		if provider == "teamsForBusiness" {
			out.Teams = true
		}
	}
	return out, nil
}
func (s *Service) graph(path string) string {
	return strings.TrimRight(s.config.Provider.GraphURL, "/") + path
}
func (s *Service) calendarPath(id string) string { return "/me/calendars/" + url.PathEscape(id) }
func (s *Service) validNext(next, path string) bool {
	u, err := url.Parse(next)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	base, err := url.Parse(s.graph(path))
	return err == nil && u.Scheme == base.Scheme && u.Host == base.Host && u.Path == base.Path
}
