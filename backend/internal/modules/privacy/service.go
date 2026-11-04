package privacy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service holds the privacy use cases.
type Service struct {
	pool *pgxpool.Pool
	deps Deps
	loc  *time.Location
	now  func() time.Time
}

// RequestView is a request with what is shown with it.
type RequestView struct {
	Request
	CandidateName string
	DueOn         *time.Time
	Overdue       bool
	DaysOpen      int
	ClosedByName  *string
}

// CandidatePrivacy is a candidate's consent (nil: none recorded) and requests.
type CandidatePrivacy struct {
	Consent  *Consent
	Requests []RequestView
}

// WithdrawnCandidate is a candidate who withdrew consent.
type WithdrawnCandidate struct {
	CandidateID, Name string
	Day               time.Time
}

// Overview is the data protection dashboard.
type Overview struct {
	Given, Withdrawn, NotRecorded int
	Open, Closed                  []RequestView
	WithdrawnList                 []WithdrawnCandidate
}

func (s *Service) today() time.Time { return Day(s.now(), s.loc) }

// Candidate returns a candidate's consent and requests.
func (s *Service) Candidate(ctx context.Context, p authz.Principal, candidateID string) (CandidatePrivacy, error) {
	var out CandidatePrivacy
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.candidateName(ctx, tx, candidateID); err != nil {
			return err
		}
		q := store.New(tx)
		if row, err := q.GetConsent(ctx, candidateID); err == nil {
			c := consentFromRow(row)
			out.Consent = &c
		} else if !database.IsNoRows(err) {
			return err
		}
		rows, err := q.ListCandidateRequests(ctx, candidateID)
		if err != nil {
			return err
		}
		out.Requests, err = s.views(ctx, tx, rows)
		return err
	})
	return out, err
}

// SetConsent records that a candidate gave or withdrew consent
// (expectedVersion 0 when none is recorded yet).
func (s *Service) SetConsent(ctx context.Context, p authz.Principal, candidateID string, expectedVersion int, in ConsentInput) (Consent, error) {
	in, err := CheckConsent(in, s.today())
	if err != nil {
		return Consent{}, err
	}
	var out Consent
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.candidateName(ctx, tx, candidateID); err != nil {
			return err
		}
		q := store.New(tx)
		var row store.CandidateConsent
		if expectedVersion == 0 {
			row, err = q.InsertConsent(ctx, store.InsertConsentParams{
				CandidateID: candidateID, Status: in.Status, Method: in.Method, Day: toDate(in.Day), Note: in.Note,
			})
		} else {
			row, err = q.UpdateConsent(ctx, store.UpdateConsentParams{
				CandidateID: candidateID, Status: in.Status, Method: in.Method, Day: toDate(in.Day), Note: in.Note,
				Version: int32(expectedVersion), //nolint:gosec // a version is small
			})
		}
		if database.IsNoRows(err) {
			return apperr.StaleVersion("consent")
		}
		if err != nil {
			return err
		}
		out = consentFromRow(row)
		// Not the note: it is free text about a person.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "consent.recorded", EntityType: "candidate", EntityID: candidateID,
			Changes: map[string]any{"status": in.Status, "method": in.Method, "day": in.Day.Format(time.DateOnly)},
		})
	})
	return out, err
}

// CreateRequest logs a request a candidate made about their data.
func (s *Service) CreateRequest(ctx context.Context, p authz.Principal, candidateID string, in RequestInput) (RequestView, error) {
	in, err := CheckRequest(in, s.today())
	if err != nil {
		return RequestView{}, err
	}
	var out RequestView
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if _, err := s.candidateName(ctx, tx, candidateID); err != nil {
			return err
		}
		row, err := store.New(tx).InsertRequest(ctx, store.InsertRequestParams{
			CandidateID: candidateID, Kind: in.Kind, ReceivedOn: toDate(in.ReceivedOn), Details: in.Details,
		})
		if err != nil {
			return err
		}
		views, err := s.views(ctx, tx, []store.PrivacyRequest{row})
		if err != nil {
			return err
		}
		out = views[0]
		return audit.Record(ctx, tx, audit.Entry{
			Action: "privacy.request_logged", EntityType: "privacy_request", EntityID: row.ID,
			Changes: map[string]any{"candidateId": candidateID, "kind": in.Kind, "receivedOn": in.ReceivedOn.Format(time.DateOnly)},
		})
	})
	return out, err
}

// Decide closes a request as done or declined (with what was done), or
// reopens it. Closing a withdraw-consent request as done records the
// candidate's consent as withdrawn, today; nothing is deleted.
func (s *Service) Decide(ctx context.Context, p authz.Principal, id string, version int, d Decision) (RequestView, error) {
	d, err := CheckDecision(d)
	if err != nil {
		return RequestView{}, err
	}
	var out RequestView
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.DecideRequest(ctx, store.DecideRequestParams{
			ID: id, Version: int32(version), Status: d.Status, Outcome: d.Outcome, //nolint:gosec // a version is small
		})
		if database.IsNoRows(err) {
			if _, err := q.GetRequest(ctx, id); database.IsNoRows(err) {
				return apperr.NotFound("request")
			}
			return apperr.StaleVersion("request")
		}
		if err != nil {
			return err
		}
		if row.Kind == KindWithdrawConsent && d.Status == StatusDone {
			note := "Withdrawn on request: " + *d.Outcome
			if err := q.PutConsent(ctx, store.PutConsentParams{
				CandidateID: row.CandidateID, Status: Withdrawn, Method: MethodRequest, Day: toDate(s.today()),
				Note: clip(note, 1000), RecordedBy: p.UserID,
			}); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				Action: "consent.recorded", EntityType: "candidate", EntityID: row.CandidateID,
				Changes: map[string]any{"status": Withdrawn, "method": MethodRequest, "requestId": id},
			}); err != nil {
				return err
			}
		}
		views, err := s.views(ctx, tx, []store.PrivacyRequest{row})
		if err != nil {
			return err
		}
		out = views[0]
		return audit.Record(ctx, tx, audit.Entry{
			Action: "privacy.request_decided", EntityType: "privacy_request", EntityID: id,
			Changes: map[string]any{"status": d.Status, "kind": row.Kind},
		})
	})
	return out, err
}

// Overview is the dashboard: consent counts, open requests (oldest first),
// the latest closed ones, and who withdrew consent.
func (s *Service) Overview(ctx context.Context, p authz.Principal) (Overview, error) {
	var out Overview
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		counts, err := q.ConsentCounts(ctx)
		if err != nil {
			return err
		}
		total, err := s.deps.Candidates.Count(ctx, tx)
		if err != nil {
			return err
		}
		out.Given, out.Withdrawn = int(counts.Given), int(counts.Withdrawn)
		out.NotRecorded = max(total-out.Given-out.Withdrawn, 0)
		open, err := q.ListOpenRequests(ctx)
		if err != nil {
			return err
		}
		if out.Open, err = s.views(ctx, tx, open); err != nil {
			return err
		}
		closed, err := q.ListClosedRequests(ctx)
		if err != nil {
			return err
		}
		if out.Closed, err = s.views(ctx, tx, closed); err != nil {
			return err
		}
		withdrawn, err := q.ListWithdrawn(ctx)
		if err != nil {
			return err
		}
		ids := make([]string, len(withdrawn))
		for i, w := range withdrawn {
			ids[i] = w.CandidateID
		}
		names, err := s.deps.Candidates.Names(ctx, tx, ids)
		if err != nil {
			return err
		}
		for _, w := range withdrawn {
			out.WithdrawnList = append(out.WithdrawnList, WithdrawnCandidate{CandidateID: w.CandidateID, Name: names[w.CandidateID], Day: fromDate(w.Day)})
		}
		return nil
	})
	return out, err
}

// views adds candidate names, due days and who closed each request.
func (s *Service) views(ctx context.Context, tx pgx.Tx, rows []store.PrivacyRequest) ([]RequestView, error) {
	var candidateIDs, userIDs []string
	for _, r := range rows {
		candidateIDs = append(candidateIDs, r.CandidateID)
		if r.ClosedBy != nil {
			userIDs = append(userIDs, *r.ClosedBy)
		}
	}
	names, err := s.deps.Candidates.Names(ctx, tx, candidateIDs)
	if err != nil {
		return nil, err
	}
	users, err := s.deps.Users.UserNames(ctx, tx, userIDs)
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]RequestView, len(rows))
	for i, row := range rows {
		r := requestFromRow(row)
		v := RequestView{Request: r, CandidateName: names[r.CandidateID], DueOn: DueOn(r), Overdue: Overdue(r, today), DaysOpen: DaysOpen(r, today, s.loc)}
		if r.ClosedBy != nil {
			if n, ok := users[*r.ClosedBy]; ok {
				v.ClosedByName = &n
			}
		}
		out[i] = v
	}
	return out, nil
}

// candidateName returns the candidate's name, or NotFound.
func (s *Service) candidateName(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	names, err := s.deps.Candidates.Names(ctx, tx, []string{id})
	if err != nil {
		return "", err
	}
	name, ok := names[id]
	if !ok {
		return "", apperr.NotFound("candidate")
	}
	return name, nil
}

func consentFromRow(r store.CandidateConsent) Consent {
	return Consent{
		CandidateID: r.CandidateID, Version: int(r.Version), Status: r.Status, Method: r.Method, Day: fromDate(r.Day),
		Note: r.Note, RecordedBy: r.RecordedBy, UpdatedAt: r.UpdatedAt,
	}
}

func requestFromRow(r store.PrivacyRequest) Request {
	return Request{
		ID: r.ID, CandidateID: r.CandidateID, Kind: r.Kind, Status: r.Status, Version: int(r.Version), ReceivedOn: fromDate(r.ReceivedOn),
		Details: r.Details, Outcome: r.Outcome, ClosedBy: r.ClosedBy, ClosedAt: r.ClosedAt, CreatedAt: r.CreatedAt,
	}
}

func toDate(t time.Time) pgtype.Date { return pgtype.Date{Time: t, Valid: true} }

func fromDate(d pgtype.Date) time.Time {
	y, m, dd := d.Time.Date()
	return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC)
}

func clip(s string, n int) *string {
	if r := []rune(s); len(r) > n {
		s = string(r[:n])
	}
	return &s
}
