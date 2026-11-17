package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const tokenEndpoint = "https://id.jobadder.com/connect/token"

type Connection struct {
	API          string `json:"api"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Account      string `json:"account"`
	Fingerprint  string `json:"fingerprint"`
}
type Client struct {
	Config Connection
	HTTP   *http.Client
	Save   func(Connection) error
	Wait   func(context.Context, time.Duration) error
	Pace   time.Duration
}

func secureHTTP() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirect refused") }}
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var apiHost = regexp.MustCompile(`^api(?:\.[a-z0-9-]+)*\.jobadder\.com$`)

func validateAPI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid JobAdder API address")
	}
	if u.Scheme != "https" || !apiHost.MatchString(strings.ToLower(u.Hostname())) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || strings.TrimSuffix(u.Path, "/") != "/v2" {
		return errors.New("use the HTTPS JobAdder API address returned with your token, ending in /v2")
	}
	return nil
}
func tokenExchange(ctx context.Context, h *http.Client, endpoint string, form url.Values) (Connection, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Connection{}, errors.New("could not prepare authorization")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := h.Do(req)
	if err != nil {
		return Connection{}, errors.New("JobAdder authorization could not be reached")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return Connection{}, errors.New("JobAdder did not authorize this connection; check the app, redirect and permissions")
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		API          string `json:"api"`
		TokenType    string `json:"token_type"`
	}
	raw, err := readLimited(res.Body, 128<<10)
	if err != nil {
		return Connection{}, err
	}
	if json.Unmarshal(raw, &payload) != nil || payload.AccessToken == "" || !strings.EqualFold(payload.TokenType, "Bearer") {
		return Connection{}, errors.New("JobAdder returned an invalid authorization response")
	}
	if err = validateAPI(payload.API); err != nil {
		return Connection{}, err
	}
	return Connection{API: strings.TrimSuffix(payload.API, "/"), AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken}, nil
}
func (c *Client) refresh(ctx context.Context) error {
	old := c.Config
	if old.RefreshToken == "" || old.ClientID == "" || old.ClientSecret == "" {
		return errors.New("connection expired; reconnect JobAdder")
	}
	next, err := tokenExchange(ctx, c.HTTP, tokenEndpoint, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {old.RefreshToken}, "client_id": {old.ClientID}, "client_secret": {old.ClientSecret}})
	if err != nil {
		return err
	}
	if next.RefreshToken == "" {
		next.RefreshToken = old.RefreshToken
	}
	next.ClientID = old.ClientID
	next.ClientSecret = old.ClientSecret
	next.Account = old.Account
	next.Fingerprint = old.Fingerprint
	// Commit rotating credentials before retrying any API request.
	if c.Save != nil {
		if err = c.Save(next); err != nil {
			return errors.New("could not preserve refreshed credentials")
		}
	}
	c.Config = next
	return nil
}
func retryDelay(h string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(h); err == nil && seconds >= 0 {
		return time.Duration(min(seconds, 300)) * time.Second
	}
	if at, err := http.ParseTime(h); err == nil {
		return max(0, min(time.Until(at), 5*time.Minute))
	}
	return time.Duration(1<<attempt) * time.Second
}
func readLimited(r io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, errors.New("response could not be read")
	}
	if int64(len(body)) > max {
		return nil, errors.New("response exceeded discovery size limit")
	}
	return body, nil
}

// get never follows API-supplied links and never accepts caller-supplied methods.
func (c *Client) get(ctx context.Context, path string) ([]byte, int, error) {
	refreshed := false
	for attempt := 0; attempt < 4; attempt++ {
		if c.Pace > 0 {
			if err := c.Wait(ctx, c.Pace); err != nil {
				return nil, 0, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.Config.API, "/")+path, nil)
		if err != nil {
			return nil, 0, errors.New("invalid API request")
		}
		req.Header.Set("Authorization", "Bearer "+c.Config.AccessToken)
		req.Header.Set("Accept", "application/json")
		res, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			if attempt < 3 {
				if err = c.Wait(ctx, time.Duration(1<<attempt)*time.Second); err != nil {
					return nil, 0, err
				}
				continue
			}
			return nil, 0, errors.New("JobAdder could not be reached")
		}
		status := res.StatusCode
		if status == 401 && !refreshed {
			_ = res.Body.Close()
			refreshed = true
			if err = c.refresh(ctx); err != nil {
				return nil, 401, err
			}
			attempt--
			continue
		}
		if status == 429 || status >= 500 {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			_ = res.Body.Close()
			if attempt < 3 {
				if err = c.Wait(ctx, delay); err != nil {
					return nil, status, err
				}
				continue
			}
			return nil, status, errors.New("JobAdder is rate-limited or temporarily unavailable; resume later")
		}
		if status != 200 {
			_ = res.Body.Close()
			return nil, status, errors.New("resource unavailable")
		}
		raw, err := readLimited(res.Body, 8<<20)
		_ = res.Body.Close()
		return raw, status, err
	}
	return nil, 0, errors.New("request could not complete")
}
