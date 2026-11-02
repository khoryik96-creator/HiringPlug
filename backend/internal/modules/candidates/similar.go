package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Similar is a possible duplicate and why.
type Similar struct {
	Summary
	SamePhone, SimilarName bool
}

// Similar returns possible duplicates of a candidate: the same phone number,
// or a name at least 60% alike (ignoring case and accents), best first.
func (s *Service) Similar(ctx context.Context, p authz.Principal, id string) ([]Similar, error) {
	var out []Similar
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		c, err := q.GetCandidate(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("candidate")
		}
		if err != nil {
			return err
		}
		rows, err := q.SimilarCandidates(ctx, store.SimilarCandidatesParams{ID: id, Phone: c.Phone, FullName: c.FirstName + " " + c.LastName})
		if err != nil {
			return err
		}
		out = make([]Similar, len(rows))
		for i, r := range rows {
			out[i] = Similar{
				Summary: Summary{
					ID: r.ID, FullName: FullName(Profile{FirstName: r.FirstName, LastName: r.LastName}), Email: r.Email, Phone: r.Phone,
					CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany, Location: r.Location,
					WorkAuthorizationStatus: r.WorkAuthorization, UpdatedAt: r.UpdatedAt,
				},
				SamePhone: r.SamePhone, SimilarName: r.SimilarName,
			}
		}
		return nil
	})
	return out, err
}

// ListSimilarCandidates handles GET /v1/candidates/{candidateId}/similar.
func (h *HTTP) ListSimilarCandidates(ctx context.Context, req api.ListSimilarCandidatesRequestObject) (api.ListSimilarCandidatesResponseObject, error) {
	similar, err := h.svc.Similar(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ListSimilarCandidates200JSONResponse{Items: make([]api.SimilarCandidate, len(similar))}
	for i, s := range similar {
		reasons := []api.SimilarCandidateReasons{}
		if s.SamePhone {
			reasons = append(reasons, api.SimilarCandidateReasonsSamePhone)
		}
		if s.SimilarName {
			reasons = append(reasons, api.SimilarCandidateReasonsSimilarName)
		}
		out.Items[i] = api.SimilarCandidate{Candidate: summaryToAPI(s.Summary), Reasons: reasons}
	}
	return out, nil
}
