package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) {
	server, err := testutil.StartSMTPServer()
	if err != nil {
		panic(err)
	}
	mailServer = server
	filesDir, err = os.MkdirTemp("", "hiringplug-test-files-")
	if err != nil {
		panic(err)
	}
	code := testutil.Main(m)
	server.Close()
	_ = os.RemoveAll(filesDir)
	os.Exit(code)
}

// filesDir is the file store of every test app, for the same reason as
// mailServer: a worker in one test may run another test's job (saving a PDF,
// reading a CV) and must find and keep files where that test looks.
var filesDir string

// mailServer receives the email every test app sends. It is one server for
// all of them: a worker in one test may run another test's jobs, so every
// test app must send the same way (tests tell their mail apart by address).
var mailServer *testutil.SMTPServer

// testMaxUpload keeps upload tests small.
const testMaxUpload = 256 << 10

// testApp is the real HTTP API (every module, real database, real
// middleware), called in-process.
type testApp struct {
	t       *testing.T
	handler http.Handler
	deps    app.Deps
	modules *app.Modules
	router  routers.Router
}

// newTestApp starts the API for a test; overrides change settings (HP_*).
func newTestApp(t *testing.T, overrides ...map[string]string) *testApp {
	t.Helper()
	env := map[string]string{
		"HP_ENV":               config.EnvTest,
		"HP_DATABASE_URL":      testutil.Current.AppURL,
		"HP_AUTH_JWT_SECRET":   testutil.JWTSecret,
		"HP_FILES_DIR":         filesDir,
		"HP_FILES_URL_SECRET":  "files-secret-for-tests-0123456789abcdef",
		"HP_INTERNAL_BASE_URL": "http://api.test",
		"HP_MAX_UPLOAD_BYTES":  "262144",
		"HP_LOG_LEVEL":         "error",
		// Webhook tests post to a local test server.
		"HP_ALLOW_PRIVATE_NETWORKS": "true",
	}
	for _, o := range overrides {
		maps.Copy(env, o)
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	require.NoError(t, err)
	logger := slog.New(slog.NewJSONHandler(testLogWriter{t}, &slog.HandlerOptions{Level: cfg.LogLevel}))
	deps, closeDeps, err := app.NewDeps(context.Background(), cfg, logger)
	require.NoError(t, err)
	t.Cleanup(closeDeps)
	deps.MailServers = fakeMailServers{}
	deps.Mailboxes = &fakeMailboxes{}
	deps.Mail = mail.NewSMTP(mail.Config{Addr: mailServer.Addr, From: "no-reply@agency.test", TLS: "none"})
	modules := app.NewModules(deps)
	handler, err := app.NewHTTPHandler(deps, modules)
	require.NoError(t, err)

	spec, err := api.GetSpec()
	require.NoError(t, err)
	spec.Servers = nil
	router, err := legacy.NewRouter(spec)
	require.NoError(t, err)
	return &testApp{t: t, handler: handler, deps: deps, modules: modules, router: router}
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

type response struct {
	t      *testing.T
	Status int
	Header http.Header
	Body   []byte
}

// JSON decodes the body into v.
func (r *response) JSON(v any) {
	r.t.Helper()
	require.NoError(r.t, json.Unmarshal(r.Body, v), "body: %s", r.Body)
}

// Map decodes the body into a generic map, to check exactly which keys exist.
func (r *response) Map() map[string]any {
	var m map[string]any
	r.JSON(&m)
	return m
}

// Error decodes an error body and checks it carries a request ID.
func (r *response) Error() api.Error {
	r.t.Helper()
	var e api.Error
	r.JSON(&e)
	require.True(r.t, e.Error.Code.Valid(), "unknown error code %q", e.Error.Code)
	require.NotEmpty(r.t, e.Error.RequestId)
	require.Equal(r.t, r.Header.Get("X-Request-Id"), e.Error.RequestId)
	return e
}

// request sends a request; body is JSON-encoded unless it is an io.Reader.
func (a *testApp) request(method, path, token string, body any, headers ...string) *response {
	a.t.Helper()
	var reader io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case io.Reader:
		reader = b
	default:
		data, err := json.Marshal(b)
		require.NoError(a.t, err)
		reader = bytes.NewReader(data)
		contentType = "application/json"
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	res := &response{t: a.t, Status: rec.Code, Header: rec.Header(), Body: rec.Body.Bytes()}
	a.checkAgainstContract(req, res)
	return res
}

// The job feed (XML) and calendar files are declared as strings in the
// contract, so they are checked as such.
func init() {
	asText := func(body io.Reader, _ http.Header, _ *openapi3.SchemaRef, _ openapi3filter.EncodingFn) (any, error) {
		b, err := io.ReadAll(body)
		return string(b), err
	}
	openapi3filter.RegisterBodyDecoder("application/xml", asText)
	openapi3filter.RegisterBodyDecoder("text/calendar", asText)
	openapi3filter.RegisterBodyDecoder("application/x-tar", asText) // a backup download
}

// checkAgainstContract fails the test if a response does not match what the
// contract promises for that operation and status. This is what keeps the Go
// handlers, the web client and the docs telling the same story.
func (a *testApp) checkAgainstContract(req *http.Request, res *response) {
	a.t.Helper()
	route, params, err := a.router.FindRoute(req)
	if err != nil {
		return // not an API route (health, files) or deliberately unknown
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route},
		Status:                 res.Status,
		Header:                 res.Header,
		Body:                   io.NopCloser(bytes.NewReader(res.Body)),
		Options:                &openapi3filter.Options{MultiError: true},
	}
	require.NoError(a.t, openapi3filter.ValidateResponse(context.Background(), input),
		"%s %s -> %d does not match the contract: %s", req.Method, req.URL.Path, res.Status, res.Body)
}

// upload sends a multipart upload. Pass source "" to leave the field out.
func (a *testApp) upload(path, token, source, fileName string, content []byte) *response {
	a.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if source != "" {
		require.NoError(a.t, w.WriteField("source", source))
	}
	if fileName != "" {
		part, err := w.CreateFormFile("file", fileName)
		require.NoError(a.t, err)
		_, err = part.Write(content)
		require.NoError(a.t, err)
	}
	require.NoError(a.t, w.Close())
	return a.request(http.MethodPost, path, token, &buf, "Content-Type", w.FormDataContentType())
}

// createCandidate creates a candidate and returns it.
func (a *testApp) createCandidate(token string, body map[string]any) api.Candidate {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/candidates", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var c api.Candidate
	res.JSON(&c)
	return c
}

// fieldNames lists the fields named in a VALIDATION_FAILED error.
func fieldNames(t *testing.T, e api.Error) []string {
	t.Helper()
	require.Equal(t, api.ErrorCodeVALIDATIONFAILED, e.Error.Code)
	require.NotNil(t, e.Error.Details)
	raw, ok := (*e.Error.Details)["fields"].([]any)
	require.True(t, ok, "details.fields missing: %v", *e.Error.Details)
	var names []string
	for _, f := range raw {
		names = append(names, f.(map[string]any)["field"].(string))
	}
	return names
}

// fakeMailServers answers mail server lookups without the network: every
// domain receives mail except *.nomail.test (no such domain) and
// *.dnsdown.test (the lookup fails).
type fakeMailServers struct{}

func (fakeMailServers) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	switch {
	case strings.HasSuffix(name, "nomail.test"):
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	case strings.HasSuffix(name, "dnsdown.test"):
		return nil, &net.DNSError{Err: "i/o timeout", Name: name, IsTimeout: true}
	}
	return []*net.MX{{Host: "mx." + name + ".", Pref: 10}}, nil
}

func (fakeMailServers) LookupHost(_ context.Context, host string) ([]string, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// fakeMailboxes answers mailbox checks without the network, by the address's
// local part: gone@ does not exist, busy@ is inconclusive (worth a recheck),
// queued@ finds the verifier busy the first time, any other is accepted. It
// keeps what it was asked.
type fakeMailboxes struct {
	mu    sync.Mutex
	calls []fakeMailboxCall
}

type fakeMailboxCall struct{ Email, Mode, ControlID string }

func (f *fakeMailboxes) Check(_ context.Context, email, mode, controlID string) (mailcheck.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeMailboxCall{email, mode, controlID})
	first := len(f.callsLocked(email)) == 1
	f.mu.Unlock()
	if strings.HasPrefix(email, "queued@") && first {
		return mailcheck.Result{}, mailcheck.ErrBusy
	}
	r := mailcheck.Result{Status: mailcheck.Accepted, Reason: "The server accepted this mailbox and rejected a made-up one.", Outcome: "recipient_supported",
		Provider: "Microsoft 365", Attempts: 1}
	switch {
	case strings.HasPrefix(email, "gone@"):
		r.Status, r.Outcome, r.Reason = mailcheck.Invalid, "recipient_missing", "The mail server explicitly reported that this mailbox does not exist."
	case strings.HasPrefix(email, "busy@"):
		r.Status, r.Outcome, r.Reason, r.Retryable = mailcheck.Unknown, "temporary", "The server temporarily deferred the mailbox check.", true
	}
	return r, nil
}

func (f *fakeMailboxes) callsFor(email string) []fakeMailboxCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callsLocked(email)
}

func (f *fakeMailboxes) callsLocked(email string) []fakeMailboxCall {
	var out []fakeMailboxCall
	for _, c := range f.calls {
		if c.Email == email {
			out = append(out, c)
		}
	}
	return out
}
