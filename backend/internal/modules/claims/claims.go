// Package claims is expense claims: a person uploads receipts (photos or
// PDFs), the Python AI worker reads each one (OCR) and fills in its date,
// total, currency, merchant and type, the person completes the purpose,
// claim type and the client or candidate it was for, and submits; approvers
// approve or reject and mark claims paid.
//
// Layout (the same in every module):
//
//	claims.go     the module's public API: what internal/app and other modules may use
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	service.go    use cases: one transaction each, rules + storage + audit
//	review.go     submitting, withdrawing and deciding claims
//	view.go       claims as the API shows them (names of people and records)
//	jobs.go       what was read from a receipt (the AI worker's answer)
//	http.go       HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/claims.sql); private to this module
package claims

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/claims/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// The Settings dropdowns claims use.
const (
	ReceiptTypeList = "claim_receipt_type"
	ClaimTypeList   = "claim_type"
)

// Option is one choice of a dropdown.
type Option struct{ Key, Label string }

// Record is a client (company or contact) or candidate a person may see.
type Record struct {
	Name   string
	Detail *string // a contact's company, a candidate's current title
}

// Deps are what the module needs from others, passed in by internal/app.
type Deps struct {
	// Users names people (claimants and reviewers), in the caller's transaction.
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// Options are the company's dropdowns (the settings module).
	Options interface {
		HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error)
		// Choices are a list's options offered for new choices, in order.
		Choices(ctx context.Context, tx pgx.Tx, list string) ([]Option, error)
	}
	// Records names the companies, contacts or candidates (kind) among ids
	// that the transaction's person may see; the others are left out.
	Records interface {
		Records(ctx context.Context, tx pgx.Tx, kind string, ids []string) (map[string]Record, error)
	}
}

// Module is the claims module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module: receipts are kept in files, at most maxUploadBytes
// each; zone is where the company works (what "today" is).
func New(pool *pgxpool.Pool, files *blob.Store, deps Deps, maxUploadBytes int64, zone *time.Location) *Module {
	if zone == nil {
		zone = time.UTC
	}
	svc := &Service{pool: pool, files: files, deps: deps, zone: zone, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}

// JobHandlers are the module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindClaimReceiptRead: m.service.saveReceiptRead}
}

// MoveCandidate moves the claims tagged to a merged duplicate to the
// candidate kept, in the merge's transaction: everyone's claims, whoever
// merges (the claims of others are not theirs to see).
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return asSystem(ctx, tx, func(q *store.Queries) error {
		return q.MoveCandidateClaims(ctx, store.MoveCandidateClaimsParams{FromID: &fromID, ToID: &toID})
	})
}

// asSystem runs fn over everyone's claims in the caller's transaction.
func asSystem(ctx context.Context, tx pgx.Tx, fn func(q *store.Queries) error) error {
	q := store.New(tx)
	tenant, err := q.CurrentClaimTenant(ctx)
	if err != nil {
		return err
	}
	return database.AsSystem(ctx, tx, tenant, func() error { return fn(q) })
}

// OptionUses counts the claims holding a Settings option (receipt or claim
// type), everyone's, so an option in use is hidden rather than removed.
func (m *Module) OptionUses(ctx context.Context, tx pgx.Tx, list, key string) (int, error) {
	var n int32
	err := asSystem(ctx, tx, func(q *store.Queries) error {
		var err error
		switch list {
		case ReceiptTypeList:
			n, err = q.CountClaimsWithReceiptType(ctx, &key)
		case ClaimTypeList:
			n, err = q.CountClaimsWithClaimType(ctx, &key)
		}
		return err
	})
	return int(n), err
}
