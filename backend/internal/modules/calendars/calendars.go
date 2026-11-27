// Package calendars owns personal calendars and Outlook connections. Other
// schedulers receive only intervals; event details and tokens remain private.
package calendars

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

type Application struct{ ClientID, ClientSecret string }
type Meeting struct {
	Kind, ID, Title, Location         string
	ApplicationID, Mode, Instructions string
	Version                           int
	Users                             []string
	Start, End                        time.Time
	Cancelled, Scheduled              bool
}

// MeetingChange is what an interview's organizer changed in Outlook to its
// invitation (nil: unchanged).
type MeetingChange struct {
	Title, Location, Instructions *string
	Start, End                    *time.Time
}
type Deps struct {
	Apps interface {
		MicrosoftApp(context.Context, pgx.Tx, string) (Application, error)
	}
	Sources interface {
		Meeting(context.Context, pgx.Tx, string, string) (Meeting, error)
		MeetingsFor(context.Context, pgx.Tx, string, time.Time, time.Time) ([]Meeting, error)
		// Existing says which of these interviews or bookings still stand
		// (an interview not deleted or cancelled, a link still booked):
		// holds of the others are stale.
		Existing(ctx context.Context, tx pgx.Tx, kind string, ids []string) (map[string]bool, error)
	}
	Users interface {
		CalendarUserActive(context.Context, pgx.Tx, string) (bool, error)
		PrincipalFor(context.Context, pgx.Tx, string) (authz.Principal, bool, error)
	}
	Recipients interface {
		InterviewRecipient(context.Context, pgx.Tx, string) (email string, optedOut bool, err error)
	}
	ReminderMail interface {
		// QueueInterviewReminder(tenant, application, organizer, email, at, reminder): sent as the organizer.
		QueueInterviewReminder(context.Context, pgx.Tx, string, string, string, string, time.Time, api.InterviewReminderDelivery) error
	}
	// Interviews takes into an interview what its organizer changed in Outlook
	// (title, place, instructions, time), in the caller's transaction, and
	// answers with the interview as it is now; organizer is the user who
	// changed it (for the audit). Without it, an invitation moved in Outlook
	// is flagged for review and its other changes are left as they are.
	Interviews interface {
		ChangeFromOutlook(ctx context.Context, tx pgx.Tx, tenant, id, organizer string, version int, c MeetingChange) (Meeting, error)
	}
}
type Provider struct{ AuthorizeURL, TokenURL, GraphURL string }
type Config struct {
	KeysSecret []byte
	PublicURL  string
	HTTP       *http.Client
	Provider   Provider
	TimeZone   *time.Location
}
type Module struct {
	service *Service
	HTTP    *HTTP
}

func New(pool *pgxpool.Pool) *Module {
	s := &Service{pool: pool, now: time.Now, netSlots: make(chan struct{}, networkConns)}
	return &Module{service: s, HTTP: &HTTP{svc: s}}
}
func (m *Module) ConnectOutlookWith(deps Deps, c Config) {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	client := *c.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("calendar provider redirect refused") }
	c.HTTP = &client
	if c.Provider.AuthorizeURL == "" {
		c.Provider = Provider{AuthorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token", GraphURL: "https://graph.microsoft.com/v1.0"} //nolint:gosec // public Microsoft OAuth endpoints, not credentials
	}
	if deps.Apps != nil {
		m.service.deps.Apps = deps.Apps
	}
	if deps.Sources != nil {
		m.service.deps.Sources = deps.Sources
	}
	if deps.Users != nil {
		m.service.deps.Users = deps.Users
	}
	if deps.Recipients != nil {
		m.service.deps.Recipients = deps.Recipients
	}
	if deps.ReminderMail != nil {
		m.service.deps.ReminderMail = deps.ReminderMail
	}
	if deps.Interviews != nil {
		m.service.deps.Interviews = deps.Interviews
	}
	if c.TimeZone == nil {
		c.TimeZone = m.service.config.TimeZone
		if c.TimeZone == nil {
			c.TimeZone = time.UTC
		}
	}
	m.service.config = c
}
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindCalendarSourceChanged: m.service.sourceChanged}
}

// OutsideJobHandlers are the jobs that talk to Outlook (up to minutes): they
// run with no queue transaction open, each step in its own short one.
func (m *Module) OutsideJobHandlers() jobqueue.OutsideHandlers {
	return jobqueue.OutsideHandlers{api.JobKindCalendarRefresh: m.service.refresh, api.JobKindCalendarPush: m.service.push}
}
func (m *Module) SourceChanged(ctx context.Context, tx pgx.Tx, tenant, kind, id string) error {
	_, err := jobqueue.Enqueue(ctx, tx, tenant, api.JobKindCalendarSourceChanged, api.CalendarSourceJob{Kind: api.CalendarSourceJobKind(kind), SourceId: id})
	return err
}

// InterviewsDeleted is told, in the deleting transaction, of interviews
// deleted for good (with their candidate or application): their Outlook
// invitations are cancelled, and their rows deleted, once it commits.
func (m *Module) InterviewsDeleted(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tenant, err := store.New(tx).CurrentCalendarTenant(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.SourceChanged(ctx, tx, tenant, "interview", id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) BusyFor(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptKind, exceptID string) ([]ical.Interval, error) {
	return m.service.busy(ctx, tx, users, from, to, exceptKind, exceptID)
}

// Conflicts are the interviewers busy by their calendars (the interviews in
// named aside: the clash check names those) and those whose Outlook has
// stopped syncing, for scheduling an interview.
func (m *Module) Conflicts(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptInterview string, named []string) (busy, stale []string, err error) {
	return m.service.conflicts(ctx, tx, users, from, to, exceptInterview, named)
}

// Connected is whether the user's Outlook is working, so it sends them their
// invitations; with a failing or broken connection HyPlug emails them.
func (m *Module) Connected(ctx context.Context, tx pgx.Tx, user string) (bool, error) {
	rows, err := store.New(tx).OutlookHealthForUsers(ctx, []string{user})
	if err != nil {
		return false, err
	}
	for _, h := range rows {
		if h.Status == "ok" || h.Status == "pending" {
			return true, nil
		}
	}
	return false, nil
}
