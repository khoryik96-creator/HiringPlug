// Package app starts HiringPlug: it reads the configuration, connects to the
// database, wires the modules, and runs either the HTTP API or the worker.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/db"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/httpserver"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
)

// maxJSONBodyBytes limits JSON request bodies (uploads have their own limit).
const maxJSONBodyBytes = 1 << 20

// Deps are the shared building blocks every module is made from.
type Deps struct {
	Config   config.Config
	Logger   *slog.Logger
	Pool     *pgxpool.Pool
	Verifier *authn.Verifier
	Files    *blob.Store
	// MailServers looks up where email domains receive mail (contacts' email
	// check). Tests replace it with a fake that needs no network.
	MailServers contacts.MailServers
	// Mailboxes asks mail servers whether a mailbox exists (contacts' email
	// verifier, port 25). Tests replace it with a fake that needs no network.
	Mailboxes contacts.Mailboxes
	// Mail sends email from the app (the configured SMTP server).
	Mail mail.Sender
}

// newMailSender returns the sender for the configured mail server, or one that
// fails every email with the reason when none is set up.
func newMailSender(c config.SMTP) mail.Sender {
	if c.Addr == "" {
		return mail.Unconfigured{}
	}
	return mail.NewSMTP(mail.Config{Addr: c.Addr, Username: c.Username, Password: c.Password, From: c.From, TLS: c.TLS})
}

// NewDeps connects to the database and refuses to continue if it is unsafe or
// out of date: connected as a role that bypasses tenant isolation, or behind
// the code's migrations.
func NewDeps(ctx context.Context, cfg config.Config, logger *slog.Logger) (Deps, func(), error) {
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return Deps{}, nil, err
	}
	fail := func(err error) (Deps, func(), error) {
		pool.Close()
		return Deps{}, nil, err
	}
	if err := database.CheckRestrictedRole(ctx, pool); err != nil {
		return fail(err)
	}
	pending, err := database.PendingMigrations(ctx, cfg.DatabaseURL, db.Migrations())
	if err != nil {
		return fail(err)
	}
	if pending > 0 {
		return fail(fmt.Errorf("the database is %d migration(s) behind this code; run `hp migrate` first", pending))
	}
	files, err := blob.New(cfg.FilesDir, cfg.FilesURLSecret, cfg.InternalBaseURL)
	if err != nil {
		return fail(err)
	}
	return Deps{
		Config:   cfg,
		Logger:   logger,
		Pool:     pool,
		Verifier: authn.NewVerifier(cfg.AuthJWTSecret, cfg.AuthIssuer, cfg.AuthAudience),
		Files:    files,

		MailServers: net.DefaultResolver,
		Mailboxes:   mailcheck.New(),
		Mail:        newMailSender(cfg.SMTP),
	}, pool.Close, nil
}

// NewHTTPHandler builds the complete HTTP API.
func NewHTTPHandler(d Deps, m *Modules) (http.Handler, error) {
	return httpserver.New(httpserver.Options{
		Logger:         d.Logger,
		API:            m.Server(),
		Authenticate:   m.authenticate,
		Files:          d.Files,
		Ready:          func(ctx context.Context) error { return d.Pool.Ping(ctx) },
		MaxBodyBytes:   maxJSONBodyBytes,
		MaxUploadBytes: d.Config.MaxUploadBytes,
		ClientIPHeader: d.Config.ClientIPHeader,
	})
}

// NewWorker builds the Go worker for the core job queue.
func NewWorker(d Deps, m *Modules) (*jobqueue.Worker, error) {
	handlers, err := m.JobHandlers()
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	return &jobqueue.Worker{
		Pool:     d.Pool,
		Queue:    jobqueue.QueueCore,
		ID:       fmt.Sprintf("go:%s:%d", host, os.Getpid()),
		Handlers: handlers,
		Logger:   d.Logger,
		Lease:    5 * time.Minute,
		Idle:     2 * time.Second,
		// Tests run the worker step by step (RunOnce); Run uses this.
		Concurrency: d.Config.WorkerConcurrency,
	}, nil
}

// RunAPI serves HTTP until ctx is cancelled, then shuts down gracefully.
func RunAPI(ctx context.Context, getenv func(string) string, out io.Writer) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	logger := logx.New(out, "api", cfg.LogFormat, cfg.LogLevel)
	d, closeDeps, err := NewDeps(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeDeps()
	handler, err := NewHTTPHandler(d, NewModules(d))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		serveErr <- srv.ListenAndServe()
	}()
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("api shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// RunWorker processes core-queue jobs until ctx is cancelled.
func RunWorker(ctx context.Context, getenv func(string) string, out io.Writer) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	logger := logx.New(out, "worker", cfg.LogFormat, cfg.LogLevel)
	d, closeDeps, err := NewDeps(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeDeps()
	w, err := NewWorker(d, NewModules(d))
	if err != nil {
		return err
	}
	go pruneJobs(ctx, d)
	return w.Run(ctx)
}

// pruneJobs deletes finished and dead jobs after jobqueue.KeepFinished, every
// hour (several workers doing it at once is harmless).
func pruneJobs(ctx context.Context, d Deps) {
	for {
		if n, err := jobqueue.Prune(ctx, d.Pool, jobqueue.KeepFinished); err != nil {
			if ctx.Err() == nil {
				d.Logger.Warn("old jobs not pruned", "error", err)
			}
		} else if n > 0 {
			d.Logger.Info("old jobs pruned", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

// authenticate signs a request in: with an API key (integrations), or with a
// user's sign-in token (identity).
func (m *Modules) authenticate(ctx context.Context, token string) (authz.Principal, error) {
	if integrations.IsAPIKey(token) {
		return m.Integrations.Authenticate(ctx, token)
	}
	return m.Identity.Authenticate(ctx, token)
}
