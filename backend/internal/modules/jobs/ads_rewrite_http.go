package jobs

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// RewriteBlindAd handles POST /v1/job-ads/{adId}/blind-rewrites.
func (h *HTTP) RewriteBlindAd(ctx context.Context, req api.RewriteBlindAdRequestObject) (api.RewriteBlindAdResponseObject, error) {
	b := req.Body
	run, err := h.svc.StartBlindRewrite(ctx, authz.MustFromContext(ctx), req.AdId, RewriteOptions{
		Tone: string(b.Tone), Strength: string(b.Strength),
		RemoveAlso: c.ValueOrNil(b.RemoveAlso), WhyJoinUs: c.ValueOrNil(b.WhyJoinUs), Context: c.ValueOrNil(b.Context),
	})
	if err != nil {
		return nil, err
	}
	return api.RewriteBlindAd202JSONResponse(rewriteToAPI(run)), nil
}

// ListBlindRewrites handles GET /v1/job-ads/{adId}/blind-rewrites.
func (h *HTTP) ListBlindRewrites(ctx context.Context, req api.ListBlindRewritesRequestObject) (api.ListBlindRewritesResponseObject, error) {
	runs, err := h.svc.ListBlindRewrites(ctx, authz.MustFromContext(ctx), req.AdId)
	if err != nil {
		return nil, err
	}
	out := api.ListBlindRewrites200JSONResponse{Items: []api.BlindRewrite{}}
	for _, r := range runs {
		out.Items = append(out.Items, rewriteToAPI(r))
	}
	return out, nil
}

func rewriteToAPI(r BlindRewrite) api.BlindRewrite {
	warnings := r.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return api.BlindRewrite{
		Id: r.ID, AdId: r.AdID, Status: api.BlindRewriteStatus(r.Status), Tone: api.BlindRewriteTone(r.Tone),
		Strength: api.BlindRewriteStrength(r.Strength), Error: c.OrNull(r.Error), Warnings: warnings, Retried: r.Retried,
		Model: c.OrNull(r.Model), Tokens: int(r.Tokens), CreatedAt: r.CreatedAt, FinishedAt: c.OrNull(r.FinishedAt),
	}
}
