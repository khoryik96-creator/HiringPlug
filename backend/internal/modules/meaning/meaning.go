// Package meaning is search by meaning: candidates found by what they do,
// not only by the words their CVs use ("talent acquisition" finds a
// "Recruitment Consultant", "someone who hired engineers" finds a CV that
// says "hired 120 engineers a year" without the word recruiter).
//
// A small meaning model in the Python AI worker (on this server: no AI
// provider, no allowance, nothing sent away) reads each candidate's titles,
// work and newest CV, and each search, as a recruiter reads them, into
// vectors (contracts/modules/meaning.yaml says how). The database keeps and
// compares the vectors (pgvector); this module ranks with them (domain.go)
// and says why each candidate was found.
//
// Each company's candidates are read in the background (sweep.go): every
// candidate once, then again when their titles, work or newest CV change.
//
// Layout (the same in every module):
//
//	meaning.go      the module's public API and what it needs from other modules
//	domain.go       the ranking and the fingerprints (unit tested)
//	sweep.go        reading the company's candidates in the background
//	rank.go         reading a search or a job, and ranking candidates for it
//	gather.go       who a search compares: everyone when few enough, else the nearest by the indexes
//	jobs.go         handlers for the module's jobs on the core queue
//	http.go         HTTP handlers: contract types <-> service calls
//	internal/store  generated SQL (db/queries/meaning.sql); private to this module
package meaning

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Work is one job a candidate held, as their profile has it.
type Work struct {
	Position           int // 0 is the newest
	Title, Description string
	StartDate, EndDate string // "2019" or "2019-04"; "" unknown
	Current            bool
}

// CV is a candidate's newest CV they sent, with its text.
type CV struct {
	DocumentID, Text string
}

// CVKey is which CV is a candidate's newest, and which reading of it (a CV
// read again is another version), without its text.
type CVKey struct {
	DocumentID, Version string
}

// Source is what is read of a candidate.
type Source struct {
	CandidateID  string
	CurrentTitle string
	Work         []Work
	CV           *CVKey
}

// Term is a word or phrase to look for in CVs, with how much it counts.
type Term struct {
	Text   string
	Weight float32
}

// CVHit is a candidate whose CV has a search's words.
type CVHit struct {
	CandidateID string
	Rank        float64 // for ordering
	Snippet     string  // where, the words marked [[like this]]
}

// Deps is what the module needs from other modules, in the caller's transaction.
type Deps struct {
	Candidates interface {
		// SourcesAfter returns the candidates after cursorID, in id order, with their titles and work.
		SourcesAfter(ctx context.Context, tx pgx.Tx, cursorID string, limit int) ([]Source, error)
		// Sources returns these candidates' titles and work (those that exist).
		Sources(ctx context.Context, tx pgx.Tx, ids []string) ([]Source, error)
		Count(ctx context.Context, tx pgx.Tx) (int, error)
		// Visible returns the ids of at most limit candidates the user may see.
		Visible(ctx context.Context, tx pgx.Tx, limit int) ([]string, error)
	}
	CVs interface {
		// NewestCVs returns each candidate's newest CV they sent that has text.
		NewestCVs(ctx context.Context, tx pgx.Tx, candidateIDs []string) (map[string]CV, error)
		// NewestCVKeys returns which CV NewestCVs would return for each, without its text.
		NewestCVKeys(ctx context.Context, tx pgx.Tx, candidateIDs []string) (map[string]CVKey, error)
		// CandidateOf returns the candidate a document belongs to ("" for none or a formatted CV).
		CandidateOf(ctx context.Context, tx pgx.Tx, documentID string) (string, error)
		// SearchCVs returns the candidates whose CVs best match the terms, best first.
		SearchCVs(ctx context.Context, tx pgx.Tx, terms []Term, exclude []string, limit int) ([]CVHit, error)
		// DocumentExists says whether a document is still there.
		DocumentExists(ctx context.Context, tx pgx.Tx, documentID string) (bool, error)
	}
}

// Module is the meaning module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps) *Module {
	svc := &Service{pool: pool, deps: deps, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{
		api.JobKindMeaningSweep:          m.service.sweep,
		api.JobKindMeaningCandidatesRead: m.service.candidatesRead,
		api.JobKindMeaningQueryRead:      m.service.queryRead,
	}
}

// ReadDone is the documents module's hook for a CV whose reading has ended:
// its candidate is read again by meaning (now, not at the next sweep).
func (m *Module) ReadDone(ctx context.Context, tx pgx.Tx, tenantID, documentID string, _ *api.ParsedProfile) error {
	return m.service.cvRead(ctx, tx, tenantID, documentID)
}

// CVBinned is the documents module's hook for a CV a candidate sent, moved
// to the Bin (binned) or back from it: what was read of a CV in the Bin goes
// at once, and the candidate is read again with their newest CV now.
func (m *Module) CVBinned(ctx context.Context, tx pgx.Tx, tenantID, candidateID, documentID string, binned bool) error {
	return m.service.cvChanged(ctx, tx, tenantID, candidateID, documentID, binned)
}
