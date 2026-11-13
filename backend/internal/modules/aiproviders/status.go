package aiproviders

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/aiproviders/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// DownFor is how long a failure keeps the notice up when nothing has been
// tried since: after it, nobody knows whether the provider is back.
const DownFor = time.Hour

// Status is whether the AI a company uses is answering.
type Status struct {
	Down     bool
	Provider string
	Message  string
	Since    *time.Time // when the failures began
	FailedAt *time.Time // the latest one
}

// ProviderLabel is a provider's name as people know it.
func ProviderLabel(provider string) string {
	switch provider {
	case string(api.AiProviderNameDeepseek):
		return "DeepSeek"
	case string(api.AiProviderNameClaude):
		return "Claude"
	case string(api.AiProviderNameOpenai):
		return "ChatGPT"
	}
	return "The server's AI"
}

// IsDown reports whether the provider counts as down now: its latest call
// failed (no answer since), within DownFor.
func IsDown(lastOK, failedAt *time.Time, now time.Time) bool {
	if failedAt == nil || now.Sub(*failedAt) > DownFor {
		return false
	}
	return lastOK == nil || lastOK.Before(*failedAt)
}

// reported handles aiprovider.reported: the AI worker's word on whether the
// provider answered a job's last call.
func (s *Service) reported(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.AiProviderReportedJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	var problem *string
	if !r.Ok {
		m := "The provider did not answer."
		if v, err := r.Error.Get(); err == nil && v != "" {
			m = v
		}
		if rs := []rune(m); len(rs) > 500 {
			m = string(rs[:500])
		}
		problem = &m
	}
	return store.New(tx).ReportProvider(ctx, store.ReportProviderParams{Provider: r.Provider, Ok: r.Ok, At: r.At, Error: problem})
}

// Status tells whether the AI the company uses is answering.
func (s *Service) Status(ctx context.Context, p authz.Principal, now time.Time) (Status, error) {
	var out Status
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).ProviderStatus(ctx)
		if database.IsNoRows(err) {
			return nil // nothing tried yet
		}
		if err != nil {
			return err
		}
		if !IsDown(row.LastOkAt, row.FailedAt, now) {
			return nil
		}
		out = Status{Down: true, Provider: ProviderLabel(row.Provider), Since: row.FailingSince, FailedAt: row.FailedAt}
		if row.Error != nil {
			out.Message = *row.Error
		}
		return nil
	})
	return out, err
}

// GetAiStatus handles GET /v1/ai-status.
func (h *HTTP) GetAiStatus(ctx context.Context, _ api.GetAiStatusRequestObject) (api.GetAiStatusResponseObject, error) {
	st, err := h.svc.Status(ctx, authz.MustFromContext(ctx), time.Now())
	if err != nil {
		return nil, err
	}
	out := api.GetAiStatus200JSONResponse{
		Down: st.Down, Provider: nullable.NewNullNullable[string](), Message: nullable.NewNullNullable[string](),
		Since: nullable.NewNullNullable[time.Time](), FailedAt: nullable.NewNullNullable[time.Time](),
	}
	if st.Down {
		out.Provider, out.Message = nullable.NewNullableWithValue(st.Provider), nullable.NewNullableWithValue(st.Message)
		if st.Since != nil {
			out.Since = nullable.NewNullableWithValue(*st.Since)
		}
		if st.FailedAt != nil {
			out.FailedAt = nullable.NewNullableWithValue(*st.FailedAt)
		}
	}
	return out, nil
}
