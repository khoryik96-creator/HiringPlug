// Package candidates manages candidate profiles and their salary details.
//
// Layout (the same in every module):
//
//	candidates.go     the module's public API: what internal/app and other modules may use
//	domain.go         the rules, as pure functions on plain values (unit tested)
//	service.go        use cases: one transaction each, rules + storage + audit + visibility
//	http.go           HTTP handlers: contract types <-> service calls; no business rules
//	internal/store    generated SQL (db/queries/candidates.sql); private to this module
//
// Salary details live in their own table. They are only loaded for users with
// candidates.compensation:read, and the database refuses them to anyone else.
package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/salary"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Module is the candidates module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. phoneRegion is the country assumed for local phone
// numbers, e.g. "MY".
func New(pool *pgxpool.Pool, phoneRegion string, users Users, settings Settings) *Module {
	svc := &Service{pool: pool, phoneRegion: phoneRegion, users: users, rules: settings, salaryRules: settings, labels: settings}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Settings is what this module needs from the settings module: the
// company's mandatory fields and lists, and its salary calculation rules.
type Settings interface {
	FieldRules
	SalaryRules
	Labeler
}

// Labeler gives the words for the company's lists (list -> key -> label),
// in the caller's transaction: exports show a source's words, not its key.
type Labeler interface {
	Labels(ctx context.Context, tx pgx.Tx) (map[string]map[string]string, error)
}

// FormatSalaryBreakdown writes a stored salary breakdown for an email
// ("12k + AWS" -> "RM 12,000 + 13th month fixed/contractual bonus per month").
// The stored wording is not changed.
func FormatSalaryBreakdown(raw, currency, period string) string {
	return salary.FormatBreakdown(raw, currency, period)
}

// CleanSalaryWording tidies salary words for an email ("perf bonus" ->
// "performance bonus") without touching any amount.
func CleanSalaryWording(text string) string { return salary.CleanWording(text) }

// Users is what this module needs from the identity module: owners' names,
// and whether someone is an active user (to own candidates).
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
}

// Exists reports whether a candidate exists in tx's tenant. Other modules use
// it to check references (e.g. a document's candidate) inside their own
// transaction.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error) {
	return m.service.Exists(ctx, tx, candidateID)
}

// Count returns how many candidates the company has, in the caller's
// transaction (the data protection dashboard).
func (m *Module) Count(ctx context.Context, tx pgx.Tx) (int, error) {
	n, err := store.New(tx).CountCandidates(ctx)
	return int(n), err
}

// VisibleIDs returns the ids of at most limit candidates the user may see,
// in the caller's transaction.
func (m *Module) VisibleIDs(ctx context.Context, tx pgx.Tx, limit int) ([]string, error) {
	return store.New(tx).VisibleCandidateIDs(ctx, int32(limit)) //nolint:gosec // a small limit
}

// Brief is what other modules need to show a candidate.
type Brief struct {
	Name         string
	FirstName    string
	Email        *string
	CurrentTitle *string
	Source       *string // where the candidate came from (jobstreet, linkedin, ...)
	Availability *string // looking, passive, not_looking or placed; nil when not set
}

// Briefs returns name and current title of the given candidates (missing ones
// are left out), for other modules (e.g. a job's pipeline).
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	return m.service.Briefs(ctx, tx, ids)
}

// IDsByEmails resolves normalized addresses within the caller's scope.
func (m *Module) IDsByEmails(ctx context.Context, tx pgx.Tx, addresses []string) ([]string, error) {
	return store.New(tx).CandidateIDsByEmails(ctx, addresses)
}

// ViewIn returns a candidate as the user may see them (salary only with
// permission), in the caller's transaction: for formatted CVs.
func (m *Module) ViewIn(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (View, error) {
	q := store.New(tx)
	row, err := q.GetCandidate(ctx, id)
	if database.IsNoRows(err) {
		return View{}, apperr.NotFound("candidate")
	}
	if err != nil {
		return View{}, err
	}
	return m.service.loadView(ctx, tx, m.service.users, p, fromRow(row))
}
