// Package careers is the public careers page: a company's published job ads,
// and applying to one with a CV. Visitors are signed out; the page address
// picks the company, and everything after runs in that company's tenant scope
// (row-level security applies as for signed-in users).
//
// The module has no tables of its own. Applying uses the candidates,
// documents, pipeline and jobs modules through the small interfaces below,
// in one transaction: a candidate, their CV and their application are added
// together or not at all.
//
// Layout (the same in every module):
//
//	careers.go   the module's public API and what it needs from other modules
//	domain.go    the rules, as pure functions on plain values (unit tested)
//	limiter.go   how many applications one address may send
//	feed.go      the open ads as an XML job feed
//	service.go   use cases: one transaction each
//	http.go      HTTP handlers: contract types <-> service calls
package careers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ad is a published job ad as the careers page shows it.
type Ad struct {
	ID, JobID, Title string
	Location         *string
	WorkType         *string
	WorkArrangement  *string
	SalaryText       *string
	Description      string
	ClosesOn         *time.Time
	PublishedAt      time.Time
}

// Applicant is the person applying.
type Applicant struct {
	FirstName, LastName, Email string
	Phone                      *string
	Channel                    string // the link they came from: careers_page, jobstreet, linkedin, ...
}

// Tenants finds the company behind a careers page address (identity module).
type Tenants interface {
	CareersTenant(ctx context.Context, slug string) (tenantID, name string, ok bool, err error)
}

// Ads reads published ads (jobs module).
type Ads interface {
	PublishedAds(ctx context.Context, tx pgx.Tx) ([]Ad, error)
	PublishedAd(ctx context.Context, tx pgx.Tx, id string) (Ad, error)
}

// Candidates finds or adds the applicant (candidates module).
type Candidates interface {
	FindOrCreateApplicant(ctx context.Context, tx pgx.Tx, a Applicant) (candidateID string, created bool, err error)
}

// CVs stores the applicant's CV and queues reading it (documents module).
type CVs interface {
	AddFile(ctx context.Context, tx pgx.Tx, tenantID, candidateID, fileName string, data []byte) (documentID string, undo func(), err error)
}

// Pipeline puts the applicant on the job (pipeline module).
type Pipeline interface {
	AddApplied(ctx context.Context, tx pgx.Tx, tenantID, jobID, candidateID, channel string) (added bool, err error)
}

// Deps are what the module uses from the others.
type Deps struct {
	Tenants    Tenants
	Ads        Ads
	Candidates Candidates
	CVs        CVs
	Pipeline   Pipeline
	Consent    Consent
}

// Consent records that an applicant consented on the form (privacy module).
type Consent interface {
	RecordFormConsent(ctx context.Context, tx pgx.Tx, candidateID string) error
}

// Module is the careers module.
type Module struct {
	HTTP *HTTP
}

// New wires the module.
// publicURL is where people reach the web app (links in the job feed).
func New(pool *pgxpool.Pool, deps Deps, maxUploadBytes int64, publicURL string) *Module {
	svc := &Service{pool: pool, deps: deps, limiter: newLimiter(time.Now), publicURL: publicURL}
	return &Module{HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}
