package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/requestid"
)

// WriteError sends err as the contract's Error body. Expected errors
// (*apperr.Error) keep their code and message; anything else is logged with
// its cause and becomes a generic 500 INTERNAL, so internals never leak.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return // the client went away; nobody is listening
	}
	appErr, ok := apperr.As(err)
	var tooLarge *http.MaxBytesError
	switch {
	case ok:
	case errors.As(err, &tooLarge):
		appErr = apperr.New(api.ErrorCodePAYLOADTOOLARGE, "The request is too large.")
	default:
		appErr = apperr.New(api.ErrorCodeINTERNAL, "Something went wrong on our side. Quote the request ID when reporting it.").WithCause(err)
	}

	status := apperr.Status(appErr.Code)
	log := logx.From(ctx)
	if status >= 500 {
		log.Error("request failed", "error_code", appErr.Code, "error", err)
	} else {
		log.Info("request rejected", "error_code", appErr.Code, "message", appErr.Message)
	}

	var body api.Error
	body.Error.Code = appErr.Code
	body.Error.Message = appErr.Message
	body.Error.RequestId = requestid.From(ctx)
	if len(appErr.Details) > 0 {
		details := appErr.Details
		body.Error.Details = &details
	}
	writeJSON(ctx, w, status, body)
}

func writeJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logx.From(ctx).Log(ctx, slog.LevelWarn, "writing response failed", "error", err)
	}
}
