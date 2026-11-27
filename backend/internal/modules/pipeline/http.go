package pipeline

import (
	"context"
	"slices"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the pipeline operations of the contract.
type HTTP struct{ svc *Service }

// ListJobApplications handles GET /v1/jobs/{jobId}/applications.
func (h *HTTP) ListJobApplications(ctx context.Context, req api.ListJobApplicationsRequestObject) (api.ListJobApplicationsResponseObject, error) {
	views, next, counts, err := h.svc.ForJob(ctx, authz.MustFromContext(ctx), req.JobId, req.Params.Cursor, c.Convert[string](req.Params.Stage), applicationPageSize(req.Params.Limit, 1000), req.Params.IncludeCounts != nil && *req.Params.IncludeCounts)
	if err != nil {
		return nil, err
	}
	return api.ListJobApplications200JSONResponse{Items: toAPIList(views), NextCursor: next, Counts: counts}, nil
}

// AddApplication handles POST /v1/jobs/{jobId}/applications.
func (h *HTTP) AddApplication(ctx context.Context, req api.AddApplicationRequestObject) (api.AddApplicationResponseObject, error) {
	stage := ""
	if req.Body.Stage != nil {
		stage = string(*req.Body.Stage)
	}
	view, err := h.svc.Add(ctx, authz.MustFromContext(ctx), req.JobId, req.Body.CandidateId, stage)
	if err != nil {
		return nil, err
	}
	return api.AddApplication201JSONResponse(toAPI(view)), nil
}

// ListCandidateApplications handles GET /v1/candidates/{candidateId}/applications.
func (h *HTTP) ListCandidateApplications(ctx context.Context, req api.ListCandidateApplicationsRequestObject) (api.ListCandidateApplicationsResponseObject, error) {
	views, next, err := h.svc.ForCandidate(ctx, authz.MustFromContext(ctx), req.CandidateId, req.Params.Cursor, applicationPageSize(req.Params.Limit, 200))
	if err != nil {
		return nil, err
	}
	return api.ListCandidateApplications200JSONResponse{Items: toAPIList(views), NextCursor: next}, nil
}

// MoveApplication handles POST /v1/applications/{applicationId}/move.
func (h *HTTP) MoveApplication(ctx context.Context, req api.MoveApplicationRequestObject) (api.MoveApplicationResponseObject, error) {
	b := req.Body
	view, err := h.svc.Move(ctx, authz.MustFromContext(ctx), req.ApplicationId, b.Version, Move{
		To: string(b.Stage), Step: b.Step, Reason: c.Convert[string](b.Reason), Note: b.ReasonNote,
	})
	if err != nil {
		return nil, err
	}
	return api.MoveApplication200JSONResponse(toAPI(view)), nil
}

func applicationPageSize(limit *int, fallback int32) int32 {
	if limit == nil || *limit < 1 || *limit > int(fallback) {
		return fallback
	}
	return int32(*limit) //nolint:gosec // bounded above
}

func toAPIList(views []View) []api.Application {
	out := make([]api.Application, len(views))
	for i, v := range views {
		out[i] = toAPI(v)
	}
	return out
}

func toAPI(v View) api.Application {
	return api.Application{
		Id: v.ID, Version: v.Version, JobId: v.JobID, JobTitle: v.JobTitle, CompanyId: v.CompanyID, CompanyName: v.CompanyName,
		CandidateId: v.CandidateID, CandidateName: v.CandidateName, CandidateTitle: c.OrNull(v.CandidateTitle),
		CandidateAvailability: c.OrNull(c.Convert[api.CandidateAvailability](v.CandidateAvailability)),
		Stage:                 api.Stage(v.Stage), Step: c.OrNull(v.Step), Category: c.OrNull(v.Category), Reason: c.OrNull(c.Convert[api.CloseReason](v.Reason)), ReasonNote: c.OrNull(v.ReasonNote),
		Channel:        c.OrNull(c.Convert[api.ApplicationChannel](v.Channel)),
		StageChangedAt: v.StageChangedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

// GetPipelineSummary handles GET /v1/pipeline/summary.
func (h *HTTP) GetPipelineSummary(ctx context.Context, req api.GetPipelineSummaryRequestObject) (api.GetPipelineSummaryResponseObject, error) {
	stuckDays := 14
	if req.Params.StuckDays != nil {
		stuckDays = *req.Params.StuckDays
	}
	sum, err := h.svc.Summary(ctx, authz.MustFromContext(ctx), stuckDays)
	if err != nil {
		return nil, err
	}
	out := api.GetPipelineSummary200JSONResponse{
		OpenJobs: sum.OpenJobs, StageCounts: countsToAPI(sum.Counts), Jobs: make([]api.DashboardJob, len(sum.Jobs)),
		Stuck: toAPIList(sum.Stuck), PlacedLast30Days: sum.PlacedLast30Days, MovedLast7Days: sum.MovedLast7Days,
		MovedPrev7Days: sum.MovedPrev7Days, AddedLast7Days: sum.AddedLast7Days, OpenedLast7Days: sum.OpenedLast7Days,
	}
	for i, j := range sum.Jobs {
		out.Jobs[i] = api.DashboardJob{JobId: j.ID, Title: j.Title, CompanyId: j.CompanyID, CompanyName: j.CompanyName, StageCounts: countsToAPI(j.Counts)}
	}
	return out, nil
}

func countsToAPI(c StageCounts) api.StageCounts {
	return api.StageCounts{
		Applied: c[Applied], Screening: c[Screening], Submitted: c[Submitted],
		Interview: c[Interview], Offer: c[Offer], Placed: c[Placed],
	}
}

// ListJobProgress handles GET /v1/pipeline/jobs.
func (h *HTTP) ListJobProgress(ctx context.Context, req api.ListJobProgressRequestObject) (api.ListJobProgressResponseObject, error) {
	ids := slices.Compact(slices.Sorted(slices.Values(req.Params.JobId)))
	rows, err := h.svc.Progress(ctx, authz.MustFromContext(ctx), ids)
	if err != nil {
		return nil, err
	}
	out := api.ListJobProgress200JSONResponse{Items: make([]api.JobProgress, len(rows))}
	for i, r := range rows {
		out.Items[i] = api.JobProgress{JobId: r.JobID, StageCounts: countsToAPI(r.Counts), LastMovedAt: c.OrNull(r.LastMoved), Waiting: r.Waiting,
			WaitingCounts: countsToAPI(r.WaitingIn)}
	}
	return out, nil
}
