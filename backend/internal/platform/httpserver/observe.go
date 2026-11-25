package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/requestid"
)

// observe is the outermost middleware. For every request it:
//   - assigns a request ID (or keeps a valid incoming X-Request-Id) and returns it;
//   - puts a logger carrying that ID into the context;
//   - turns a panic into a 500 response with the ID, logging the stack;
//   - writes one access-log line with status, duration, tenant and user,
//     and the path with any secret in it left out (secretPaths).
func observe(logger *slog.Logger, clientIPHeader string, secrets secretPaths, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := requestid.Sanitize(r.Header.Get(requestid.Header))
		if id == "" {
			id = requestid.New()
		}
		w.Header().Set(requestid.Header, id)

		info := &requestInfo{}
		ctx := requestid.With(r.Context(), id)
		ctx = clientip.With(ctx, clientip.Of(r, clientIPHeader))
		ctx = context.WithValue(ctx, requestInfoKey{}, info)
		ctx = logx.Into(ctx, logger.With("request_id", id))
		r = r.WithContext(ctx)
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if v := recover(); v != nil {
				logx.From(ctx).Error("panic while handling request", "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
				if !rec.wroteHeader {
					WriteError(rec, r, fmt.Errorf("panic: %v", v))
				}
			}
			if strings.HasPrefix(r.URL.Path, "/health/") {
				return // load balancer probes would drown the log
			}
			attrs := []any{
				"method", r.Method, "path", secrets.redact(r.URL.Path), "status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(), "bytes", rec.bytes,
			}
			if info.tenantID != "" {
				attrs = append(attrs, "tenant_id", info.tenantID, "user_id", info.userID)
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			logx.From(ctx).Log(ctx, level, "http request", attrs...)
		}()
		next.ServeHTTP(rec, r)
	})
}

// requestInfo lets inner middleware (auth) report who the caller was to the
// access log written by observe.
type requestInfo struct{ tenantID, userID string }

type requestInfoKey struct{}

func setCaller(ctx context.Context, tenantID, userID string) {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		info.tenantID, info.userID = tenantID, userID
	}
}

type recorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *recorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status, r.wroteHeader = status, true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the real writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// secretPaths are the contract's paths that carry a secret (a parameter
// named ...Token: an invitation or reset link, a booking or shortlist
// link), each up to its last secret: whoever reads the logs must not be able
// to use them. Any path starting like one (whatever method, or even no
// route) is logged with the secret replaced by the parameter's name.
type secretPaths [][]string

func secretPathsOf(paths []string) secretPaths {
	var out secretPaths
	for _, p := range paths {
		segments := strings.Split(p, "/")
		last := -1
		for i, s := range segments {
			if isSecretParam(s) {
				last = i
			}
		}
		if last >= 0 {
			out = append(out, segments[:last+1])
		}
	}
	return out
}

func isSecretParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(strings.ToLower(segment), "token}")
}

func (s secretPaths) redact(path string) string {
	segments := strings.Split(path, "/")
	for _, route := range s {
		if len(segments) < len(route) {
			continue
		}
		matches := true
		for i, r := range route {
			if !strings.HasPrefix(r, "{") && r != segments[i] {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		out := append([]string(nil), segments...)
		for i, r := range route {
			if isSecretParam(r) {
				out[i] = r
			}
		}
		return strings.Join(out, "/")
	}
	return path
}
