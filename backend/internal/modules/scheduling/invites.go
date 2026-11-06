package scheduling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

// sendInvites emails the meeting to the candidate and to the host as a
// calendar invitation (REQUEST to add or move it, CANCEL to remove it):
// Gmail and Outlook add it to the calendar, which blocks the time. The
// invitation comes from the server's address in the company's name, so
// both are attendees and either calendar takes it.
func (s *Service) sendInvites(ctx context.Context, tx pgx.Tx, tenantID string, row store.BookingLink, method string, cand Person) error {
	if row.StartsAt == nil {
		return nil
	}
	host, err := s.deps.Users.Person(ctx, tx, row.HostUserID)
	if err != nil {
		return err
	}
	company, err := s.deps.Users.TenantName(ctx, tx, tenantID)
	if err != nil {
		return err
	}
	loc := s.settings.DefaultTimeZone
	if sched, err := store.New(tx).GetSchedule(ctx, row.HostUserID); err == nil {
		if l, err := time.LoadLocation(sched.TimeZone); err == nil {
			loc = l
		}
	}
	length := time.Duration(row.DurationMinutes) * time.Minute
	hostName := "the recruiter"
	in := ical.Invite{
		Method: method, UID: row.ID + "@hiringplug", Sequence: int(row.Sequence),
		Organizer: ical.Person{Name: company, Email: s.settings.OrganizerEmail},
		Start:     *row.StartsAt, End: row.StartsAt.Add(length), Summary: row.Title,
	}
	if row.Location != nil {
		in.Location = *row.Location
	}
	if host != nil && host.Email != nil {
		hostName = host.Name
		in.Attendees = append(in.Attendees, ical.Person{Name: host.Name, Email: *host.Email})
	}
	if cand.Email != nil {
		in.Attendees = append(in.Attendees, ical.Person{Name: cand.Name, Email: *cand.Email})
	}
	when := When(*row.StartsAt, length, loc)
	var lines []string
	subject := row.Title + ": " + when
	switch method {
	case "CANCEL":
		subject = "Cancelled: " + subject
		lines = append(lines, fmt.Sprintf("%s on %s is cancelled.", row.Title, when))
	default:
		lines = append(lines, fmt.Sprintf("%s with %s (%s), %s.", row.Title, hostName, company, when))
		if in.Location != "" {
			lines = append(lines, "Where: "+in.Location)
		}
		if row.Sequence > 1 {
			subject = "Moved: " + subject
		}
	}
	in.Body = strings.Join(lines, "\n")
	calendar := in.Render(s.now())
	body := in.Body + "\n\nThe invitation is attached: add it to your calendar."
	app := row.ApplicationID
	if cand.Email != nil {
		cid := row.CandidateID
		if err := s.deps.Mailer.Send(ctx, tx, tenantID, Mail{
			To: *cand.Email, Subject: subject, Body: body + "\nTo move it, open the booking link again.", Calendar: calendar,
			CandidateID: &cid, ApplicationID: app,
		}); err != nil {
			return err
		}
	}
	if host != nil && host.Email != nil {
		return s.deps.Mailer.Send(ctx, tx, tenantID, Mail{
			To: *host.Email, Subject: subject + " (" + cand.Name + ")", Body: fmt.Sprintf("Booked by %s.\n\n%s", cand.Name, body), Calendar: calendar,
		})
	}
	return nil
}
