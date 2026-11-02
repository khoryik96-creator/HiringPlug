package pipeline

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the pipeline operations of the contract.
type HTTP struct{ svc *Service }

// ListJobApplications handles GET /v1/jobs/{jobId}/applications.
func (h *HTTP) ListJobApplications(ctx context.Context, req api.ListJobApplicationsRequestObject) (api.ListJobApplicationsResponseObject, error) {
	views, err := h.svc.ForJob(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	return api.ListJobApplications200JSONResponse{Items: toAPIList(views)}, nil
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
	views, err := h.svc.ForCandidate(ctx, authz.MustFromContext(ctx), req.CandidateId)
	if err != nil {
		return nil, err
	}
	return api.ListCandidateApplications200JSONResponse{Items: toAPIList(views)}, nil
}

// MoveApplication handles POST /v1/applications/{applicationId}/move.
func (h *HTTP) MoveApplication(ctx context.Context, req api.MoveApplicationRequestObject) (api.MoveApplicationResponseObject, error) {
	b := req.Body
	view, err := h.svc.Move(ctx, authz.MustFromContext(ctx), req.ApplicationId, b.Version, Move{
		To: string(b.Stage), Reason: c.Convert[string](b.Reason), Note: b.ReasonNote,
	})
	if err != nil {
		return nil, err
	}
	return api.MoveApplication200JSONResponse(toAPI(view)), nil
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
		Stage: api.Stage(v.Stage), Reason: c.OrNull(c.Convert[api.CloseReason](v.Reason)), ReasonNote: c.OrNull(v.ReasonNote),
		StageChangedAt: v.StageChangedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}
