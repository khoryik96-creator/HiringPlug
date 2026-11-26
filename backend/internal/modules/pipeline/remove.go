package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Remove takes a candidate off a job they were put on by mistake: the
// application and its stage history go, never one carrying an interview, an
// offer, a placement or a client link (those are withdrawn instead).
// expectedVersion must be the version the user loaded.
func (s *Service) Remove(ctx context.Context, p authz.Principal, id string, expectedVersion int) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetApplication(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("application")
		}
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("application").WithDetail("currentVersion", row.Version)
		}
		for _, check := range s.attached {
			has, err := check(ctx, tx, []string{id})
			if err != nil {
				return err
			}
			if has[id] {
				return apperr.New(api.ErrorCodeAPPLICATIONNOTREMOVABLE,
					"This application has an interview, an offer, a placement or a client link: move it to Withdrawn instead.")
			}
		}
		if _, err := q.DeleteApplications(ctx, []string{id}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "application.removed", EntityType: "application", EntityID: id,
			Changes: map[string]any{"jobId": row.JobOrderID, "candidateId": row.CandidateID, "stage": row.Stage},
		})
	})
}

// RemoveApplication handles DELETE /v1/applications/{applicationId}.
func (h *HTTP) RemoveApplication(ctx context.Context, req api.RemoveApplicationRequestObject) (api.RemoveApplicationResponseObject, error) {
	if err := h.svc.Remove(ctx, authz.MustFromContext(ctx), req.ApplicationId, req.Params.Version); err != nil {
		return nil, err
	}
	return api.RemoveApplication204Response{}, nil
}
