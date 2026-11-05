package pipeline

// The timeline of stage moves: the dashboard's, and each candidate's.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Users is what the timeline needs from the identity module.
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// NameUsersWith sets the identity module (called by internal/app).
func (m *Module) NameUsersWith(u Users) { m.service.users = u }

// StageMove is an application added (From nil) or moved to a stage.
type StageMove struct {
	ID, ApplicationID, CandidateID, CandidateName, JobID, JobTitle, CompanyName string
	From                                                                        *string
	To                                                                          string
	At                                                                          time.Time
	ByName                                                                      *string // nil when the app moved it
}

// Moves returns the latest stage moves of applications the user may see,
// newest first; only one candidate's when candidateID is set.
func (s *Service) Moves(ctx context.Context, p authz.Principal, candidateID *string, limit int) ([]StageMove, error) {
	var out []StageMove
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).StageMoves(ctx, store.StageMovesParams{CandidateID: candidateID, MaxRows: int32(limit)}) //nolint:gosec // 1..100 per the contract
		if err != nil {
			return err
		}
		apps := make([]store.Application, len(rows))
		var userIDs []string
		for i, r := range rows {
			apps[i] = store.Application{ID: r.ApplicationID, CandidateID: r.CandidateID, JobOrderID: r.JobOrderID}
			if r.ChangedBy != nil {
				userIDs = append(userIDs, *r.ChangedBy)
			}
		}
		views, err := s.views(ctx, tx, apps)
		if err != nil {
			return err
		}
		names := map[string]string{}
		if s.users != nil && len(userIDs) > 0 {
			if names, err = s.users.UserNames(ctx, tx, userIDs); err != nil {
				return err
			}
		}
		out = make([]StageMove, len(rows))
		for i, r := range rows {
			v := views[i]
			out[i] = StageMove{
				ID: r.ID, ApplicationID: r.ApplicationID, CandidateID: r.CandidateID, CandidateName: v.CandidateName,
				JobID: r.JobOrderID, JobTitle: v.JobTitle, CompanyName: v.CompanyName, From: r.FromStage, To: r.ToStage, At: r.ChangedAt,
			}
			if r.ChangedBy != nil {
				if n, ok := names[*r.ChangedBy]; ok {
					out[i].ByName = &n
				}
			}
		}
		return nil
	})
	return out, err
}

// ListStageMoves handles GET /v1/pipeline/moves.
func (h *HTTP) ListStageMoves(ctx context.Context, req api.ListStageMovesRequestObject) (api.ListStageMovesResponseObject, error) {
	limit := 30
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	moves, err := h.svc.Moves(ctx, authz.MustFromContext(ctx), req.Params.CandidateId, limit)
	if err != nil {
		return nil, err
	}
	out := api.StageMoveList{Items: make([]api.StageMove, len(moves))}
	for i, m := range moves {
		item := api.StageMove{
			Id: m.ID, ApplicationId: m.ApplicationID, CandidateId: m.CandidateID, CandidateName: m.CandidateName,
			JobId: m.JobID, JobTitle: m.JobTitle, CompanyName: m.CompanyName, ToStage: api.Stage(m.To),
			ChangedAt: m.At, ChangedByName: c.OrNull(m.ByName),
		}
		item.FromStage = c.OrNull(c.Convert[api.Stage](m.From))
		out.Items[i] = item
	}
	return api.ListStageMoves200JSONResponse(out), nil
}
