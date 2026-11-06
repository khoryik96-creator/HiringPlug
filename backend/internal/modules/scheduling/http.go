package scheduling

import (
	"context"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// GetMySchedule handles GET /v1/scheduling/me.
func (h *HTTP) GetMySchedule(ctx context.Context, _ api.GetMyScheduleRequestObject) (api.GetMyScheduleResponseObject, error) {
	s, err := h.svc.MySchedule(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMySchedule200JSONResponse(scheduleToAPI(s)), nil
}

// PutMySchedule handles PUT /v1/scheduling/me.
func (h *HTTP) PutMySchedule(ctx context.Context, req api.PutMyScheduleRequestObject) (api.PutMyScheduleResponseObject, error) {
	b := req.Body
	in := Settings{TimeZone: b.TimeZone, BufferMinutes: b.BufferMinutes, MinNoticeHours: b.MinNoticeHours, MaxDaysAhead: b.MaxDaysAhead}
	for _, w := range b.Hours {
		in.Hours = append(in.Hours, Hours{Day: w.Day, Start: w.Start, End: w.End})
	}
	var cal *CalendarChange
	if b.CalendarUrl.IsSpecified() {
		if b.CalendarUrl.IsNull() {
			cal = &CalendarChange{Remove: true}
		} else {
			cal = &CalendarChange{URL: b.CalendarUrl.MustGet()}
		}
	}
	s, err := h.svc.PutMySchedule(ctx, authz.MustFromContext(ctx), in, cal)
	if err != nil {
		return nil, err
	}
	return api.PutMySchedule200JSONResponse(scheduleToAPI(s)), nil
}

// ListBookingLinks handles GET /v1/booking-links.
func (h *HTTP) ListBookingLinks(ctx context.Context, req api.ListBookingLinksRequestObject) (api.ListBookingLinksResponseObject, error) {
	links, err := h.svc.Links(ctx, authz.MustFromContext(ctx), req.Params.CandidateId)
	if err != nil {
		return nil, err
	}
	out := api.ListBookingLinks200JSONResponse{Items: make([]api.BookingLink, len(links))}
	for i, l := range links {
		out.Items[i] = linkToAPI(l)
	}
	return out, nil
}

// CreateBookingLink handles POST /v1/booking-links.
func (h *HTTP) CreateBookingLink(ctx context.Context, req api.CreateBookingLinkRequestObject) (api.CreateBookingLinkResponseObject, error) {
	l, err := h.svc.CreateLink(ctx, authz.MustFromContext(ctx), *req.Body)
	if err != nil {
		return nil, err
	}
	return api.CreateBookingLink201JSONResponse(linkToAPI(l)), nil
}

// CancelBookingLink handles POST /v1/booking-links/{bookingLinkId}/cancel.
func (h *HTTP) CancelBookingLink(ctx context.Context, req api.CancelBookingLinkRequestObject) (api.CancelBookingLinkResponseObject, error) {
	l, err := h.svc.CancelLink(ctx, authz.MustFromContext(ctx), req.BookingLinkId)
	if err != nil {
		return nil, err
	}
	return api.CancelBookingLink200JSONResponse(linkToAPI(l)), nil
}

// GetPublicBooking handles GET /v1/public/bookings/{bookingToken}.
func (h *HTTP) GetPublicBooking(ctx context.Context, req api.GetPublicBookingRequestObject) (api.GetPublicBookingResponseObject, error) {
	p, err := h.svc.PublicPage(ctx, req.BookingToken)
	if err != nil {
		return nil, err
	}
	return api.GetPublicBooking200JSONResponse(pageToAPI(p)), nil
}

// BookPublicBooking handles POST /v1/public/bookings/{bookingToken}.
func (h *HTTP) BookPublicBooking(ctx context.Context, req api.BookPublicBookingRequestObject) (api.BookPublicBookingResponseObject, error) {
	p, err := h.svc.Book(ctx, req.BookingToken, req.Body.StartsAt, clientip.From(ctx))
	if err != nil {
		return nil, err
	}
	return api.BookPublicBooking200JSONResponse(pageToAPI(p)), nil
}

func scheduleToAPI(s Schedule) api.Schedule {
	out := api.Schedule{
		Set: s.Set, TimeZone: s.Settings.TimeZone, Hours: []api.WeeklyHours{}, BufferMinutes: s.Settings.BufferMinutes,
		MinNoticeHours: s.Settings.MinNoticeHours, MaxDaysAhead: s.Settings.MaxDaysAhead,
	}
	for _, w := range s.Settings.Hours {
		out.Hours = append(out.Hours, api.WeeklyHours{Day: w.Day, Start: w.Start, End: w.End})
	}
	out.Calendar.SetNull()
	if cal := s.Calendar; cal != nil {
		out.Calendar.Set(api.CalendarFeed{
			Host: cal.Host, Status: api.CalendarFeedStatus(cal.Status), Error: c.OrNull(cal.Error), CheckedAt: c.OrNull(cal.ReadAt), BusyCount: cal.BusyCount,
		})
	}
	return out
}

func linkToAPI(l Link) api.BookingLink {
	r := l.Row
	return api.BookingLink{
		Id: r.ID, Url: c.OrNull(l.URL), CandidateId: r.CandidateID, CandidateName: l.CandidateName, HostName: l.HostName,
		Title: r.Title, DurationMinutes: int(r.DurationMinutes), Location: c.OrNull(r.Location), Status: api.BookingStatus(l.Status),
		StartsAt: c.OrNull(r.StartsAt), ExpiresAt: r.ExpiresAt, CreatedAt: r.CreatedAt,
	}
}

func pageToAPI(p Page) api.PublicBooking {
	slots := p.Slots
	if slots == nil {
		slots = []time.Time{}
	}
	return api.PublicBooking{
		CompanyName: p.Company, HostName: p.Host, Title: p.Title, DurationMinutes: p.Duration, Location: c.OrNull(p.Location),
		TimeZone: p.TimeZone, Status: api.BookingStatus(p.Status), StartsAt: c.OrNull(p.StartsAt), CanChange: p.CanChange, Slots: slots,
	}
}
