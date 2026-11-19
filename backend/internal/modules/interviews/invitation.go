package interviews

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) invitation(ctx context.Context, p authz.Principal, id string, input *api.InterviewInvitationInput) (api.InterviewInvitation, error) {
	var out api.InterviewInvitation
	if !strings.HasPrefix(p.UserID, "usr_") {
		return out, apperr.New(api.ErrorCodeFORBIDDEN, "Sign in to use Outlook interview invitations.")
	}
	if s.calendar == nil {
		return out, apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, "Outlook invitations are unavailable.")
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		// The interview's normal RLS applies before any calendar projection.
		if _, err := getRow(ctx, store.New(tx), id); err != nil {
			return err
		}
		var err error
		if input == nil {
			out, err = s.calendar.InterviewInvitation(ctx, tx, p, id)
		} else {
			out, err = s.calendar.SendInterviewInvitation(ctx, tx, p, id, *input)
		}
		return err
	})
	return out, err
}

func (h *HTTP) GetInterviewInvitation(ctx context.Context, req api.GetInterviewInvitationRequestObject) (api.GetInterviewInvitationResponseObject, error) {
	out, err := h.svc.invitation(ctx, authz.MustFromContext(ctx), req.InterviewId, nil)
	return api.GetInterviewInvitation200JSONResponse(out), err
}

func (h *HTTP) SendInterviewInvitation(ctx context.Context, req api.SendInterviewInvitationRequestObject) (api.SendInterviewInvitationResponseObject, error) {
	out, err := h.svc.invitation(ctx, authz.MustFromContext(ctx), req.InterviewId, req.Body)
	return api.SendInterviewInvitation202JSONResponse(out), err
}

func (h *HTTP) SetInterviewOutlookReminder(ctx context.Context, req api.SetInterviewOutlookReminderRequestObject) (api.SetInterviewOutlookReminderResponseObject, error) {
	p := authz.MustFromContext(ctx)
	var out api.InterviewInvitation
	if !strings.HasPrefix(p.UserID, "usr_") {
		return nil, apperr.New(api.ErrorCodeFORBIDDEN, "Sign in to manage Outlook interview reminders.")
	}
	if h.svc.calendar == nil {
		return nil, apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, "Outlook reminders are unavailable.")
	}
	err := database.InTx(ctx, h.svc.pool, p, func(tx pgx.Tx) error {
		if _, err := getRow(ctx, store.New(tx), req.InterviewId); err != nil {
			return err
		}
		var err error
		out, err = h.svc.calendar.SetInterviewReminder(ctx, tx, p, req.InterviewId, *req.Body)
		return err
	})
	return api.SetInterviewOutlookReminder200JSONResponse(out), err
}
