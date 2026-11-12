package emails

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdmail "net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Provider replies may include tokens, subjects or addresses. Errors contain
// only fixed messages/status codes, never a provider's response text.
func (s *Service) providerRequest(ctx context.Context, method, target, contentType, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("mail provider request could not be created")
	}
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := s.connections.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("mail provider could not be reached")
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, errors.New("mail provider response could not be read")
	}
	if response.StatusCode/100 != 2 {
		if response.StatusCode == 401 || response.StatusCode == 403 {
			return nil, fmt.Errorf("%w: mailbox authorization was refused; reconnect the mailbox", mail.ErrPermanent)
		}
		if response.StatusCode == 400 || response.StatusCode == 404 || response.StatusCode == 413 {
			return nil, fmt.Errorf("%w: mail provider refused the request (HTTP %d)", mail.ErrPermanent, response.StatusCode)
		}
		return nil, fmt.Errorf("mail provider is temporarily unavailable (HTTP %d)", response.StatusCode)
	}
	return raw, nil
}
func (s *Service) exchangeMailboxToken(ctx context.Context, endpoint OAuthProvider, app OAuthApplication, form url.Values) (mailboxSecret, error) {
	form.Set("client_id", app.ClientID)
	form.Set("client_secret", app.ClientSecret)
	raw, err := s.providerRequest(ctx, "POST", endpoint.TokenURL, "application/x-www-form-urlencoded", "", []byte(form.Encode()))
	if err != nil {
		return mailboxSecret{}, err
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if json.Unmarshal(raw, &result) != nil || result.AccessToken == "" || !strings.EqualFold(result.TokenType, "Bearer") || result.ExpiresIn <= 0 || result.ExpiresIn > 86400 {
		return mailboxSecret{}, errors.New("mail provider returned an invalid authorization")
	}
	return mailboxSecret{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, ExpiresAt: time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)}, nil
}
func (s *Service) mailboxProfile(ctx context.Context, provider string, endpoint OAuthProvider, token string) (string, error) {
	raw, err := s.providerRequest(ctx, "GET", endpoint.ProfileURL, "application/json", token, nil)
	if err != nil {
		return "", err
	}
	var profile struct {
		Email     string `json:"email"`
		Verified  bool   `json:"email_verified"`
		Mail      string `json:"mail"`
		Principal string `json:"userPrincipalName"`
	}
	if json.Unmarshal(raw, &profile) != nil {
		return "", errors.New("mail provider returned an invalid account")
	}
	address := profile.Mail
	if address == "" {
		address = profile.Principal
	}
	if provider == "google" {
		if !profile.Verified {
			return "", errors.New("mailbox address is not verified")
		}
		address = profile.Email
	}
	parsed, err := stdmail.ParseAddress(address)
	if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
		return "", errors.New("mail provider returned an invalid mailbox address")
	}
	return strings.ToLower(address), nil
}
func (s *Service) mailboxAccessIn(ctx context.Context, tx pgx.Tx, row store.EmailMailbox) (string, error) {
	var secret mailboxSecret
	if err := s.openMailbox(row.TenantID, row.UserID, row.Provider, row.SealedSecret, &secret); err != nil {
		return "", fmt.Errorf("%w: reconnect the mailbox after its encryption settings changed", mail.ErrPermanent)
	}
	if time.Until(secret.ExpiresAt) > time.Minute {
		return secret.AccessToken, nil
	}
	if secret.RefreshToken == "" {
		return "", fmt.Errorf("%w: mailbox authorization expired; reconnect the mailbox", mail.ErrPermanent)
	}
	endpoint := s.endpoint(row.Provider)
	app, err := s.appFor(ctx, tx, row.TenantID, row.Provider)
	if err != nil {
		return "", err
	}
	refreshed, err := s.exchangeMailboxToken(ctx, endpoint, app, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secret.RefreshToken}})
	if err != nil {
		return "", err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = secret.RefreshToken
	}
	sealed, err := s.sealMailbox(row.TenantID, row.UserID, row.Provider, refreshed)
	if err != nil {
		return "", err
	}
	if err = store.New(tx).RefreshMailboxSecret(ctx, store.RefreshMailboxSecretParams{ID: row.ID, SealedSecret: sealed}); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}
func (s *Service) sendOAuthMailbox(ctx context.Context, row store.EmailMailbox, msg mail.Message) error {
	token, err := s.mailboxAccess(ctx, row)
	if err != nil {
		return err
	}
	msg.ReplyTo = row.Address
	raw, err := mail.Build(row.Address, msg, time.Now())
	if err != nil {
		return fmt.Errorf("%w: email content is invalid", mail.ErrPermanent)
	}
	// SMTP uses an envelope for Bcc and deliberately omits its header in Build.
	// API transports instead derive recipients from MIME, then strip Bcc before
	// delivering. Include it only in the provider-bound request.
	if len(msg.Bcc) > 0 {
		raw = append([]byte("Bcc: "+strings.Join(msg.Bcc, ", ")+"\r\n"), raw...)
	}
	endpoint := s.endpoint(row.Provider)
	if row.Provider == "google" {
		body, _ := json.Marshal(map[string]string{"raw": base64.RawURLEncoding.EncodeToString(raw)})
		_, err = s.providerRequest(ctx, "POST", endpoint.SendURL, "application/json", token, body)
		return err
	}
	if len(raw) <= 2<<20 {
		_, err = s.providerRequest(ctx, "POST", endpoint.SendURL, "text/plain", token, []byte(base64.StdEncoding.EncodeToString(raw)))
		return err
	}
	return s.sendLargeMicrosoftMail(ctx, endpoint, token, msg)
}

// Refresh-token rotation cannot roll back when a later send fails. Commit it in
// an isolated, short transaction before delivery. A single temporary connection
// also avoids exhausting the worker pool while its job transactions are open.
func (s *Service) mailboxAccess(ctx context.Context, row store.EmailMailbox) (string, error) {
	var secret mailboxSecret
	if err := s.openMailbox(row.TenantID, row.UserID, row.Provider, row.SealedSecret, &secret); err != nil {
		return "", fmt.Errorf("%w: reconnect the mailbox after its encryption settings changed", mail.ErrPermanent)
	}
	if time.Until(secret.ExpiresAt) > time.Minute {
		return secret.AccessToken, nil
	}
	config := s.pool.Config()
	config.MinConns = 0
	config.MaxConns = 1
	isolated, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return "", err
	}
	defer isolated.Close()
	var token string
	err = database.InTx(ctx, isolated, authz.System(row.TenantID), func(tx pgx.Tx) error {
		latest, err := store.New(tx).MailboxForUser(ctx, row.UserID)
		if err != nil {
			return err
		}
		if latest.ID != row.ID {
			return fmt.Errorf("%w: the selected mailbox was replaced; review and send again", mail.ErrPermanent)
		}
		token, err = s.mailboxAccessIn(ctx, tx, latest)
		return err
	})
	return token, err
}
