package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var assets embed.FS

type App struct {
	connecting              bool
	transferring            bool
	tickets                 map[string]DownloadTicket
	closing                 bool
	work                    sync.WaitGroup
	mu                      sync.Mutex
	store                   *Store
	origin, session         string
	demo, unlocked, running bool
	connection              Connection
	clientHTTP              *http.Client
	pace                    time.Duration
	cancel                  context.CancelFunc
	current, message        string
	mode                    string
	oauthState              string
	oauthUntil              time.Time
	pending                 Connection
}

func randomID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b)
}
func (a *App) client(c Connection) *Client {
	return &Client{Config: c, HTTP: a.clientHTTP, Wait: wait, Pace: a.pace, LoadRetry: a.store.RetryUntil, SaveRetry: a.store.SetRetryUntil, Save: func(next Connection) error {
		if err := a.store.SaveConfig(next); err != nil {
			return err
		}
		a.mu.Lock()
		a.connection = next
		a.mu.Unlock()
		return nil
	}}
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/extract", a.startExtraction)
	mux.HandleFunc("GET /api/extraction", a.extractionReport)
	mux.HandleFunc("POST /api/sources", a.sourceUpload)
	mux.HandleFunc("GET /api/downloads", a.downloadPlan)
	mux.HandleFunc("POST /api/download-ticket", a.downloadTicket)
	mux.HandleFunc("GET /download/{ticket}", a.downloadExtraction)
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("POST /api/unlock", a.unlock)
	mux.HandleFunc("POST /api/connect", a.connect)
	mux.HandleFunc("POST /api/oauth", a.authorize)
	mux.HandleFunc("POST /api/start", a.start)
	mux.HandleFunc("POST /api/pause", a.pause)
	mux.HandleFunc("GET /api/report", a.report)
	mux.HandleFunc("GET /api/originals.zip", a.originals)
	mux.HandleFunc("GET /oauth/callback", a.callback)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := "web/index.html"
		kind := "text/html; charset=utf-8"
		switch r.URL.Path {
		case "/":
		case "/app.js":
			name = "web/app.js"
			kind = "text/javascript; charset=utf-8"
		case "/extraction.js":
			name = "web/extraction.js"
			kind = "text/javascript; charset=utf-8"
		case "/app.css":
			name = "web/app.css"
			kind = "text/css; charset=utf-8"
		default:
			http.NotFound(w, r)
			return
		}
		body, err := assets.ReadFile(name)
		if err != nil {
			http.Error(w, "Not available", 500)
			return
		}
		w.Header().Set("Content-Type", kind)
		_, _ = w.Write(body)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		if a.closing {
			a.mu.Unlock()
			http.Error(w, "Application is closing", http.StatusServiceUnavailable)
			return
		}
		a.work.Add(1)
		a.mu.Unlock()
		defer a.work.Done()
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		origin, _ := url.Parse(a.origin)
		if r.Host != origin.Host && r.Host != strings.Replace(origin.Host, "localhost", "127.0.0.1", 1) {
			http.Error(w, "Host refused", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Migration-Session")), []byte(a.session)) != 1 {
				http.Error(w, "Open the launch shortcut from this workspace", 401)
				return
			}
			if o := r.Header.Get("Origin"); o != "" && o != a.origin {
				http.Error(w, "Origin refused", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, message string) {
	reply(w, 400, map[string]string{"error": message})
}
func input(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		failure(w, "Invalid request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		failure(w, "Invalid request")
		return false
	}
	return true
}
func (a *App) statusData() (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	data := map[string]any{"demo": a.demo, "unlocked": a.unlocked, "connected": a.connection.Fingerprint != "", "running": a.running, "connecting": a.connecting, "transferring": a.transferring, "current": a.current, "mode": a.mode, "message": a.message, "resources": len(Catalog()), "account": a.connection.Account, "callback": a.origin + "/oauth/callback"}
	if a.unlocked {
		report, err := a.store.Report()
		if err != nil {
			return nil, err
		}
		until, err := a.store.RetryUntil()
		if err != nil {
			return nil, err
		}
		if report.Status == "running" && !a.running {
			report.Status = "paused"
		}
		data["report"] = report
		if time.Now().Before(until) {
			data["retryAt"] = until.UTC().Format(time.RFC3339Nano)
		}
	}
	return data, nil
}
func workspaceFailure(w http.ResponseWriter) {
	reply(w, http.StatusServiceUnavailable, map[string]string{"error": "Workspace progress could not be read. Saved results may be stale; check the workspace before continuing."})
}
func (a *App) status(w http.ResponseWriter, _ *http.Request) {
	data, err := a.statusData()
	if err != nil {
		workspaceFailure(w)
		return
	}
	reply(w, http.StatusOK, data)
}
func (a *App) unlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Passphrase string `json:"passphrase"`
	}
	if !input(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.unlocked {
		failure(w, "Workspace is already unlocked")
		return
	}
	if err := a.store.Unlock(in.Passphrase); err != nil {
		failure(w, err.Error())
		return
	}
	c, err := a.store.Config()
	if err != nil {
		failure(w, "Encrypted connection could not be opened")
		return
	}
	a.connection = c
	a.unlocked = true
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) available() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.transferring || a.demo || a.connecting || a.closing {
		return false
	}
	a.connecting = true
	return true
}
func (a *App) connectionDone() { a.mu.Lock(); a.connecting = false; a.mu.Unlock() }

func (a *App) accept(ctx context.Context, c Connection) error {
	// A new connection is untrusted until its identity matches the workspace.
	// Refreshes during this check must not overwrite an existing connection.
	client := &Client{Config: c, HTTP: a.clientHTTP, Wait: wait, Pace: a.pace, LoadRetry: a.store.RetryUntil, SaveRetry: a.store.SetRetryUntil}
	identified, err := identify(ctx, client)
	if err != nil {
		return err
	}
	old, err := a.store.Config()
	if err != nil {
		return errors.New("saved connection could not be checked")
	}
	if old.Fingerprint != "" && old.Fingerprint != identified.Fingerprint {
		return errors.New("this workspace belongs to another account or API user; choose a separate workspace")
	}
	if err = a.store.SaveConfig(identified); err != nil {
		return errors.New("encrypted connection could not be saved")
	}
	a.mu.Lock()
	a.connection = identified
	a.message = "JobAdder connected. No source records changed."
	a.mu.Unlock()
	return nil
}
func (a *App) connect(w http.ResponseWriter, r *http.Request) {
	if !a.available() {
		failure(w, "Unlock the workspace and pause any inventory first")
		return
	}
	defer a.connectionDone()
	var in struct {
		API   string `json:"api"`
		Token string `json:"token"`
	}
	if !input(w, r, &in) {
		return
	}
	if validateAPI(in.API) != nil || in.Token == "" {
		failure(w, "Enter the JobAdder API address and an access token")
		return
	}
	if err := a.accept(r.Context(), Connection{API: strings.TrimSuffix(in.API, "/"), AccessToken: in.Token}); err != nil {
		failure(w, err.Error())
		return
	}
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) start(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Resume bool `json:"resume"`
	}
	if !input(w, r, &in) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.unlocked || a.running || a.transferring || a.connecting || a.closing || a.connection.Fingerprint == "" {
		failure(w, "Connect JobAdder before starting an inventory")
		return
	}
	if _, err := a.store.Report(); err != nil {
		workspaceFailure(w)
		return
	}
	until, err := a.store.RetryUntil()
	if err != nil {
		workspaceFailure(w)
		return
	}
	if time.Now().Before(until) {
		reply(w, http.StatusTooManyRequests, map[string]string{"error": (&CooldownError{Until: until}).Error()})
		return
	}
	run, err := a.store.Start(a.connection.Fingerprint, in.Resume)
	if err != nil {
		failure(w, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = true
	a.message = ""
	a.current = "Starting"
	a.mode = "inventory"
	c := a.client(a.connection)
	a.work.Add(1)
	go func() {
		defer a.work.Done()
		err := discover(ctx, a.store, c, run, Catalog(), in.Resume, func(label string) { a.mu.Lock(); a.current = label; a.mu.Unlock() })
		status, message := "finished", "Inventory finished. Review all coverage gaps before planning a full export."
		if err != nil {
			status = "paused"
			message = err.Error()
			if errors.Is(err, context.Canceled) {
				message = "Paused. Completed checkpoints are saved."
			}
		}
		if e := a.store.Finish(run, status); e != nil {
			message = "Inventory stopped; final checkpoint could not be saved."
		}
		a.mu.Lock()
		a.running = false
		a.current = ""
		a.message = message
		a.mu.Unlock()
	}()
	reply(w, 202, map[string]bool{"ok": true})
}
func (a *App) pause(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) report(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	if !a.unlocked {
		a.mu.Unlock()
		failure(w, "Unlock the workspace")
		return
	}
	r, err := a.store.Report()
	running := a.running
	a.mu.Unlock()
	if err != nil {
		failure(w, "Report could not be read")
		return
	}
	if r.Status == "running" && !running {
		r.Status = "paused"
	}
	w.Header().Set("Content-Disposition", `attachment; filename="jobadder-discovery-report.json"`)
	reply(w, 200, map[string]any{"demo": a.demo, "report": r})
}
