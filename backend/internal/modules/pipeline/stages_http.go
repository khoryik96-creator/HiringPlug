package pipeline

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListStageTemplates handles GET /v1/stage-templates.
func (h *HTTP) ListStageTemplates(ctx context.Context, _ api.ListStageTemplatesRequestObject) (api.ListStageTemplatesResponseObject, error) {
	list, err := h.svc.Templates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListStageTemplates200JSONResponse{Items: make([]api.StageTemplate, len(list)), Standard: stepsToAPI(StandardSteps())}
	for i, t := range list {
		out.Items[i] = templateToAPI(t)
	}
	return out, nil
}

// CreateStageTemplate handles POST /v1/stage-templates.
func (h *HTTP) CreateStageTemplate(ctx context.Context, req api.CreateStageTemplateRequestObject) (api.CreateStageTemplateResponseObject, error) {
	b := req.Body
	t, err := h.svc.CreateTemplate(ctx, authz.MustFromContext(ctx), TemplateInput{Name: b.Name, IsDefault: b.IsDefault != nil && *b.IsDefault, Steps: stepsFromAPI(b.Steps)})
	if err != nil {
		return nil, err
	}
	return api.CreateStageTemplate201JSONResponse(templateToAPI(t)), nil
}

// UpdateStageTemplate handles PUT /v1/stage-templates/{stageTemplateId}.
func (h *HTTP) UpdateStageTemplate(ctx context.Context, req api.UpdateStageTemplateRequestObject) (api.UpdateStageTemplateResponseObject, error) {
	b := req.Body
	t, err := h.svc.UpdateTemplate(ctx, authz.MustFromContext(ctx), req.StageTemplateId, b.Version,
		TemplateInput{Name: b.Name, IsDefault: b.IsDefault != nil && *b.IsDefault, Steps: stepsFromAPI(b.Steps)})
	if err != nil {
		return nil, err
	}
	return api.UpdateStageTemplate200JSONResponse(templateToAPI(t)), nil
}

// DeleteStageTemplate handles DELETE /v1/stage-templates/{stageTemplateId}.
func (h *HTTP) DeleteStageTemplate(ctx context.Context, req api.DeleteStageTemplateRequestObject) (api.DeleteStageTemplateResponseObject, error) {
	if err := h.svc.DeleteTemplate(ctx, authz.MustFromContext(ctx), req.StageTemplateId); err != nil {
		return nil, err
	}
	return api.DeleteStageTemplate204Response{}, nil
}

// GetJobStages handles GET /v1/jobs/{jobId}/stages.
func (h *HTTP) GetJobStages(ctx context.Context, req api.GetJobStagesRequestObject) (api.GetJobStagesResponseObject, error) {
	js, err := h.svc.JobStages(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	return api.GetJobStages200JSONResponse(jobStagesToAPI(js)), nil
}

// SetJobStages handles PUT /v1/jobs/{jobId}/stages.
func (h *HTTP) SetJobStages(ctx context.Context, req api.SetJobStagesRequestObject) (api.SetJobStagesResponseObject, error) {
	b := req.Body
	var templateID *string
	if v, err := b.TemplateId.Get(); err == nil {
		templateID = &v
	}
	var steps []Step
	if v, err := b.Steps.Get(); err == nil {
		steps = stepsFromAPI(v)
		if steps == nil {
			steps = []Step{} // given, but empty: refused as such
		}
	}
	js, err := h.svc.SetJobStages(ctx, authz.MustFromContext(ctx), req.JobId, b.Version, templateID, steps)
	if err != nil {
		return nil, err
	}
	return api.SetJobStages200JSONResponse(jobStagesToAPI(js)), nil
}

func stepsFromAPI(in []api.StageStepInput) []Step {
	if in == nil {
		return nil
	}
	out := make([]Step, len(in))
	for i, s := range in {
		out[i] = Step{Label: s.Label, Stage: string(s.Stage)}
		if s.Key != nil {
			out[i].Key = *s.Key
		}
	}
	return out
}

func stepsToAPI(in []Step) []api.StageStep {
	out := make([]api.StageStep, len(in))
	for i, s := range in {
		out[i] = api.StageStep{Key: s.Key, Label: s.Label, Stage: api.OpenStage(s.Stage)}
	}
	return out
}

func templateToAPI(t Template) api.StageTemplate {
	return api.StageTemplate{Id: t.ID, Version: t.Version, Name: t.Name, IsDefault: t.IsDefault, Steps: stepsToAPI(t.Steps), JobCount: t.JobCount,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

func jobStagesToAPI(js JobStages) api.JobStages {
	return api.JobStages{JobId: js.JobID, Version: js.Version, Source: api.JobStagesSource(js.Source),
		TemplateId: c.OrNull(js.TemplateID), TemplateName: c.OrNull(js.TemplateName), Steps: stepsToAPI(js.Steps)}
}
