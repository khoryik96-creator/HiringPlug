package meaning

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// HTTP holds the module's HTTP handlers.
type HTTP struct{ svc *Service }

// Status says whether search by meaning is ready and how far its reading of
// the company's candidates has got (and starts that reading if it stopped).
func (s *Service) Status(ctx context.Context, p authz.Principal) (api.MeaningStatus, error) {
	var out api.MeaningStatus
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		sweep, err := s.ensureRunning(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		var outcome api.MeaningOutcome
		if err := s.progress(ctx, tx, &outcome, sweep); err != nil {
			return err
		}
		out = api.MeaningStatus{
			Available: sweep.UnavailableUntil == nil || !sweep.UnavailableUntil.After(s.now()),
			Read:      *outcome.Read, Total: *outcome.Total,
		}
		return nil
	})
	return out, err
}

// GetMeaningStatus handles GET /v1/meaning/status.
func (h *HTTP) GetMeaningStatus(ctx context.Context, _ api.GetMeaningStatusRequestObject) (api.GetMeaningStatusResponseObject, error) {
	out, err := h.svc.Status(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMeaningStatus200JSONResponse(out), nil
}
