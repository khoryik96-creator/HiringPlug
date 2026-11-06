package scheduling

// The scheduling rules, as plain functions on plain values: no database, no
// HTTP.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

// Hours is a weekly window for calls: day 1 = Monday ... 7 = Sunday,
// start and end "HH:MM" in the user's time zone.
type Hours struct {
	Day   int    `json:"day"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Settings is a user's schedule for calls.
type Settings struct {
	TimeZone       string
	Hours          []Hours
	BufferMinutes  int
	MinNoticeHours int
	MaxDaysAhead   int
}

// DefaultSettings is what a user starts from: weekdays 9 to 5.
func DefaultSettings(timeZone string) Settings {
	s := Settings{TimeZone: timeZone, BufferMinutes: 10, MinNoticeHours: 12, MaxDaysAhead: 14}
	for d := 1; d <= 5; d++ {
		s.Hours = append(s.Hours, Hours{Day: d, Start: "09:00", End: "17:00"})
	}
	return s
}

func minutes(hhmm string) int {
	var h, m int
	_, _ = fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return h*60 + m
}

// CheckSettings checks a schedule: a known time zone, and windows that end
// after they start and do not overlap on a day. Windows come back sorted.
func CheckSettings(s Settings) (Settings, *time.Location, error) {
	var errs apperr.FieldErrors
	s.TimeZone = strings.TrimSpace(s.TimeZone)
	loc, err := time.LoadLocation(s.TimeZone)
	if err != nil || s.TimeZone == "" || s.TimeZone == "Local" {
		errs.Add("timeZone", "must be a time zone, e.g. Asia/Kuala_Lumpur")
	}
	sort.Slice(s.Hours, func(i, j int) bool {
		if s.Hours[i].Day != s.Hours[j].Day {
			return s.Hours[i].Day < s.Hours[j].Day
		}
		return minutes(s.Hours[i].Start) < minutes(s.Hours[j].Start)
	})
	for i, h := range s.Hours {
		if minutes(h.End) <= minutes(h.Start) {
			errs.Add(fmt.Sprintf("hours[%d]", i), "must end after it starts")
		}
		if i > 0 && s.Hours[i-1].Day == h.Day && minutes(s.Hours[i-1].End) > minutes(h.Start) {
			errs.Add(fmt.Sprintf("hours[%d]", i), "overlaps the hours before it on that day")
		}
	}
	return s, loc, errs.Err()
}

// CalendarURL checks a calendar's private address: https (webcal:// is how
// Outlook and Apple write it), with a host. Plain http only for local
// testing (allowHTTP).
func CalendarURL(raw string, allowHTTP bool) (string, error) {
	s := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(strings.ToLower(s), "webcal://"); ok {
		s = "https://" + s[len(s)-len(rest):]
	}
	u, err := url.Parse(s)
	plainHTTP := allowHTTP && u != nil && u.Scheme == "http"
	if err != nil || (u.Scheme != "https" && !plainHTTP) || u.Host == "" || u.User != nil {
		return "", apperr.Validation(apperr.FieldErrors{{Field: "calendarUrl",
			Message: "must be your calendar's private address (https://... or webcal://..., ending in .ics)"}})
	}
	return u.String(), nil
}

// Host is what is shown of a calendar address: its host only (the address
// itself gives anyone who has it the whole calendar).
func Host(address string) string {
	if u, err := url.Parse(address); err == nil {
		return u.Hostname()
	}
	return ""
}

// step is how far apart the offered start times are.
func step(length time.Duration) time.Duration {
	if length < 30*time.Minute {
		return 15 * time.Minute
	}
	return 30 * time.Minute
}

// Slots are the free start times for a meeting of the given length within
// the weekly hours, between earliest and latest (inclusive of a start at
// latest), keeping buffer free before and after every busy time.
func Slots(s Settings, loc *time.Location, length time.Duration, earliest, latest time.Time, busy []ical.Interval) []time.Time {
	buffer := time.Duration(s.BufferMinutes) * time.Minute
	every := step(length)
	var out []time.Time
	first := earliest.In(loc)
	day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, loc)
	for ; !day.After(latest); day = day.AddDate(0, 0, 1) {
		weekday := int(day.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		for _, h := range s.Hours {
			if h.Day != weekday {
				continue
			}
			from := minutes(h.Start)
			end := time.Date(day.Year(), day.Month(), day.Day(), 0, minutes(h.End), 0, 0, loc)
			for start := time.Date(day.Year(), day.Month(), day.Day(), 0, from, 0, 0, loc); !start.Add(length).After(end); start = start.Add(every) {
				if start.Before(earliest) || start.After(latest) {
					continue
				}
				if free(start, start.Add(length), buffer, busy) {
					out = append(out, start)
				}
			}
		}
	}
	return out
}

func free(start, end time.Time, buffer time.Duration, busy []ical.Interval) bool {
	for _, b := range busy {
		if b.Start.Before(end.Add(buffer)) && b.End.After(start.Add(-buffer)) {
			return false
		}
	}
	return true
}

// NewToken is a booking link's secret: 32 random bytes, URL-safe. Only its
// hash is stored.
func NewToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is what is stored of a token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Status is a link's status as shown: open links past their date have expired.
func Status(stored string, expiresAt, now time.Time) string {
	if stored == "open" && now.After(expiresAt) {
		return "expired"
	}
	return stored
}

// CanChange reports whether a booked meeting may still be moved: not once
// it is closer than the minimum notice.
func CanChange(status string, startsAt *time.Time, minNotice time.Duration, now time.Time) bool {
	return status == "booked" && startsAt != nil && startsAt.After(now.Add(minNotice))
}

// When writes a meeting's time for an email: "Tuesday 10 November 2026,
// 10:00–10:30 (Asia/Kuala_Lumpur)".
func When(start time.Time, length time.Duration, loc *time.Location) string {
	s := start.In(loc)
	return fmt.Sprintf("%s, %s–%s (%s)", s.Format("Monday 2 January 2006"), s.Format("15:04"), s.Add(length).Format("15:04"), loc.String())
}
