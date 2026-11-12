package automations

import (
	"context"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API.
type HTTP struct{ svc *Service }

// ListAutomations handles GET /v1/automations.
func (h *HTTP) ListAutomations(ctx context.Context, _ api.ListAutomationsRequestObject) (api.ListAutomationsResponseObject, error) {
	views, mode, err := h.svc.List(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	items := make([]api.Automation, len(views))
	for i, v := range views {
		items[i] = toAPI(v)
	}
	return api.ListAutomations200JSONResponse{Items: items, Settings: api.AutomationSettings{DefaultSendMode: api.AutomationSendMode(mode)}}, nil
}

// CreateAutomation handles POST /v1/automations.
func (h *HTTP) CreateAutomation(ctx context.Context, req api.CreateAutomationRequestObject) (api.CreateAutomationResponseObject, error) {
	v, err := h.svc.Create(ctx, authz.MustFromContext(ctx), ruleIn(*req.Body))
	if err != nil {
		return nil, err
	}
	return api.CreateAutomation201JSONResponse(toAPI(v)), nil
}

// UpdateAutomation handles PUT /v1/automations/{automationId}.
func (h *HTTP) UpdateAutomation(ctx context.Context, req api.UpdateAutomationRequestObject) (api.UpdateAutomationResponseObject, error) {
	version := 0
	if req.Body.Version != nil {
		version = *req.Body.Version
	}
	v, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.AutomationId, version, ruleIn(*req.Body))
	if err != nil {
		return nil, err
	}
	return api.UpdateAutomation200JSONResponse(toAPI(v)), nil
}

// DeleteAutomation handles DELETE /v1/automations/{automationId}.
func (h *HTTP) DeleteAutomation(ctx context.Context, req api.DeleteAutomationRequestObject) (api.DeleteAutomationResponseObject, error) {
	if err := h.svc.Delete(ctx, authz.MustFromContext(ctx), req.AutomationId); err != nil {
		return nil, err
	}
	return api.DeleteAutomation204Response{}, nil
}

// PutAutomationSettings handles PUT /v1/automation-settings.
func (h *HTTP) PutAutomationSettings(ctx context.Context, req api.PutAutomationSettingsRequestObject) (api.PutAutomationSettingsResponseObject, error) {
	mode, err := h.svc.SetDefault(ctx, authz.MustFromContext(ctx), string(req.Body.DefaultSendMode))
	if err != nil {
		return nil, err
	}
	return api.PutAutomationSettings200JSONResponse{DefaultSendMode: api.AutomationSendMode(mode)}, nil
}

// ListAutomationRuns handles GET /v1/automation-runs.
func (h *HTTP) ListAutomationRuns(ctx context.Context, req api.ListAutomationRunsRequestObject) (api.ListAutomationRunsResponseObject, error) {
	runs, err := h.svc.Runs(ctx, authz.MustFromContext(ctx), c.Convert[string](req.Params.Status))
	if err != nil {
		return nil, err
	}
	items := make([]api.AutomationRun, len(runs))
	for i, r := range runs {
		items[i] = runToAPI(r)
	}
	return api.ListAutomationRuns200JSONResponse{Items: items}, nil
}

// SendAutomationRun handles POST /v1/automation-runs/{runId}/send.
func (h *HTTP) SendAutomationRun(ctx context.Context, req api.SendAutomationRunRequestObject) (api.SendAutomationRunResponseObject, error) {
	r, err := h.svc.Approve(ctx, authz.MustFromContext(ctx), req.RunId)
	if err != nil {
		return nil, err
	}
	return api.SendAutomationRun200JSONResponse(runToAPI(r)), nil
}

// SkipAutomationRun handles POST /v1/automation-runs/{runId}/skip.
func (h *HTTP) SkipAutomationRun(ctx context.Context, req api.SkipAutomationRunRequestObject) (api.SkipAutomationRunResponseObject, error) {
	r, err := h.svc.Skip(ctx, authz.MustFromContext(ctx), req.RunId)
	if err != nil {
		return nil, err
	}
	return api.SkipAutomationRun200JSONResponse(runToAPI(r)), nil
}

// ListTasks handles GET /v1/tasks.
func (h *HTTP) ListTasks(ctx context.Context, _ api.ListTasksRequestObject) (api.ListTasksResponseObject, error) {
	tasks, err := h.svc.Tasks(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	items := make([]api.Task, len(tasks))
	for i, t := range tasks {
		items[i] = taskToAPI(t)
	}
	return api.ListTasks200JSONResponse{Items: items}, nil
}

// UpdateTask handles PATCH /v1/tasks/{taskId}.
func (h *HTTP) UpdateTask(ctx context.Context, req api.UpdateTaskRequestObject) (api.UpdateTaskResponseObject, error) {
	t, err := h.svc.SetTaskDone(ctx, authz.MustFromContext(ctx), req.TaskId, req.Body.Done)
	if err != nil {
		return nil, err
	}
	return api.UpdateTask200JSONResponse(taskToAPI(t)), nil
}

func ruleIn(b api.AutomationInput) Rule {
	r := Rule{Name: b.Name, Enabled: b.Enabled, Trigger: string(b.Trigger), Stage: c.Convert[string](b.Stage), OffsetDays: b.OffsetDays,
		SendMode: string(b.SendMode), Actions: make([]Action, len(b.Actions))}
	if id, err := b.CompanyId.Get(); err == nil {
		r.CompanyID = &id
	}
	for i, a := range b.Actions {
		r.Actions[i] = Action{Kind: string(a.Kind), Subject: deref(a.Subject), Body: deref(a.Body), Title: deref(a.Title)}
	}
	return r
}

func actionsToAPI(in []Action) []api.AutomationAction {
	out := make([]api.AutomationAction, len(in))
	for i, a := range in {
		out[i] = api.AutomationAction{Kind: api.AutomationActionKind(a.Kind)}
		if a.IsEmail() {
			out[i].Subject, out[i].Body = &a.Subject, &a.Body
		} else {
			out[i].Title = &a.Title
		}
	}
	return out
}

func toAPI(v View) api.Automation {
	out := api.Automation{
		Id: v.ID, Version: int(v.Version), Name: v.Name, Enabled: v.Enabled, Trigger: api.AutomationTrigger(v.Trigger),
		OffsetDays: int(v.OffsetDays), CompanyId: c.OrNull(v.CompanyID), CompanyName: c.OrNull(v.CompanyName),
		Actions: actionsToAPI(v.Rule.Actions), SendMode: api.AutomationSendMode(v.SendMode), Preset: c.OrNull(v.Preset),
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	out.Stage = nullable.NewNullNullable[api.Stage]()
	if v.Stage != nil {
		out.Stage = nullable.NewNullableWithValue(api.Stage(*v.Stage))
	}
	return out
}

func runToAPI(r RunView) api.AutomationRun {
	out := api.AutomationRun{
		Id: r.ID, AutomationId: r.AutomationID, AutomationName: r.AutomationName, Status: api.AutomationRunStatus(r.Status), DueAt: r.DueAt,
		ApplicationId: nullable.NewNullableWithValue(r.ApplicationID), Actions: actionsToAPI(r.Actions), Detail: c.OrNull(r.Detail),
		FinishedAt: c.OrNull(r.FinishedAt), CandidateId: nullable.NewNullNullable[string](), CandidateName: nullable.NewNullNullable[string](),
		JobTitle: nullable.NewNullNullable[string](), CompanyName: nullable.NewNullNullable[string](),
	}
	if a := r.App; a != nil {
		out.CandidateId, out.CandidateName = nullable.NewNullableWithValue(a.CandidateID), nullable.NewNullableWithValue(a.CandidateName)
		out.JobTitle, out.CompanyName = nullable.NewNullableWithValue(a.JobTitle), nullable.NewNullableWithValue(a.CompanyName)
	}
	return out
}

func taskToAPI(t TaskView) api.Task {
	out := api.Task{
		Id: t.ID, Title: t.Title, DueOn: openapi_types.Date{Time: t.DueOn}, DoneAt: c.OrNull(t.DoneAt), ApplicationId: c.OrNull(t.ApplicationID),
		AutomationName: c.OrNull(t.AutomationName), CreatedAt: t.CreatedAt, CandidateId: nullable.NewNullNullable[string](),
		CandidateName: nullable.NewNullNullable[string](), JobTitle: nullable.NewNullNullable[string](),
	}
	if a := t.App; a != nil {
		out.CandidateId, out.CandidateName = nullable.NewNullableWithValue(a.CandidateID), nullable.NewNullableWithValue(a.CandidateName)
		out.JobTitle = nullable.NewNullableWithValue(a.JobTitle)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
