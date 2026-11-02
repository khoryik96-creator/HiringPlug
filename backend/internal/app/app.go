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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/config"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/httpserver"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
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
	}, pool.Close, nil
}

// NewHTTPHandler builds the complete HTTP API.
func NewHTTPHandler(d Deps, m *Modules) (http.Handler, error) {
	return httpserver.New(httpserver.Options{
		Logger:         d.Logger,
		API:            m.Server(),
		Authenticate:   m.Identity.Authenticate,
		Files:          d.Files,
		Ready:          func(ctx context.Context) error { return d.Pool.Ping(ctx) },
		MaxBodyBytes:   maxJSONBodyBytes,
		MaxUploadBytes: d.Config.MaxUploadBytes,
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
	return w.Run(ctx)
}
