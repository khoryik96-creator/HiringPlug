package calendars

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Attendee is independent of the HTTP contract and provider representations.
type Attendee struct {
	Email string  `json:"email"`
	Name  *string `json:"name,omitempty"`
}

// Input is deliberately independent of Graph's representation.
type Input struct {
	Title, Notes, Location, TimeZone string
	Start, End                       time.Time
	AllDay, Teams                    bool
	Attendees                        []Attendee
}

func normalize(in Input) (Input, error) {
	var errs apperr.FieldErrors
	in.Title = strings.TrimSpace(in.Title)
	in.Notes = strings.TrimSpace(in.Notes)
	in.Location = strings.TrimSpace(in.Location)
	if in.Title == "" {
		errs.Add("title", "enter a title")
	}
	if in.Start.IsZero() || !in.End.After(in.Start) || in.End.Sub(in.Start) > 31*24*time.Hour {
		errs.Add("endsAt", "must be after the start and within 31 days")
	}
	loc, err := time.LoadLocation(in.TimeZone)
	if err != nil {
		errs.Add("timeZone", "choose a valid time zone")
	}
	if in.AllDay && loc != nil {
		a, b := in.Start.In(loc), in.End.In(loc)
		if a.Hour() != 0 || a.Minute() != 0 || a.Second() != 0 || a.Nanosecond() != 0 || b.Hour() != 0 || b.Minute() != 0 || b.Second() != 0 || b.Nanosecond() != 0 {
			errs.Add("allDay", "all-day events must start and end at midnight in the selected time zone")
		}
	}
	seen := map[string]bool{}
	out := make([]Attendee, 0, len(in.Attendees))
	for _, a := range in.Attendees {
		address := strings.ToLower(strings.TrimSpace(a.Email))
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
			errs.Add("attendees", "enter valid email addresses")
			continue
		}
		if !seen[address] {
			seen[address] = true
			a.Email = address
			out = append(out, a)
		}
	}
	in.Attendees = out
	return in, errs.Err()
}
func checkRange(from, to time.Time) error {
	if !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		return apperr.Validation(apperr.FieldErrors{{Field: "to", Message: "choose a range of up to 62 days after the start"}})
	}
	return nil
}
func stableTransactionID(id string) string {
	h := sha256.Sum256([]byte("hiringplug-calendar:" + id))
	raw := hex.EncodeToString(h[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[:8], raw[8:12], raw[12:16], raw[16:20], raw[20:])
}
func safeWebURL(raw string) *string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if host != "outlook.office.com" && host != "outlook.office365.com" && host != "outlook.live.com" && host != "teams.microsoft.com" && host != "teams.live.com" {
		return nil
	}
	return &raw
}
func bounded(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
