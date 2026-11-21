package contacts

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
)

type apolloClient struct {
	config.Apollo
	http *http.Client
}

const apolloProtocol = 2

type apolloReply struct {
	Protocol         int    `json:"protocol"`
	Connected        *bool  `json:"connected"`
	IdentityChecked  bool   `json:"identity_checked"`
	NotDispatched    bool   `json:"not_dispatched"`
	FlowID           string `json:"flow_id"`
	AuthorizationURL string `json:"authorization_url"`
	ConnectionID     string `json:"connection_id"`
	Name             string `json:"name"`
	State            string `json:"state"`
	Email            string `json:"email"`
	EmailStatus      string `json:"email_status"`
	Reason           string `json:"reason"`
}

type apolloPerson struct {
	URL        string `json:"url"`
	Name       string `json:"name"`
	Company    string `json:"company"`
	CompanyURL string `json:"company_url"`
	CompanyID  string `json:"company_id"`
}

var apolloHexID = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ConfigureApollo enables Apollo for Find HR. The broker origin is deployment
// configuration. Recruiters supply neither an API key nor a server address.
func (m *Module) ConfigureApollo(settings config.Apollo, transport http.RoundTripper) {
	m.service.apollo = apolloClient{Apollo: settings, http: &http.Client{
		Transport:     transport,
		Timeout:       42 * time.Second, // allows a broker token refresh followed by People Match
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c apolloClient) enabled() bool { return c.BrokerOrigin != "" && len(c.KeysSecret) >= 32 }

func (c apolloClient) post(ctx context.Context, path string, body any) (apolloReply, int, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return apolloReply{}, 0, errApolloResponse
	}
	// #nosec G107 -- validated deployment origin and internal, fixed broker routes.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BrokerOrigin+path, bytes.NewReader(encoded))
	if err != nil {
		return apolloReply{}, 0, errApolloResponse
	}
	req.GetBody = nil // a paid request is never replayed on a broken connection
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// The deployed Cloudflare broker rejected default script user agents.
	req.Header.Set("User-Agent", "CVStudio-Apollo/1.0")
	res, err := c.http.Do(req)
	if err != nil {
		return apolloReply{}, 0, errApolloResponse
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return apolloReply{}, res.StatusCode, errApolloResponse
	}
	const maxResponse = 16 * 1024
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	var reply apolloReply
	if err != nil || len(raw) > maxResponse || json.Unmarshal(raw, &reply) != nil {
		return apolloReply{}, res.StatusCode, errApolloResponse
	}
	return reply, res.StatusCode, nil
}

var errApolloResponse = errors.New("apollo broker request could not be confirmed")

func apolloConnectionError(status int) error {
	message := "The Apollo broker did not respond. Try again in a moment."
	switch status {
	case http.StatusForbidden:
		message = "The Apollo broker blocked the request. Ask your administrator to check Cloudflare access."
	case http.StatusTooManyRequests:
		message = "The Apollo broker reached its sign-in limit. Try again later."
	case http.StatusServiceUnavailable, http.StatusNotFound:
		message = "The Apollo broker is unavailable. Ask your administrator to check its deployment and callback setup."
	case http.StatusOK:
		message = "The Apollo broker returned an unexpected sign-in response. Ask your administrator to check its version."
	}
	return apperr.New(api.ErrorCodeAPOLLOBROKERUNAVAILABLE, message)
}

func (c apolloClient) validLogin(reply apolloReply) bool {
	if !apolloHexID.MatchString(reply.FlowID) {
		return false
	}
	u, err := url.Parse(reply.AuthorizationURL)
	if err != nil || u.Scheme != "https" || u.Host != "app.apollo.io" || u.User != nil ||
		u.Path != "/" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery {
		return false
	}
	// Fragment query parameters contain nested percent encodings. Read the
	// original fragment before ParseQuery so each value is decoded once.
	_, fragment, _ := strings.Cut(reply.AuthorizationURL, "#")
	path, query, _ := strings.Cut(fragment, "?")
	params, err := url.ParseQuery(query)
	if err != nil || path != "/oauth/authorize" {
		return false
	}
	for _, name := range []string{"state", "redirect_uri", "response_type", "client_id", "scope"} {
		if len(params[name]) != 1 || params.Get(name) == "" {
			return false
		}
	}
	return params.Get("state") == reply.FlowID && params.Get("redirect_uri") == c.BrokerOrigin+"/callback" && params.Get("response_type") == "code"
}

func apolloRandomID() string {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw) // crypto/rand.Read cannot return an error on the supported Go version
	return hex.EncodeToString(raw)
}

func (c apolloClient) confirmation(tenant, user, company, flow, cancelledReceipt string, person apolloPerson) string {
	mac := hmac.New(sha256.New, c.KeysSecret)
	// A fixed array of strings and a plain struct always marshal successfully.
	raw, _ := json.Marshal([]any{"apollo-confirm-v2", tenant, user, company, flow, cancelledReceipt, person})
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c apolloClient) aead() (cipher.AEAD, error) {
	key := sha256.Sum256(append([]byte("hiringplug/apollo/v1:"), c.KeysSecret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, errApolloResponse
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (c apolloClient) seal(tenant, user, origin, id, purpose, value string) (string, error) {
	aead, err := c.aead()
	if err != nil {
		return "", err
	}
	context := []byte(strings.Join([]string{tenant, user, origin, id, purpose}, "\x00"))
	return hex.EncodeToString(aead.Seal(nil, nil, []byte(value), context)), nil //nolint:gosec // NewGCMWithRandomNonce generates and prepends its nonce
}

func (c apolloClient) open(tenant, user, origin, id, purpose, sealed string) (string, error) {
	aead, err := c.aead()
	if err != nil {
		return "", errApolloResponse
	}
	raw, err := hex.DecodeString(sealed)
	if err != nil {
		return "", errApolloResponse
	}
	context := []byte(strings.Join([]string{tenant, user, origin, id, purpose}, "\x00"))
	clear, err := aead.Open(nil, nil, raw, context)
	if err != nil {
		return "", errApolloResponse
	}
	return string(clear), nil
}

func apolloCompanyURL(raw string) string { return strings.ToLower(CompanyPageURL(raw)) }

const personalMailDomains = " gmail.com googlemail.com yahoo.com yahoo.com.my yahoo.com.sg yahoo.co.uk ymail.com rocketmail.com hotmail.com hotmail.co.uk outlook.com live.com msn.com aol.com icloud.com me.com mac.com mail.com email.com gmx.com gmx.net proton.me protonmail.com protonmail.ch pm.me "

var apolloMail = regexp.MustCompile("^[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,63}$")

func apolloEmailReject(value string) api.ApolloLookupReason {
	switch {
	case value == "":
		return api.ApolloLookupReasonEmailNotReturned
	case strings.EqualFold(value, "email_not_unlocked@domain.com"):
		return api.ApolloLookupReasonEmailRedacted
	case len(value) > 254 || !apolloMail.MatchString(value):
		return api.ApolloLookupReasonEmailUnusable
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return api.ApolloLookupReasonEmailUnusable
	}
	_, domain, _ := strings.Cut(value, "@")
	if strings.Contains(personalMailDomains, " "+strings.ToLower(domain)+" ") {
		return api.ApolloLookupReasonPersonalEmailOnly
	}
	return ""
}
