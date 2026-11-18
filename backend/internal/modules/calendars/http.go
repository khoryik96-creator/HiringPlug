package calendars

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

type HTTP struct{ svc *Service }

func (h *HTTP) GetOutlookConnection(ctx context.Context, _ api.GetOutlookConnectionRequestObject) (api.GetOutlookConnectionResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.status(ctx, p)
	return api.GetOutlookConnection200JSONResponse(v), err
}
func (h *HTTP) AuthorizeOutlookCalendar(ctx context.Context, _ api.AuthorizeOutlookCalendarRequestObject) (api.AuthorizeOutlookCalendarResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.authorize(ctx, p)
	return api.AuthorizeOutlookCalendar200JSONResponse{Url: v}, err
}
func (h *HTTP) CompleteOutlookCallback(ctx context.Context, r api.CompleteOutlookCallbackRequestObject) (api.CompleteOutlookCallbackResponseObject, error) {
	outcome := "failed"
	if ticket, err := h.svc.park(ctx, text(r.Params.State), text(r.Params.Code), text(r.Params.Error)); err == nil {
		outcome = "confirm&ticket=" + ticket
	}
	target := h.svc.config.PublicURL + "/#/calendar?outlook=" + outcome
	cache, referrer := "no-store", "no-referrer"
	return api.CompleteOutlookCallback303Response{Headers: api.CompleteOutlookCallback303ResponseHeaders{Location: &target, CacheControl: &cache, ReferrerPolicy: &referrer}}, nil
}
func (h *HTTP) FinishOutlookCalendar(ctx context.Context, r api.FinishOutlookCalendarRequestObject) (api.FinishOutlookCalendarResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.complete(ctx, p, r.Body.Ticket); err != nil {
		return nil, err
	}
	v, err := h.svc.status(ctx, p)
	return api.FinishOutlookCalendar200JSONResponse(v), err
}
func (h *HTTP) DisconnectOutlookCalendar(ctx context.Context, _ api.DisconnectOutlookCalendarRequestObject) (api.DisconnectOutlookCalendarResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.disconnect(ctx, p); err != nil {
		return nil, err
	}
	return api.DisconnectOutlookCalendar204Response{}, nil
}
func (h *HTTP) RefreshOutlookCalendar(ctx context.Context, _ api.RefreshOutlookCalendarRequestObject) (api.RefreshOutlookCalendarResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.svc.askRefresh(ctx, p); err != nil {
		return nil, err
	}
	v, err := h.svc.status(ctx, p)
	return api.RefreshOutlookCalendar200JSONResponse(v), err
}
func (h *HTTP) ListCalendarEvents(ctx context.Context, r api.ListCalendarEventsRequestObject) (api.ListCalendarEventsResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.list(ctx, p, r.Params.From, r.Params.To)
	return api.ListCalendarEvents200JSONResponse(v), err
}
func (h *HTTP) CreateCalendarEvent(ctx context.Context, r api.CreateCalendarEventRequestObject) (api.CreateCalendarEventResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.save(ctx, p, "", 0, inputOf(*r.Body))
	return api.CreateCalendarEvent201JSONResponse(v), err
}
func (h *HTTP) UpdateCalendarEvent(ctx context.Context, r api.UpdateCalendarEventRequestObject) (api.UpdateCalendarEventResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	b := r.Body
	in := Input{Title: b.Title, Notes: text(b.Notes), Location: text(b.Location), Start: b.StartsAt, End: b.EndsAt, TimeZone: b.TimeZone, Attendees: inputAttendees(b.Attendees), AllDay: b.AllDay, Teams: b.Teams}
	v, err := h.svc.save(ctx, p, r.EventId, b.Version, in)
	return api.UpdateCalendarEvent200JSONResponse(v), err
}
func (h *HTTP) CancelCalendarEvent(ctx context.Context, r api.CancelCalendarEventRequestObject) (api.CancelCalendarEventResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.action(ctx, p, r.EventId, r.Body.Version, false)
	return api.CancelCalendarEvent200JSONResponse(v), err
}
func (h *HTTP) RetryCalendarEvent(ctx context.Context, r api.RetryCalendarEventRequestObject) (api.RetryCalendarEventResponseObject, error) {
	p, err := personal(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.svc.action(ctx, p, r.EventId, r.Body.Version, true)
	return api.RetryCalendarEvent200JSONResponse(v), err
}
func inputOf(b api.CalendarEventInput) Input {
	return Input{Title: b.Title, Notes: text(b.Notes), Location: text(b.Location), Start: b.StartsAt, End: b.EndsAt, TimeZone: b.TimeZone, Attendees: inputAttendees(b.Attendees), AllDay: b.AllDay, Teams: b.Teams}
}
func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func inputAttendees(people []api.CalendarAttendee) []Attendee {
	out := make([]Attendee, 0, len(people))
	for _, p := range people {
		out = append(out, Attendee{Email: string(p.Email), Name: p.Name})
	}
	return out
}
