package matching

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// ListMatches handles GET /v1/matches.
func (h *HTTP) ListMatches(ctx context.Context, req api.ListMatchesRequestObject) (api.ListMatchesResponseObject, error) {
	items, err := h.svc.List(ctx, authz.MustFromContext(ctx), deref(req.Params.JobId), deref(req.Params.ApplicationId))
	if err != nil {
		return nil, err
	}
	on, err := h.svc.Enabled(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListMatches200JSONResponse{Enabled: on, Items: make([]api.Match, len(items))}
	for i, m := range items {
		out.Items[i] = toAPI(m)
	}
	return out, nil
}

// ScoreApplication handles POST /v1/matches.
func (h *HTTP) ScoreApplication(ctx context.Context, req api.ScoreApplicationRequestObject) (api.ScoreApplicationResponseObject, error) {
	m, err := h.svc.Score(ctx, authz.MustFromContext(ctx), req.Body.ApplicationId)
	if err != nil {
		return nil, err
	}
	return api.ScoreApplication202JSONResponse(toAPI(m)), nil
}

// ScoreJobApplications handles POST /v1/matches/for-job.
func (h *HTTP) ScoreJobApplications(ctx context.Context, req api.ScoreJobApplicationsRequestObject) (api.ScoreJobApplicationsResponseObject, error) {
	rescore := req.Body.Rescore != nil && *req.Body.Rescore
	res, err := h.svc.ScoreJob(ctx, authz.MustFromContext(ctx), req.Body.JobId, rescore)
	if err != nil {
		return nil, err
	}
	return api.ScoreJobApplications202JSONResponse{Queued: res.Queued, AlreadyScored: res.AlreadyScored, WithoutCv: res.WithoutCV, ConsentWithdrawn: res.ConsentWithdrawn}, nil
}

// SuggestCandidates handles GET /v1/matches/suggestions.
func (h *HTTP) SuggestCandidates(ctx context.Context, req api.SuggestCandidatesRequestObject) (api.SuggestCandidatesResponseObject, error) {
	found, err := h.svc.Suggest(ctx, authz.MustFromContext(ctx), req.Params.JobId)
	if err != nil {
		return nil, err
	}
	out := api.SuggestCandidates200JSONResponse{Items: make([]api.MatchSuggestion, len(found))}
	for i, s := range found {
		out.Items[i] = api.MatchSuggestion{
			CandidateId: s.CandidateID, CandidateName: s.Name, CurrentTitle: apiconv.OrNull(s.Title), Rank: float32(s.Rank), Snippet: s.Snippet,
		}
	}
	return out, nil
}

func toAPI(m Match) api.Match {
	reasons := make([]api.MatchReason, len(m.Reasons))
	for i, r := range m.Reasons {
		reasons[i] = api.MatchReason{
			Requirement: r.Requirement, Verdict: api.MatchVerdict(r.Verdict), Evidence: apiconv.OrNull(r.Evidence), Note: apiconv.OrNull(r.Note),
		}
	}
	dims := make([]api.MatchDimension, len(m.Dimensions))
	for i, d := range m.Dimensions {
		dims[i] = api.MatchDimension{Name: api.MatchDimensionName(d.Name), Score: d.Score, Note: apiconv.OrNull(d.Note)}
	}
	return api.Match{
		Dimensions: dims, Strengths: nonNil(m.Strengths), Gaps: nonNil(m.Gaps),
		Recommendation: apiconv.OrNull(apiconv.Convert[api.MatchRecommendation](Recommendation(m.Score))),
		Id:             m.ID, ApplicationId: m.ApplicationID, JobId: m.JobID, Status: api.MatchStatus(m.Status), Score: apiconv.OrNull(m.Score),
		Summary: apiconv.OrNull(m.Summary), Reasons: reasons, Dropped: m.Dropped, Outdated: m.Outdated, Error: apiconv.OrNull(m.Error),
		Model: apiconv.OrNull(m.Model), CreatedAt: m.CreatedAt, ScoredAt: apiconv.OrNull(m.ScoredAt),
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
