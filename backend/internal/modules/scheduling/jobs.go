package scheduling

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/safehttp"
)

const (
	calendarMaxBytes = 5 << 20
	calendarAhead    = 61 * 24 * time.Hour
)

// refreshCalendar handles scheduling.refresh_calendar: reads the user's
// calendar address and keeps when they are busy (never what the events are).
// A calendar that cannot be read keeps its last busy times and says why.
func (s *Service) refreshCalendar(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var r api.SchedulingRefreshCalendarJob
	if err := job.Decode(&r); err != nil {
		return err
	}
	q := store.New(tx)
	sched, err := q.GetSchedule(ctx, r.UserId)
	if database.IsNoRows(err) || (err == nil && sched.CalendarUrl == nil) {
		return nil // removed since
	}
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(sched.TimeZone)
	if err != nil {
		loc = s.settings.DefaultTimeZone
	}
	busy, readErr := s.readCalendar(ctx, *sched.CalendarUrl, loc)
	if readErr != nil {
		msg := readErr.Error()
		logx.From(ctx).Info("calendar not read", "user_id", r.UserId, "error", msg) // never the address
		return q.SaveCalendarRead(ctx, store.SaveCalendarReadParams{UserID: r.UserId, CalendarStatus: ptr("failed"), CalendarError: &msg})
	}
	if err := q.ClearBusy(ctx, r.UserId); err != nil {
		return err
	}
	starts, ends := make([]time.Time, len(busy)), make([]time.Time, len(busy))
	for i, b := range busy {
		starts[i], ends[i] = b.Start, b.End
	}
	if err := q.InsertBusy(ctx, store.InsertBusyParams{UserID: r.UserId, Starts: starts, Ends: ends}); err != nil {
		return err
	}
	return q.SaveCalendarRead(ctx, store.SaveCalendarReadParams{UserID: r.UserId, CalendarStatus: ptr("ok")})
}

func (s *Service) readCalendar(ctx context.Context, address string, loc *time.Location) ([]ical.Interval, error) {
	client := safehttp.Client(safehttp.Options{AllowPrivate: s.settings.AllowPrivateNetworks, Timeout: 20 * time.Second, FollowRedirect: true})
	data, err := safehttp.Get(ctx, client, address, "HiringPlug-Calendar/1.0", "text/calendar, */*;q=0.5", calendarMaxBytes)
	if err != nil {
		var status safehttp.StatusError
		if errors.As(err, &status) {
			return nil, errors.New("the calendar address answered " + status.Error() + ": check it, or make a new one")
		}
		return nil, errors.New("the calendar could not be reached")
	}
	now := s.now()
	busy, err := ical.Busy(data, now.Add(-24*time.Hour), now.Add(calendarAhead), loc)
	if err != nil {
		if strings.Contains(err.Error(), "not a calendar") {
			return nil, errors.New("the address does not give a calendar: use the private address in iCal format")
		}
		return nil, err
	}
	return busy, nil
}

func ptr(s string) *string { return &s }
