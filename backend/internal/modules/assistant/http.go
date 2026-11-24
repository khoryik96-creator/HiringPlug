package assistant

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// GetAssistant handles GET /v1/assistant.
func (h *HTTP) GetAssistant(ctx context.Context, _ api.GetAssistantRequestObject) (api.GetAssistantResponseObject, error) {
	st, err := h.svc.Status(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetAssistant200JSONResponse{Enabled: st.Enabled, UsedTokens: st.UsedTokens, MonthlyTokens: st.LimitTokens, UsedPercent: st.UsedPercent}, nil
}

// ListAssistantAnswers handles GET /v1/assistant/answers.
func (h *HTTP) ListAssistantAnswers(ctx context.Context, req api.ListAssistantAnswersRequestObject) (api.ListAssistantAnswersResponseObject, error) {
	rows, err := h.svc.List(ctx, authz.MustFromContext(ctx), string(req.Params.SubjectType), req.Params.SubjectId)
	if err != nil {
		return nil, err
	}
	out := api.AssistantAnswerList{Items: make([]api.AssistantAnswer, len(rows))}
	for i, r := range rows {
		out.Items[i] = answerToAPI(r)
	}
	return api.ListAssistantAnswers200JSONResponse(out), nil
}

// AskAssistant handles POST /v1/assistant/answers.
func (h *HTTP) AskAssistant(ctx context.Context, req api.AskAssistantRequestObject) (api.AskAssistantResponseObject, error) {
	b := req.Body
	a, err := h.svc.Ask(ctx, authz.MustFromContext(ctx), Question{
		SubjectType: string(b.SubjectType), SubjectID: b.SubjectId, Kind: string(b.Kind), Question: b.Question,
	})
	if err != nil {
		return nil, err
	}
	return api.AskAssistant202JSONResponse(answerToAPI(a)), nil
}

// DraftEmailWithAi handles POST /v1/assistant/email-drafts.
func (h *HTTP) DraftEmailWithAi(ctx context.Context, req api.DraftEmailWithAiRequestObject) (api.DraftEmailWithAiResponseObject, error) {
	b := req.Body
	ask := EmailAsk{Mode: string(b.Mode), Recipient: string(b.Recipient), Instruction: b.Instruction, Subject: b.Subject,
		Draft: b.Draft, TheirText: b.TheirMessage, ApplicationID: b.ApplicationId}
	if b.Style != nil {
		st := string(*b.Style)
		ask.Style = &st
	}
	d, err := h.svc.DraftEmail(ctx, authz.MustFromContext(ctx), ask)
	if err != nil {
		return nil, err
	}
	return api.DraftEmailWithAi202JSONResponse(emailToAPI(d)), nil
}

// GetAiEmailDraft handles GET /v1/assistant/email-drafts/{draftId}.
func (h *HTTP) GetAiEmailDraft(ctx context.Context, req api.GetAiEmailDraftRequestObject) (api.GetAiEmailDraftResponseObject, error) {
	d, err := h.svc.EmailDraft(ctx, authz.MustFromContext(ctx), req.DraftId)
	if err != nil {
		return nil, err
	}
	return api.GetAiEmailDraft200JSONResponse(emailToAPI(d)), nil
}

func emailToAPI(r store.AssistantEmail) api.AssistantEmailDraft {
	return api.AssistantEmailDraft{Id: r.ID, Status: api.AssistantEmailStatus(r.Status), Subject: apiconv.OrNull(r.Subject),
		Body: apiconv.OrNull(r.Body), Error: apiconv.OrNull(r.Error), CreatedAt: r.CreatedAt}
}

func answerToAPI(r store.AssistantAnswer) api.AssistantAnswer {
	return api.AssistantAnswer{
		Id: r.ID, SubjectType: api.AssistantSubjectType(r.SubjectType), SubjectId: r.SubjectID, Kind: api.AssistantKind(r.Kind),
		Question: apiconv.OrNull(r.Question), Status: api.AssistantAnswerStatus(r.Status), Answer: apiconv.OrNull(r.Answer),
		Error: apiconv.OrNull(r.Error), Model: apiconv.OrNull(r.Model), CreatedAt: r.CreatedAt, AnsweredAt: apiconv.OrNull(r.AnsweredAt),
	}
}
