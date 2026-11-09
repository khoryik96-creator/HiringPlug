package candidates

import (
	"context"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// FindCandidates handles GET /v1/candidates/find.
func (h *HTTP) FindCandidates(ctx context.Context, req api.FindCandidatesRequestObject) (api.FindCandidatesResponseObject, error) {
	results, skipped, err := h.svc.Find(ctx, authz.MustFromContext(ctx), req.Params.Q)
	if err != nil {
		return nil, err
	}
	out := api.FindCandidates200JSONResponse{Searches: make([]api.CandidateFindSearch, len(results)), Skipped: skipped}
	for i, r := range results {
		out.Searches[i] = api.CandidateFindSearch{
			Text: r.Text, Kind: api.CandidateFindSearchKind(r.Kind),
			Company: orNull(nonEmpty(r.Company)), Error: orNull(nonEmpty(r.Error)), Note: orNull(nonEmpty(r.Note)),
			Items: foundToAPI(r.Items), Similar: foundToAPI(r.Similar), More: r.More,
		}
	}
	return out, nil
}

func foundToAPI(found []Found) []api.CandidateFound {
	out := make([]api.CandidateFound, len(found))
	for i, f := range found {
		out[i] = api.CandidateFound{
			Id: f.ID, FullName: f.FullName, CurrentTitle: orNull(f.CurrentTitle), CurrentCompany: orNull(f.CurrentCompany),
			Email: orNull(f.Email), Phone: orNull(f.Phone),
			Employer: nullable.NewNullNullable[api.CandidateFoundEmployer](), Score: nullable.NewNullNullable[float32](),
		}
		if f.Employer != nil {
			out[i].Employer = nullable.NewNullableWithValue(api.CandidateFoundEmployer{Company: f.Employer.Company, Title: orNull(f.Employer.Title), Current: f.Employer.Current})
		}
		if f.Score > 0 {
			out[i].Score = nullable.NewNullableWithValue(float32(f.Score))
		}
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
