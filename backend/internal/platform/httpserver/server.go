// Package httpserver assembles the HTTP API: request IDs, access logs and
// panic recovery around everything; then, for /v1/*, the contract checks
// (route, sign-in, permission, input validation) before the generated
// handlers call into the modules.
package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Options are the parts the HTTP layer needs from the rest of the app.
type Options struct {
	Logger         *slog.Logger
	API            api.StrictServerInterface // every module's handlers, composed in internal/app
	Authenticate   Authenticator
	Files          http.Handler                    // signed file downloads for workers
	Ready          func(ctx context.Context) error // readiness: can we serve traffic?
	MaxBodyBytes   int64                           // JSON request limit
	MaxUploadBytes int64                           // file upload limit
	ClientIPHeader string                          // header the reverse proxy names the client in, or "" (see clientip)
}

// New returns the complete HTTP handler.
func New(o Options) (http.Handler, error) {
	// A private copy of the contract for routing: matching ignores `servers`.
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("httpserver: embedded contract: %w", err)
	}
	spec.Servers = nil
	router, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("httpserver: building router from contract: %w", err)
	}

	strict := api.NewStrictHandlerWithOptions(o.API, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			logx.From(r.Context()).Info("could not decode request", "error", err)
			WriteError(w, r, apperr.Validation(apperr.FieldErrors{{Field: "body", Message: "The request body could not be read."}}))
		},
		ResponseErrorHandlerFunc: WriteError,
	})
	apiHandler := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			WriteError(w, r, apperr.Validation(apperr.FieldErrors{{Field: "request", Message: err.Error()}}))
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := o.Ready(ctx); err != nil {
			logx.From(ctx).Error("not ready", "error", err)
			WriteError(w, r, apperr.New(api.ErrorCodeSERVICEUNAVAILABLE, "Not ready.").WithCause(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("GET "+blob.PathPrefix, o.Files)
	mux.HandleFunc("GET /openapi.json", serveContract)
	mux.HandleFunc("GET /docs", serveDocs)
	mux.Handle("/v1/", enforceContract(router, o.Authenticate, o.MaxBodyBytes, o.MaxUploadBytes, apiHandler))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, apperr.New(api.ErrorCodeROUTENOTFOUND, "No such endpoint."))
	})
	return observe(o.Logger, o.ClientIPHeader, mux), nil
}

func serveContract(w http.ResponseWriter, r *http.Request) {
	spec, err := api.Spec()
	if err != nil {
		WriteError(w, r, err)
		return
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// serveDocs renders the contract as browsable API documentation (Scalar).
// It loads Scalar's script from a CDN, so it needs internet access.
// The API reference's script: one fixed version, with its fingerprint, so
// the browser refuses anything else the CDN might serve under that address.
// To move to a newer version, change both (the hash is the sha384 of the
// file in the npm package: dist/browser/standalone.js).
const (
	docsScript     = "https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.76.0/dist/browser/standalone.js"
	docsScriptHash = "sha384-oH1C0RwqcwbVXkC+r8A+4qRoyEUQdtxAzOwumkxZtQBBcsy61FOoPt7B50OVJYYQ"
)

func serveDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head><title>HiringPlug API</title><meta charset="utf-8" /><meta name="viewport" content="width=device-width, initial-scale=1" /></head>
  <body>
    <script id="api-reference" data-url="/openapi.json"></script>
    <script src="` + docsScript + `" integrity="` + docsScriptHash + `" crossorigin="anonymous"></script>
  </body>
</html>
`))
}
