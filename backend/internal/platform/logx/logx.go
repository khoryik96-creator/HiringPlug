// Package logx sets up structured JSON logging and carries a request-scoped
// logger through context.Context, so every log line about a request or a job
// automatically includes request_id, tenant_id, user_id and job_id.
//
// Field names match the Python worker's logs, so one search covers both.
package logx

import (
	"context"
	"io"
	"log/slog"
)

// New returns the root logger for a service ("api", "worker", "hp").
func New(w io.Writer, service, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h).With("service", service)
}

type ctxKey struct{}

// With returns a context whose logger has the extra attributes.
func With(ctx context.Context, args ...any) context.Context {
	return context.WithValue(ctx, ctxKey{}, From(ctx).With(args...))
}

// Into stores logger in ctx (used once per request or job).
func Into(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// From returns the context's logger, or the default logger.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
