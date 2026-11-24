package auditlog

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/auditlog/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the module's use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	loc  *time.Location
	now  func() time.Time
}

// record counts one showing of a salary to the transaction's user (the
// database function leaves out the system).
func (s *Service) record(ctx context.Context, tx pgx.Tx, kind string, candidateIDs []string) error {
	if !IsKind(kind) {
		return fmt.Errorf("auditlog: unknown salary view kind %q", kind)
	}
	ids := Distinct(candidateIDs)
	if len(ids) == 0 {
		return nil
	}
	now := s.now()
	day := pgtype.Date{Time: Day(now, s.loc), Valid: true}
	q := store.New(tx)
	if Grouped(kind) {
		return q.RecordSalaryView(ctx, store.RecordSalaryViewParams{Kind: kind, CandidateIds: ids, Day: day, At: now})
	}
	for _, id := range ids {
		if err := q.RecordSalaryView(ctx, store.RecordSalaryViewParams{Kind: kind, CandidateID: &id, CandidateIds: []string{}, Day: day, At: now}); err != nil {
			return err
		}
	}
	return nil
}

// --- The audit log --------------------------------------------------------

// LogFilter narrows the audit log; each field is optional. From and To are
// days in the agency's time zone, both included.
type LogFilter struct {
	Actor, Action, EntityType, EntityID *string
	From, To                            *time.Time
}

func (s *Service) auditFilter(f LogFilter) audit.Filter {
	action, kind := SplitAction(f.Action)
	from, to := DayRange(f.From, f.To, s.loc)
	return audit.Filter{Actor: f.Actor, Action: action, ActionKind: kind, EntityType: f.EntityType, EntityID: f.EntityID, From: from, To: to}
}

// Entry is an audit log entry with who did it and what it is about, named.
type Entry struct {
	audit.Logged
	ActorKind, ActorName string
	EntityName           *string
	CandidateID          *string // a candidate on a job: the candidate the record opens
}

// LogPage is one page of the audit log, newest first.
type LogPage struct {
	Items      []Entry
	NextCursor *string
}

// Log returns a page of the company's audit log (as p may see it).
func (s *Service) Log(ctx context.Context, p authz.Principal, f LogFilter, cursor *string, limit int) (LogPage, error) {
	var out LogPage
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.logPage(ctx, tx, s.auditFilter(f), cursor, limit)
		return err
	})
	return out, err
}

func (s *Service) logPage(ctx context.Context, tx pgx.Tx, f audit.Filter, cursor *string, limit int) (LogPage, error) {
	logged, more, err := audit.Page(ctx, tx, f, cursor, limit)
	if err != nil {
		return LogPage{}, err
	}
	actors := make([]string, len(logged))
	byType := map[string][]string{}
	for i, l := range logged {
		actors[i] = l.ActorID
		byType[l.EntityType] = append(byType[l.EntityType], l.EntityID)
	}
	users, keys, err := s.actorNames(ctx, tx, append(actors, byType["user"]...))
	if err != nil {
		return LogPage{}, err
	}
	records := map[string]map[string]string{"user": users, "api_key": keys}
	for _, kind := range RecordKinds {
		lookup, ok := s.deps.Records[kind]
		if !ok || len(byType[kind]) == 0 {
			continue
		}
		if records[kind], err = lookup.Names(ctx, tx, Distinct(byType[kind])); err != nil {
			return LogPage{}, err
		}
	}
	apps := map[string]Application{}
	if len(byType["application"]) > 0 && s.deps.Applications != nil {
		if apps, err = s.deps.Applications.Applications(ctx, tx, Distinct(byType["application"])); err != nil {
			return LogPage{}, err
		}
	}
	if len(byType["api_key"]) > 0 && s.deps.Keys != nil {
		if records["api_key"], err = s.deps.Keys.KeyNames(ctx, tx, Distinct(byType["api_key"])); err != nil {
			return LogPage{}, err
		}
	}
	out := LogPage{Items: make([]Entry, len(logged))}
	for i, l := range logged {
		e := Entry{Logged: l, ActorKind: ActorKind(l.ActorID), ActorName: ActorName(l.ActorID, users, keys)}
		if name, ok := records[l.EntityType][l.EntityID]; ok && name != "" {
			e.EntityName = &name
		}
		if app, ok := apps[l.EntityID]; ok && l.EntityType == "application" {
			e.EntityName, e.CandidateID = &app.Name, &app.CandidateID
		}
		out.Items[i] = e
	}
	if more && len(logged) > 0 {
		last := logged[len(logged)-1].ID
		out.NextCursor = &last
	}
	return out, nil
}

// actorNames names the people and API keys among ids.
func (s *Service) actorNames(ctx context.Context, tx pgx.Tx, ids []string) (users, keys map[string]string, err error) {
	var people, apiKeys []string
	for _, id := range Distinct(ids) {
		switch ActorKind(id) {
		case ActorUser:
			people = append(people, id)
		case ActorAPIKey:
			apiKeys = append(apiKeys, id)
		}
	}
	users, keys = map[string]string{}, map[string]string{}
	if len(people) > 0 {
		if users, err = s.deps.Users.UserNames(ctx, tx, people); err != nil {
			return nil, nil, err
		}
	}
	if len(apiKeys) > 0 && s.deps.Keys != nil {
		if keys, err = s.deps.Keys.KeyNames(ctx, tx, apiKeys); err != nil {
			return nil, nil, err
		}
	}
	return users, keys, nil
}

// --- Salary views ------------------------------------------------------

// ViewFilter narrows the salary views; each field is optional. From and To
// are days in the agency's time zone, both included.
type ViewFilter struct {
	User, CandidateID, Kind *string
	From, To                *time.Time
}

// View is one line of the salary views: who was shown whose salary, where,
// on which day and how often.
type View struct {
	ID                string
	Day               time.Time
	UserID, UserName  string
	Kind              string
	CandidateID       *string
	CandidateName     *string
	Candidates, Count int
	FirstAt, LastAt   time.Time
}

// ViewPage is one page of salary views, newest day first.
type ViewPage struct {
	Items      []View
	NextCursor *string
}

func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// SalaryViews returns a page of who was shown a salary (as p may see it).
func (s *Service) SalaryViews(ctx context.Context, p authz.Principal, f ViewFilter, cursor *string, limit int) (ViewPage, error) {
	var out ViewPage
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.viewPage(ctx, tx, f, cursor, limit)
		return err
	})
	return out, err
}

func (s *Service) viewPage(ctx context.Context, tx pgx.Tx, f ViewFilter, cursor *string, limit int) (ViewPage, error) {
	rows, err := store.New(tx).ListSalaryViews(ctx, store.ListSalaryViewsParams{
		UserID: f.User, Kind: f.Kind, CandidateID: f.CandidateID, FromDay: date(f.From), ToDay: date(f.To), Cursor: cursor,
		RowLimit: int32(limit + 1), //nolint:gosec // 1..200 per the contract
	})
	if err != nil {
		return ViewPage{}, err
	}
	var out ViewPage
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		out.NextCursor = &last
	}
	who, candidates := make([]string, len(rows)), []string{}
	for i, r := range rows {
		who[i] = r.UserID
		if r.CandidateID != nil {
			candidates = append(candidates, *r.CandidateID)
		}
	}
	users, keys, err := s.actorNames(ctx, tx, who)
	if err != nil {
		return ViewPage{}, err
	}
	names := map[string]string{}
	if lookup, ok := s.deps.Records["candidate"]; ok && len(candidates) > 0 {
		if names, err = lookup.Names(ctx, tx, Distinct(candidates)); err != nil {
			return ViewPage{}, err
		}
	}
	out.Items = make([]View, len(rows))
	for i, r := range rows {
		v := View{
			ID: r.ID, Day: r.Day.Time, UserID: r.UserID, UserName: ActorName(r.UserID, users, keys), Kind: r.Kind,
			CandidateID: r.CandidateID, Candidates: int(r.Candidates), Count: int(r.Count), FirstAt: r.FirstAt, LastAt: r.LastAt,
		}
		if r.CandidateID != nil {
			if name, ok := names[*r.CandidateID]; ok && name != "" {
				v.CandidateName = &name
			}
		}
		out.Items[i] = v
	}
	return out, nil
}
