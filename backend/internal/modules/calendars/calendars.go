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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

type Application struct{ ClientID, ClientSecret string }
type Meeting struct {
	Kind, ID, Title, Location string
	Version                   int
	Users                     []string
	Start, End                time.Time
	Cancelled                 bool
}
type Deps struct {
	Apps interface {
		MicrosoftApp(context.Context, pgx.Tx, string) (Application, error)
	}
	Sources interface {
		Meeting(context.Context, pgx.Tx, string, string) (Meeting, error)
		MeetingsFor(context.Context, pgx.Tx, string, time.Time, time.Time) ([]Meeting, error)
	}
	Users interface {
		CalendarUserActive(context.Context, pgx.Tx, string) (bool, error)
	}
}
type Provider struct{ AuthorizeURL, TokenURL, GraphURL string }
type Config struct {
	KeysSecret []byte
	PublicURL  string
	HTTP       *http.Client
	Provider   Provider
}
type Module struct {
	service *Service
	HTTP    *HTTP
}

func New(pool *pgxpool.Pool) *Module {
	s := &Service{pool: pool, now: time.Now}
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
	m.service.config = c
}
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindCalendarRefresh: m.service.refresh, api.JobKindCalendarPush: m.service.push, api.JobKindCalendarSourceChanged: m.service.sourceChanged}
}
func (m *Module) SourceChanged(ctx context.Context, tx pgx.Tx, tenant, kind, id string) error {
	_, err := jobqueue.Enqueue(ctx, tx, tenant, api.JobKindCalendarSourceChanged, api.CalendarSourceJob{Kind: api.CalendarSourceJobKind(kind), SourceId: id})
	return err
}
func (m *Module) BusyFor(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptKind, exceptID string) ([]ical.Interval, error) {
	return m.service.busy(ctx, tx, users, from, to, exceptKind, exceptID)
}

func (m *Module) Connected(ctx context.Context, tx pgx.Tx, user string) (bool, error) {
	rows, err := store.New(tx).OutlookHealthForUsers(ctx, []string{user})
	if database.IsNoRows(err) {
		return false, nil
	}
	return len(rows) > 0, err
}
