package interviewprep

import (
	"context"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the interview_prep operations of the contract.
type HTTP struct{ svc *Service }

// ListInterviewQuestions handles GET /v1/jobs/{jobId}/interview-questions.
func (h *HTTP) ListInterviewQuestions(ctx context.Context, req api.ListInterviewQuestionsRequestObject) (api.ListInterviewQuestionsResponseObject, error) {
	items, err := h.svc.Questions(ctx, authz.MustFromContext(ctx), req.JobId, req.Params.Archived != nil && *req.Params.Archived)
	if err != nil {
		return nil, err
	}
	out := api.ListInterviewQuestions200JSONResponse{Items: make([]api.InterviewQuestion, len(items))}
	for i, q := range items {
		out.Items[i] = questionToAPI(q)
	}
	return out, nil
}

// CreateInterviewQuestion handles POST /v1/jobs/{jobId}/interview-questions.
func (h *HTTP) CreateInterviewQuestion(ctx context.Context, req api.CreateInterviewQuestionRequestObject) (api.CreateInterviewQuestionResponseObject, error) {
	b := req.Body
	f := QuestionFields{Question: b.Question, Kind: string(b.Kind), Source: value(b.Source), Notes: value(b.Notes), AskedOn: day(b.AskedOn)}
	if r, err := b.Round.Get(); err == nil {
		round := string(r)
		f.Round = &round
	}
	q, err := h.svc.AddQuestion(ctx, authz.MustFromContext(ctx), req.JobId, f)
	if err != nil {
		return nil, err
	}
	return api.CreateInterviewQuestion201JSONResponse(questionToAPI(q)), nil
}

// UpdateInterviewQuestion handles PATCH /v1/interview-questions/{questionId}.
func (h *HTTP) UpdateInterviewQuestion(ctx context.Context, req api.UpdateInterviewQuestionRequestObject) (api.UpdateInterviewQuestionResponseObject, error) {
	b := req.Body
	c := QuestionChange{Question: b.Question, Archived: b.Archived}
	if b.Kind != nil {
		k := string(*b.Kind)
		c.Kind = &k
	}
	if b.Round.IsSpecified() {
		if r, err := b.Round.Get(); err == nil {
			round := string(r)
			c.Round = &round
		} else {
			c.ClearRound = true
		}
	}
	c.Source, c.ClearSource = change(b.Source)
	c.Notes, c.ClearNotes = change(b.Notes)
	if b.AskedOn.IsSpecified() {
		if d, err := b.AskedOn.Get(); err == nil {
			c.AskedOn = &d.Time
		} else {
			c.ClearAskedOn = true
		}
	}
	q, err := h.svc.ChangeQuestion(ctx, authz.MustFromContext(ctx), req.QuestionId, b.Version, c)
	if err != nil {
		return nil, err
	}
	return api.UpdateInterviewQuestion200JSONResponse(questionToAPI(q)), nil
}

// DeleteInterviewQuestion handles DELETE /v1/interview-questions/{questionId}.
func (h *HTTP) DeleteInterviewQuestion(ctx context.Context, req api.DeleteInterviewQuestionRequestObject) (api.DeleteInterviewQuestionResponseObject, error) {
	if err := h.svc.DeleteQuestion(ctx, authz.MustFromContext(ctx), req.QuestionId); err != nil {
		return nil, err
	}
	return api.DeleteInterviewQuestion204Response{}, nil
}

// ListInterviewPreps handles GET /v1/jobs/{jobId}/interview-preps.
func (h *HTTP) ListInterviewPreps(ctx context.Context, req api.ListInterviewPrepsRequestObject) (api.ListInterviewPrepsResponseObject, error) {
	items, err := h.svc.Preps(ctx, authz.MustFromContext(ctx), req.JobId, req.Params.Archived != nil && *req.Params.Archived)
	if err != nil {
		return nil, err
	}
	out := api.ListInterviewPreps200JSONResponse{Items: make([]api.InterviewPrep, len(items))}
	for i, p := range items {
		out.Items[i] = prepToAPI(p)
	}
	return out, nil
}

// CreateInterviewPrep handles POST /v1/jobs/{jobId}/interview-preps.
func (h *HTTP) CreateInterviewPrep(ctx context.Context, req api.CreateInterviewPrepRequestObject) (api.CreateInterviewPrepResponseObject, error) {
	p, err := h.svc.StartPrep(ctx, authz.MustFromContext(ctx), req.JobId, req.Body.Focus)
	if err != nil {
		return nil, err
	}
	return api.CreateInterviewPrep202JSONResponse(prepToAPI(p)), nil
}

// UpdateInterviewPrep handles PATCH /v1/interview-preps/{prepId}.
func (h *HTTP) UpdateInterviewPrep(ctx context.Context, req api.UpdateInterviewPrepRequestObject) (api.UpdateInterviewPrepResponseObject, error) {
	p, err := h.svc.ArchivePrep(ctx, authz.MustFromContext(ctx), req.PrepId, req.Body.Archived)
	if err != nil {
		return nil, err
	}
	return api.UpdateInterviewPrep200JSONResponse(prepToAPI(p)), nil
}

func questionToAPI(q Question) api.InterviewQuestion {
	out := api.InterviewQuestion{
		Id: q.ID, JobId: q.JobID, Question: q.Question, Kind: api.InterviewQuestionKind(q.Kind), Round: nullRound(q.Round),
		Source: orNull(q.Source), Notes: orNull(q.Notes), Archived: q.Archived, Version: q.Version, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
		AskedOn: nullable.NewNullNullable[openapi_types.Date](),
	}
	if q.AskedOn != nil {
		out.AskedOn.Set(openapi_types.Date{Time: *q.AskedOn})
	}
	return out
}

func prepToAPI(p Prep) api.InterviewPrep {
	out := api.InterviewPrep{
		Id: p.ID, JobId: p.JobID, Status: api.InterviewPrepStatus(p.Status), Focus: orNull(p.Focus), Error: orNull(p.Error), Model: orNull(p.Model),
		QuestionCount: p.QuestionCount, Archived: p.Archived, CreatedAt: p.CreatedAt,
		Content: nullable.NewNullNullable[api.InterviewPrepContent](), FinishedAt: nullable.NewNullNullable[time.Time](),
	}
	if p.Content != nil {
		out.Content.Set(*p.Content)
	}
	if p.FinishedAt != nil {
		out.FinishedAt.Set(*p.FinishedAt)
	}
	return out
}

func value(n nullable.Nullable[string]) *string {
	if v, err := n.Get(); err == nil {
		return &v
	}
	return nil
}

// change is an optional field of a patch: a value, or cleared (sent as null).
func change(n nullable.Nullable[string]) (*string, bool) {
	if !n.IsSpecified() {
		return nil, false
	}
	if v, err := n.Get(); err == nil {
		return &v, false
	}
	return nil, true
}

func day(n nullable.Nullable[openapi_types.Date]) *time.Time {
	if d, err := n.Get(); err == nil {
		return &d.Time
	}
	return nil
}
