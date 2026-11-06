package scheduling

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// linkLife is how long a booking link stays open.
const linkLife = 30 * 24 * time.Hour

// Service holds the module's use cases.
type Service struct {
	pool     *pgxpool.Pool
	deps     Deps
	settings Config
	now      func() time.Time
	limiter  *ratelimit.Limiter
}

// Schedule is a user's schedule as shown.
type Schedule struct {
	Set      bool
	Settings Settings
	Calendar *Calendar
}

// Calendar is a user's calendar address as shown: never the address.
type Calendar struct {
	Host, Status string
	Error        *string
	ReadAt       *time.Time
	BusyCount    int
}

func settingsFrom(row store.Schedule) (Settings, error) {
	s := Settings{TimeZone: row.TimeZone, BufferMinutes: int(row.BufferMinutes), MinNoticeHours: int(row.MinNoticeHours), MaxDaysAhead: int(row.MaxDaysAhead)}
	err := json.Unmarshal(row.Hours, &s.Hours)
	return s, err
}

func (s *Service) schedule(ctx context.Context, q *store.Queries, userID string) (Schedule, error) {
	row, err := q.GetSchedule(ctx, userID)
	if database.IsNoRows(err) {
		return Schedule{Settings: DefaultSettings(s.settings.DefaultTimeZone.String())}, nil
	}
	if err != nil {
		return Schedule{}, err
	}
	out := Schedule{Set: true}
	if out.Settings, err = settingsFrom(row); err != nil {
		return Schedule{}, err
	}
	if row.CalendarUrl != nil {
		c := &Calendar{Host: Host(*row.CalendarUrl), Status: "pending", Error: row.CalendarError, ReadAt: row.CalendarReadAt}
		if row.CalendarStatus != nil {
			c.Status = *row.CalendarStatus
		}
		n, err := q.CountBusy(ctx, userID)
		if err != nil {
			return Schedule{}, err
		}
		c.BusyCount = int(n)
		out.Calendar = c
	}
	return out, nil
}

// MySchedule returns the user's schedule (the default one before they save
// theirs).
func (s *Service) MySchedule(ctx context.Context, p authz.Principal) (Schedule, error) {
	var out Schedule
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.schedule(ctx, store.New(tx), p.UserID)
		return err
	})
	return out, err
}

// CalendarChange is what to do with the calendar address: keep it (nil),
// remove it (Remove) or set it.
type CalendarChange struct {
	Remove bool
	URL    string
}

// PutMySchedule saves the user's schedule and, when given, their calendar
// address, whose busy times are then read in the background.
func (s *Service) PutMySchedule(ctx context.Context, p authz.Principal, in Settings, cal *CalendarChange) (Schedule, error) {
	in, _, err := CheckSettings(in)
	if err != nil {
		return Schedule{}, err
	}
	if cal != nil && !cal.Remove {
		if cal.URL, err = CalendarURL(cal.URL, s.settings.AllowPrivateNetworks); err != nil {
			return Schedule{}, err
		}
	}
	hours, err := json.Marshal(in.Hours)
	if err != nil {
		return Schedule{}, err
	}
	var out Schedule
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.PutSchedule(ctx, store.PutScheduleParams{
			UserID: p.UserID, TimeZone: in.TimeZone, Hours: hours, BufferMinutes: int32(in.BufferMinutes), //nolint:gosec // checked by the contract
			MinNoticeHours: int32(in.MinNoticeHours), MaxDaysAhead: int32(in.MaxDaysAhead), //nolint:gosec // checked by the contract
		}); err != nil {
			return err
		}
		changes := map[string]any{"timeZone": in.TimeZone, "hours": in.Hours}
		if cal != nil {
			var url *string
			if !cal.Remove {
				url = &cal.URL
			}
			if err := q.SetCalendarURL(ctx, store.SetCalendarURLParams{UserID: p.UserID, CalendarUrl: url}); err != nil {
				return err
			}
			if cal.Remove {
				if err := q.ClearBusy(ctx, p.UserID); err != nil {
					return err
				}
			} else if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindSchedulingRefreshCalendar, api.SchedulingRefreshCalendarJob{UserId: p.UserID}); err != nil {
				return err
			}
			changes["calendar"] = map[string]any{"changed": true, "redacted": true} // the address is a secret
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "schedule.updated", EntityType: "user", EntityID: p.UserID, Changes: changes}); err != nil {
			return err
		}
		out, err = s.schedule(ctx, q, p.UserID)
		return err
	})
	return out, err
}

// Link is a booking link as shown.
type Link struct {
	Row           store.BookingLink
	URL           *string // only when just made
	CandidateName string
	HostName      string
	Status        string
}

// CreateLink makes a booking link for a candidate, hosted by the user.
func (s *Service) CreateLink(ctx context.Context, p authz.Principal, in api.BookingLinkInput) (Link, error) {
	token, hash, err := NewToken()
	if err != nil {
		return Link{}, err
	}
	var out Link
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.GetSchedule(ctx, p.UserID); database.IsNoRows(err) {
			return apperr.New(api.ErrorCodeSCHEDULINGNOTSETUP, "Set your hours for calls first (Account > Booking hours).")
		} else if err != nil {
			return err
		}
		cand, err := s.deps.Candidates.Person(ctx, tx, p, in.CandidateId)
		if err != nil {
			return err
		}
		if in.ApplicationId != nil {
			owner, err := s.deps.Applications.CandidateOf(ctx, tx, *in.ApplicationId)
			if err != nil {
				return err
			}
			if owner != in.CandidateId {
				return apperr.Validation(apperr.FieldErrors{{Field: "applicationId", Message: "is not one of this candidate's"}})
			}
		}
		host, err := s.deps.Users.Person(ctx, tx, p.UserID)
		if err != nil {
			return err
		}
		row, err := q.InsertBookingLink(ctx, store.InsertBookingLinkParams{
			TokenHash: hash, HostUserID: p.UserID, CandidateID: in.CandidateId, ApplicationID: in.ApplicationId,
			Title: in.Title, DurationMinutes: int32(in.DurationMinutes), Location: emptyNil(in.Location), //nolint:gosec // an enum
			ExpiresAt: s.now().Add(linkLife),
		})
		if err != nil {
			return err
		}
		url := s.settings.PublicURL + "/book/" + token
		out = Link{Row: row, URL: &url, CandidateName: cand.Name, Status: row.Status}
		if host != nil {
			out.HostName = host.Name
		}
		return audit.Record(ctx, tx, audit.Entry{
			Action: "booking_link.created", EntityType: "candidate", EntityID: in.CandidateId,
			Changes: map[string]any{"title": in.Title, "durationMinutes": in.DurationMinutes},
		})
	})
	return out, err
}

// Links returns a candidate's booking links.
func (s *Service) Links(ctx context.Context, p authz.Principal, candidateID string) ([]Link, error) {
	var out []Link
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		cand, err := s.deps.Candidates.Person(ctx, tx, p, candidateID)
		if err != nil {
			return err
		}
		rows, err := store.New(tx).CandidateBookingLinks(ctx, candidateID)
		if err != nil {
			return err
		}
		names := map[string]string{}
		for _, r := range rows {
			if _, ok := names[r.HostUserID]; !ok {
				if h, err := s.deps.Users.Person(ctx, tx, r.HostUserID); err != nil {
					return err
				} else if h != nil {
					names[r.HostUserID] = h.Name
				}
			}
			out = append(out, Link{Row: r, CandidateName: cand.Name, HostName: names[r.HostUserID], Status: Status(r.Status, r.ExpiresAt, s.now())})
		}
		return nil
	})
	return out, err
}

// CancelLink cancels a booking link; a booked meeting is taken off both
// calendars.
func (s *Service) CancelLink(ctx context.Context, p authz.Principal, id string) (Link, error) {
	var out Link
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetBookingLink(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("booking link")
		}
		if err != nil {
			return err
		}
		cand, err := s.deps.Candidates.Person(ctx, tx, p, row.CandidateID)
		if err != nil {
			return err
		}
		wasBooked := row.Status == "booked"
		if row.Status != "cancelled" {
			if row, err = q.CancelBookingLink(ctx, id); err != nil {
				return err
			}
			if wasBooked {
				if err := s.sendInvites(ctx, tx, p.TenantID, row, "CANCEL", cand); err != nil {
					return err
				}
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "booking_link.cancelled", EntityType: "candidate", EntityID: row.CandidateID,
				Changes: map[string]any{"title": row.Title, "wasBooked": wasBooked},
			}); err != nil {
				return err
			}
		}
		out = Link{Row: row, CandidateName: cand.Name, Status: row.Status}
		if h, err := s.deps.Users.Person(ctx, tx, row.HostUserID); err != nil {
			return err
		} else if h != nil {
			out.HostName = h.Name
		}
		return nil
	})
	return out, err
}

func emptyNil(s nullable.Nullable[string]) *string {
	v, err := s.Get()
	if err != nil || strings.TrimSpace(v) == "" {
		return nil
	}
	v = strings.TrimSpace(v)
	return &v
}
