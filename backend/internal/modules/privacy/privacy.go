// Package privacy is data protection (Malaysia's PDPA) for candidates: their
// consent (given on the careers form, or recorded by a recruiter; withdrawn
// means do not contact), the requests they make about their data (a copy,
// a correction, stop using it, remove it) with when each is due, and a copy
// of everything held about a candidate.
//
// Nothing is deleted or anonymised by the app: people decide each request
// and record what they did. Data is kept until they choose otherwise.
//
// Layout (the same in every module):
//
//	privacy.go      the module's public API and what it needs from other modules
//	domain.go       the rules, as pure functions (unit tested)
//	service.go      use cases: one transaction each
//	export.go       a copy of a candidate's data
//	http.go         HTTP handlers: contract types <-> service calls
//	internal/store  generated SQL (db/queries/privacy.sql); private to this module
package privacy

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Deps is what the module needs from other modules, in the caller's
// transaction.
type Deps struct {
	Candidates interface {
		// Names returns the names of the given candidates (missing ones are left out).
		Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
		Count(ctx context.Context, tx pgx.Tx) (int, error)
	}
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	Tenants interface {
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	// Data gathers what the other modules hold about a candidate, as the
	// given user may see it (NotFound for a candidate that does not exist).
	Data DataSource
}

// Module is the privacy module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. Days (consent, requests, due dates) are counted in
// loc, the company's time zone.
func New(pool *pgxpool.Pool, deps Deps, loc *time.Location) *Module {
	if loc == nil {
		loc = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, loc: loc, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// RecordFormConsent records that a candidate consented on the careers form
// today, in the caller's transaction. A consent they withdrew before stays
// withdrawn (the form cannot prove who filled it in): the history notes that
// they applied, for someone in the team to confirm with them.
func (m *Module) RecordFormConsent(ctx context.Context, tx pgx.Tx, candidateID string) error {
	_, err := store.New(tx).GiveConsent(ctx, store.GiveConsentParams{
		CandidateID: candidateID, Method: MethodCareersForm, Day: toDate(m.service.today()),
	})
	if database.IsNoRows(err) {
		// They withdrew consent before. Anyone can type an email into the
		// public form, so the withdrawal stands until a person in the team
		// confirms with them and records it.
		return audit.Record(ctx, tx, audit.Entry{
			Action: "consent.kept_withdrawn", EntityType: "candidate", EntityID: candidateID,
			Changes: map[string]any{"method": MethodCareersForm, "note": "applied through the careers form while consent is withdrawn"},
		})
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "consent.recorded", EntityType: "candidate", EntityID: candidateID,
		Changes: map[string]any{"status": Given, "method": MethodCareersForm},
	})
}

// DoNotContact reports whether the candidate withdrew consent, in the
// caller's transaction (emails to them are refused).
func (m *Module) DoNotContact(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error) {
	c, err := store.New(tx).GetConsent(ctx, candidateID)
	if database.IsNoRows(err) {
		return false, nil
	}
	return c.Status == Withdrawn, err
}

// MoveCandidate moves a merged duplicate's requests and consent to the
// candidate who stays (withdrawn consent wins), in the caller's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if err := q.MoveRequests(ctx, store.MoveRequestsParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	moving, err := q.TakeConsent(ctx, fromID)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var staying *Consent
	if row, err := q.GetConsent(ctx, toID); err == nil {
		c := consentFromRow(row)
		staying = &c
	} else if !database.IsNoRows(err) {
		return err
	}
	out, changed := Merged(consentFromRow(moving), staying)
	if !changed {
		return nil
	}
	return q.PutConsent(ctx, store.PutConsentParams{
		CandidateID: toID, Status: out.Status, Method: out.Method, Day: toDate(out.Day), Note: out.Note, RecordedBy: out.RecordedBy,
	})
}
