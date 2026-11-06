package scheduling

// The public booking page: the candidate needs no account. The link's token
// is turned into its company (app_booking_tenant); everything after runs in
// that company's tenant scope as the app.

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Page is the public booking page.
type Page struct {
	Company, Host, Title, TimeZone, Status string
	Duration                               int
	Location                               *string
	StartsAt                               *time.Time
	CanChange                              bool
	Slots                                  []time.Time
}

func (s *Service) tenant(ctx context.Context, token string) (authz.Principal, string, string, error) {
	hash := HashToken(token)
	row, err := store.New(s.pool).BookingTenant(ctx, hash)
	if database.IsNoRows(err) {
		return authz.Principal{}, "", "", apperr.NotFound("booking link")
	}
	if err != nil {
		return authz.Principal{}, "", "", err
	}
	return authz.System(row.TenantID), row.TenantName, hash, nil
}

// busy is when the host is busy between from and to: their calendar, the
// interviews they are on and their other bookings.
func (s *Service) busy(ctx context.Context, tx pgx.Tx, hostID, exceptLink string, from, to time.Time) ([]ical.Interval, error) {
	q := store.New(tx)
	cal, err := q.CalendarBusy(ctx, store.CalendarBusyParams{UserID: hostID, FromTime: from, ToTime: to})
	if err != nil {
		return nil, err
	}
	out := make([]ical.Interval, 0, len(cal))
	for _, b := range cal {
		out = append(out, ical.Interval{Start: b.StartsAt, End: b.EndsAt})
	}
	interviews, err := s.deps.Interviews.BusyFor(ctx, tx, hostID, from, to)
	if err != nil {
		return nil, err
	}
	out = append(out, interviews...)
	others, err := q.HostBookings(ctx, store.HostBookingsParams{HostUserID: hostID, ExceptID: exceptLink, FromTime: from, ToTime: to})
	if err != nil {
		return nil, err
	}
	for _, b := range others {
		if b.StartsAt != nil {
			out = append(out, ical.Interval{Start: *b.StartsAt, End: b.StartsAt.Add(time.Duration(b.DurationMinutes) * time.Minute)})
		}
	}
	return out, nil
}

// page builds the page for a link (locked or not), with the free times when
// it can be booked or moved.
func (s *Service) page(ctx context.Context, tx pgx.Tx, company string, row store.BookingLink, sched store.Schedule) (Page, error) {
	settings, err := settingsFrom(sched)
	if err != nil {
		return Page{}, err
	}
	loc, err := time.LoadLocation(sched.TimeZone)
	if err != nil {
		loc = s.settings.DefaultTimeZone
	}
	now := s.now()
	notice := time.Duration(settings.MinNoticeHours) * time.Hour
	out := Page{
		Company: company, Title: row.Title, TimeZone: loc.String(), Duration: int(row.DurationMinutes),
		Status: Status(row.Status, row.ExpiresAt, now), StartsAt: row.StartsAt,
	}
	out.CanChange = CanChange(out.Status, row.StartsAt, notice, now)
	if out.Status == "booked" {
		out.Location = row.Location
	}
	if h, err := s.deps.Users.Person(ctx, tx, row.HostUserID); err != nil {
		return Page{}, err
	} else if h != nil {
		out.Host = h.Name
	}
	if out.Status == "open" || out.CanChange {
		length := time.Duration(row.DurationMinutes) * time.Minute
		earliest, latest := now.Add(notice), now.AddDate(0, 0, settings.MaxDaysAhead)
		busy, err := s.busy(ctx, tx, row.HostUserID, row.ID, earliest.Add(-24*time.Hour), latest.Add(24*time.Hour))
		if err != nil {
			return Page{}, err
		}
		out.Slots = Slots(settings, loc, length, earliest, latest, busy)
	}
	// A calendar not read for a while is read again for the next visitor.
	if sched.CalendarUrl != nil {
		if n, err := store.New(tx).AskCalendarRead(ctx, row.HostUserID); err != nil {
			return Page{}, err
		} else if n > 0 && (sched.CalendarReadAt == nil || now.Sub(*sched.CalendarReadAt) > 15*time.Minute) {
			if _, err := jobqueue.Enqueue(ctx, tx, row.TenantID, api.JobKindSchedulingRefreshCalendar, api.SchedulingRefreshCalendarJob{UserId: row.HostUserID}); err != nil {
				return Page{}, err
			}
		}
	}
	return out, nil
}

// PublicPage returns a booking page.
func (s *Service) PublicPage(ctx context.Context, token string) (Page, error) {
	p, company, hash, err := s.tenant(ctx, token)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.BookingLinkByToken(ctx, hash)
		if err != nil {
			return err
		}
		sched, err := q.GetSchedule(ctx, row.HostUserID)
		if err != nil {
			return err
		}
		out, err = s.page(ctx, tx, company, row, sched)
		return err
	})
	return out, err
}

// Book takes a time (or moves the booked one) and sends both calendars the
// invitation. The host's schedule is locked meanwhile, so two people cannot
// take the same time.
func (s *Service) Book(ctx context.Context, token string, startsAt time.Time, client string) (Page, error) {
	if !s.limiter.Allow(client) {
		return Page{}, apperr.New(api.ErrorCodeBOOKINGTOOMANYTRIES, "Too many tries from your address; try again in an hour.")
	}
	p, company, hash, err := s.tenant(ctx, token)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.LockBookingLink(ctx, hash)
		if err != nil {
			return err
		}
		sched, err := q.LockSchedule(ctx, row.HostUserID)
		if err != nil {
			return err
		}
		before, err := s.page(ctx, tx, company, row, sched)
		if err != nil {
			return err
		}
		if before.Status != "open" && !before.CanChange {
			return apperr.New(api.ErrorCodeBOOKINGCLOSED, "This booking link can no longer be used: ask for a new one.")
		}
		if !slices.ContainsFunc(before.Slots, func(t time.Time) bool { return t.Equal(startsAt) }) {
			return apperr.New(api.ErrorCodeBOOKINGSLOTTAKEN, "That time is no longer free: pick another.")
		}
		moved := row.Status == "booked"
		if row, err = q.Book(ctx, store.BookParams{ID: row.ID, StartsAt: &startsAt}); err != nil {
			return err
		}
		cand, err := s.deps.Candidates.Person(ctx, tx, p, row.CandidateID)
		if err != nil {
			return err
		}
		if err := s.sendInvites(ctx, tx, row.TenantID, row, "REQUEST", cand); err != nil {
			return err
		}
		action := "booking_link.booked"
		if moved {
			action = "booking_link.moved"
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: action, EntityType: "candidate", EntityID: row.CandidateID,
			Changes: map[string]any{"title": row.Title, "startsAt": startsAt.UTC()},
		}); err != nil {
			return err
		}
		out, err = s.page(ctx, tx, company, row, sched)
		return err
	})
	return out, err
}
