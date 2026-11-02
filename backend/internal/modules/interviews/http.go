package interviews

import (
	"context"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the interviews operations of the contract.
type HTTP struct{ svc *Service }

// ListApplicationInterviews handles GET /v1/applications/{applicationId}/interviews.
func (h *HTTP) ListApplicationInterviews(ctx context.Context, req api.ListApplicationInterviewsRequestObject) (api.ListApplicationInterviewsResponseObject, error) {
	views, err := h.svc.ForApplication(ctx, authz.MustFromContext(ctx), req.ApplicationId)
	if err != nil {
		return nil, err
	}
	return api.ListApplicationInterviews200JSONResponse(list(views)), nil
}

// ScheduleInterview handles POST /v1/applications/{applicationId}/interviews.
func (h *HTTP) ScheduleInterview(ctx context.Context, req api.ScheduleInterviewRequestObject) (api.ScheduleInterviewResponseObject, error) {
	b := req.Body
	v, err := h.svc.Schedule(ctx, authz.MustFromContext(ctx), req.ApplicationId, Details{
		Title: b.Title, StartsAt: b.StartsAt, Duration: b.DurationMinutes, Mode: string(b.Mode),
		Location: c.ValueOrNil(b.Location), Instructions: c.ValueOrNil(b.Instructions), Interviewers: b.InterviewerIds,
	}, b.IgnoreClashes != nil && *b.IgnoreClashes)
	if err != nil {
		return nil, err
	}
	return api.ScheduleInterview201JSONResponse(toAPI(v)), nil
}

// ListInterviews handles GET /v1/interviews.
func (h *HTTP) ListInterviews(ctx context.Context, req api.ListInterviewsRequestObject) (api.ListInterviewsResponseObject, error) {
	p := req.Params
	views, err := h.svc.List(ctx, authz.MustFromContext(ctx), p.From, p.To, p.Mine != nil && *p.Mine, p.FeedbackDue != nil && *p.FeedbackDue)
	if err != nil {
		return nil, err
	}
	return api.ListInterviews200JSONResponse(list(views)), nil
}

// GetInterview handles GET /v1/interviews/{interviewId}.
func (h *HTTP) GetInterview(ctx context.Context, req api.GetInterviewRequestObject) (api.GetInterviewResponseObject, error) {
	v, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.InterviewId)
	if err != nil {
		return nil, err
	}
	return api.GetInterview200JSONResponse(toAPI(v)), nil
}

// UpdateInterview handles PATCH /v1/interviews/{interviewId}.
func (h *HTTP) UpdateInterview(ctx context.Context, req api.UpdateInterviewRequestObject) (api.UpdateInterviewResponseObject, error) {
	b := req.Body
	patch := Patch{
		Title:        c.Required(b.Title),
		StartsAt:     c.Required(b.StartsAt),
		Duration:     c.Required(b.DurationMinutes),
		Mode:         c.Required(c.Convert[string](b.Mode)),
		Location:     c.Optional(b.Location, c.Same[string]),
		Instructions: c.Optional(b.Instructions, c.Same[string]),
		Interviewers: c.Required(b.InterviewerIds),
		Status:       c.Required(c.Convert[string](b.Status)),
	}
	v, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.InterviewId, b.Version, patch, b.IgnoreClashes != nil && *b.IgnoreClashes)
	if err != nil {
		return nil, err
	}
	return api.UpdateInterview200JSONResponse(toAPI(v)), nil
}

// PutInterviewFeedback handles PUT /v1/interviews/{interviewId}/feedback.
func (h *HTTP) PutInterviewFeedback(ctx context.Context, req api.PutInterviewFeedbackRequestObject) (api.PutInterviewFeedbackResponseObject, error) {
	b := req.Body
	v, err := h.svc.PutFeedback(ctx, authz.MustFromContext(ctx), req.InterviewId, string(b.Recommendation), c.ValueOrNil(b.Comments))
	if err != nil {
		return nil, err
	}
	return api.PutInterviewFeedback200JSONResponse(toAPI(v)), nil
}

// GetInterviewCalendar handles GET /v1/interviews/{interviewId}/calendar.ics.
func (h *HTTP) GetInterviewCalendar(ctx context.Context, req api.GetInterviewCalendarRequestObject) (api.GetInterviewCalendarResponseObject, error) {
	ics, err := h.svc.CalendarFile(ctx, authz.MustFromContext(ctx), req.InterviewId)
	if err != nil {
		return nil, err
	}
	return api.GetInterviewCalendar200TextcalendarResponse{Body: strings.NewReader(ics), ContentLength: int64(len(ics))}, nil
}

func list(views []View) api.InterviewList {
	out := api.InterviewList{Items: make([]api.Interview, len(views))}
	for i, v := range views {
		out.Items[i] = toAPI(v)
	}
	return out
}

func toAPI(v View) api.Interview {
	d := v.Details
	out := api.Interview{
		Id: v.ID, Version: v.Version, ApplicationId: v.App.ID, CandidateId: v.App.CandidateID, CandidateName: v.App.CandidateName,
		JobId: v.App.JobID, JobTitle: v.App.JobTitle, Title: d.Title, StartsAt: d.StartsAt.UTC().Truncate(time.Second),
		DurationMinutes: d.Duration, Mode: api.InterviewMode(d.Mode), Location: c.OrNull(d.Location),
		Instructions: c.OrNull(d.Instructions), Status: api.InterviewStatus(d.Status),
		Interviewers: make([]api.Interviewer, len(v.Interviewers)), Feedback: make([]api.Feedback, len(v.Feedback)),
		FeedbackHidden: v.FeedbackHidden, MyFeedbackDue: v.MyFeedbackDue, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	for i, p := range v.Interviewers {
		out.Interviewers[i] = api.Interviewer{UserId: p.ID, Name: p.Name}
	}
	for i, f := range v.Feedback {
		out.Feedback[i] = api.Feedback{
			UserId: f.UserID, Name: v.Names[f.UserID], Recommendation: api.Recommendation(f.Recommendation),
			Comments: c.OrNull(f.Comments), SubmittedAt: f.SubmittedAt,
		}
	}
	return out
}
