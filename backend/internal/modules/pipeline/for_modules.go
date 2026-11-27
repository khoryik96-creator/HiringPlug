package pipeline

// What other modules use: application briefs, and the stage moves that
// follow from their records (an interview scheduled, an offer made, a
// placement recorded).

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Brief is an application as other modules show it (interviews, placements).
type Brief struct {
	ID, Stage                  string
	JobID, JobTitle            string
	CompanyName                string
	CandidateID, CandidateName string
}

// Briefs returns the given applications (missing ones are left out), in the
// caller's transaction.
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	out := map[string]Brief{}
	q := store.New(tx)
	var rows []store.Application
	for _, id := range ids {
		if _, seen := out[id]; seen {
			continue
		}
		row, err := q.GetApplication(ctx, id)
		if database.IsNoRows(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = Brief{}
		rows = append(rows, row)
	}
	views, err := m.service.views(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	for _, v := range views {
		out[v.ID] = Brief{ID: v.ID, Stage: v.Stage, JobID: v.JobID, JobTitle: v.JobTitle, CompanyName: v.CompanyName, CandidateID: v.CandidateID, CandidateName: v.CandidateName}
	}
	return out, nil
}

// OnJob is an application as seen from its job (e.g. for CV vs job matching).
type OnJob struct {
	ID, CandidateID, Stage string
}

// OpenOnJob returns the job's applications that are not rejected or
// withdrawn, oldest first, in the caller's transaction.
func (m *Module) OpenOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]OnJob, error) {
	rows, err := store.New(tx).ListJobApplications(ctx, jobID)
	if err != nil {
		return nil, err
	}
	var out []OnJob
	for _, r := range rows {
		if !IsClosed(r.Stage) {
			out = append(out, OnJob{ID: r.ID, CandidateID: r.CandidateID, Stage: r.Stage})
		}
	}
	return out, nil
}

// WhenAdded registers what runs when a candidate is put on a job (called by
// internal/app once every module exists).
func (m *Module) WhenAdded(hooks ...AddedHook) { m.service.whenAdded = hooks }

// UnplacedHook runs, in the move's transaction, when an application is moved
// out of placed (back to offer, or withdrawn): the placements module backs
// its placement out.
type UnplacedHook func(ctx context.Context, tx pgx.Tx, applicationID string) error

// WhenUnplaced registers what runs when an application leaves placed.
func (m *Module) WhenUnplaced(hooks ...UnplacedHook) { m.service.whenUnplaced = hooks }

// CandidatesOnJob returns the candidates on a job, at any stage, in the
// caller's transaction.
func (m *Module) CandidatesOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]string, error) {
	rows, err := store.New(tx).ListJobApplications(ctx, jobID)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.CandidateID
	}
	return out, err
}

// PlacedOnJobs returns the applications on some jobs that are or were ever
// placed, in the caller's transaction (e.g. to add up what a company was billed).
func (m *Module) PlacedOnJobs(ctx context.Context, tx pgx.Tx, jobIDs []string) ([]string, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}
	return store.New(tx).PlacedApplicationIDsOnJobs(ctx, jobIDs)
}

// OnJobOf is an application of a candidate, with its job.
type OnJobOf struct {
	OnJob
	JobID string
}

// OpenOfCandidate returns a candidate's applications that are not rejected
// or withdrawn, in the caller's transaction.
func (m *Module) OpenOfCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]OnJobOf, error) {
	rows, err := store.New(tx).ListCandidateApplications(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	var out []OnJobOf
	for _, r := range rows {
		if !IsClosed(r.Stage) {
			out = append(out, OnJobOf{OnJob: OnJob{ID: r.ID, CandidateID: r.CandidateID, Stage: r.Stage}, JobID: r.JobOrderID})
		}
	}
	return out, nil
}

// ReachInterview moves an application to the interview stage when it is at an
// earlier one (applied, screening, submitted), in the caller's transaction:
// scheduling an interview means the candidate got that far.
func (m *Module) ReachInterview(ctx context.Context, tx pgx.Tx, id string) error {
	return m.reach(ctx, tx, id, Interview, "interview scheduled")
}

// ReachOffer moves an application to the offer stage when it is at an earlier
// one, in the caller's transaction: making an offer means the candidate got
// that far.
func (m *Module) ReachOffer(ctx context.Context, tx pgx.Tx, id string) error {
	return m.reach(ctx, tx, id, Offer, "offer made")
}

// reach moves an application forward to stage if it is at an earlier open stage.
func (m *Module) reach(ctx context.Context, tx pgx.Tx, id, stage, via string) error {
	q := store.New(tx)
	row, err := q.GetApplication(ctx, id)
	if database.IsNoRows(err) {
		return apperr.NotFound("application")
	}
	if err != nil {
		return err
	}
	if !slices.Contains(OpenStages[:slices.Index(OpenStages, stage)], row.Stage) {
		return nil
	}
	if err := m.moveTo(ctx, tx, row, stage); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "application.moved", EntityType: "application", EntityID: id,
		Changes: map[string]any{"stage": map[string]string{"from": row.Stage, "to": stage}, "via": via},
	})
}

// Unplace moves a placed application back to offer, in the caller's
// transaction: its placement was marked backed out on the placement sheet.
// The move hooks do not run (the placement is already backed out). Not
// placed: nothing changes.
func (m *Module) Unplace(ctx context.Context, tx pgx.Tx, id string) error {
	q := store.New(tx)
	row, err := q.GetApplication(ctx, id)
	if database.IsNoRows(err) {
		return apperr.NotFound("application")
	}
	if err != nil || row.Stage != Placed {
		return err
	}
	if err := m.moveTo(ctx, tx, row, Offer); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "application.moved", EntityType: "application", EntityID: id,
		Changes: map[string]any{"stage": map[string]string{"from": Placed, "to": Offer}, "via": "placement backed out"},
	})
}

// Place moves an application to placed (recording a placement), in the
// caller's transaction, by the pipeline rules: a closed application is
// reopened first. Already placed: nothing changes.
func (m *Module) Place(ctx context.Context, tx pgx.Tx, id string) error {
	q := store.New(tx)
	row, err := q.GetApplication(ctx, id)
	if database.IsNoRows(err) {
		return apperr.NotFound("application")
	}
	if err != nil {
		return err
	}
	if row.Stage == Placed {
		return nil
	}
	if IsClosed(row.Stage) {
		return MoveNotAllowedError{Message: "Reopen the application (move it to an open stage) before placing the candidate."}
	}
	if err := m.moveTo(ctx, tx, row, Placed); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "application.moved", EntityType: "application", EntityID: id,
		Changes: map[string]any{"stage": map[string]string{"from": row.Stage, "to": Placed}, "via": "placement recorded"},
	})
}

// Fact is an application as reports count it: every stage it has been at, in
// order, and where it came from.
type Fact struct {
	ID, JobID, CandidateID string
	Channel                *string
	CreatedAt              time.Time
	Stages                 []string
	Categories             []string // the category of each of Stages ("": not recorded)
}

// Facts returns the applications created in [from, to), in the caller's
// transaction.
func (m *Module) Facts(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Fact, error) {
	rows, err := store.New(tx).ApplicationFacts(ctx, store.ApplicationFactsParams{FromTime: from, ToTime: to})
	out := make([]Fact, len(rows))
	for i, r := range rows {
		out[i] = Fact{ID: r.ID, JobID: r.JobOrderID, CandidateID: r.CandidateID, Channel: r.Channel, CreatedAt: r.CreatedAt, Stages: r.Stages, Categories: r.Categories}
	}
	return out, err
}

// PlacedFact is an application's first move to placed.
type PlacedFact struct {
	ID, JobID           string
	CreatedAt, PlacedAt time.Time
}

// FirstPlaced returns the applications first placed in [from, to), in the
// caller's transaction.
func (m *Module) FirstPlaced(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]PlacedFact, error) {
	rows, err := store.New(tx).FirstPlacedBetween(ctx, store.FirstPlacedBetweenParams{FromTime: from, ToTime: to})
	out := make([]PlacedFact, len(rows))
	for i, r := range rows {
		out[i] = PlacedFact{ID: r.ID, JobID: r.JobOrderID, CreatedAt: r.CreatedAt, PlacedAt: r.PlacedAt}
	}
	return out, err
}

// JobFirst is when something first happened on a job.
type JobFirst struct {
	JobID string
	At    time.Time
}

// FirstSubmitted returns the jobs whose first CV was sent (their first move
// to submitted) between two instants, in the caller's transaction.
func (m *Module) FirstSubmitted(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]JobFirst, error) {
	rows, err := store.New(tx).FirstSubmittedBetween(ctx, store.FirstSubmittedBetweenParams{FromTime: from, ToTime: to})
	if err == nil && len(rows) >= MaxReportJobs {
		return nil, ErrTooManyMoves
	}
	out := make([]JobFirst, len(rows))
	for i, r := range rows {
		out[i] = JobFirst{JobID: r.JobOrderID, At: r.FirstAt}
	}
	return out, err
}

// Reached is an application getting to a category at a time.
type Reached struct {
	Category string
	At       time.Time
}

// CategoriesReached returns, for each of some applications, every category it
// got to and when, oldest first, in the caller's transaction. A move recorded
// without a category (before categories) is in its stage's first.
func (m *Module) CategoriesReached(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]Reached, error) {
	rows, err := store.New(tx).CategoriesReached(ctx, ids)
	if err != nil {
		return nil, err
	}
	cats, _, err := categories(ctx, tx)
	if err != nil {
		return nil, err
	}
	history := historyOf(cats)
	out := map[string][]Reached{}
	for _, r := range rows {
		if c := history.categoryOf(r.ToStage, r.ToCategory); c != "" {
			out[r.ApplicationID] = append(out[r.ApplicationID], Reached{Category: c, At: r.LastAt})
		}
	}
	return out, nil
}

// FirstPlacedJobs returns the jobs whose first placement was between two
// instants, in the caller's transaction.
func (m *Module) FirstPlacedJobs(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]JobFirst, error) {
	rows, err := store.New(tx).FirstPlacedJobsBetween(ctx, store.FirstPlacedJobsBetweenParams{FromTime: from, ToTime: to})
	if err == nil && len(rows) >= MaxReportJobs {
		return nil, ErrTooManyMoves
	}
	out := make([]JobFirst, len(rows))
	for i, r := range rows {
		out[i] = JobFirst{JobID: r.JobOrderID, At: r.FirstAt}
	}
	return out, err
}

// ErrTooManyMoves is returned when a period holds more moves than one read
// takes (MaxReportMoves): the caller asks for a shorter period instead of
// counting a part of them.
var ErrTooManyMoves = errors.New("pipeline: too many moves in the period")

// MaxReportMoves is the most stage moves one report read takes, and
// MaxReportJobs the most jobs (the queries' LIMIT).
const (
	MaxReportMoves = 1000000
	MaxReportJobs  = 200000
)

// history reads the categories of recorded moves against today's categories.
type history struct {
	known map[string]bool
	first map[string]string // each kind's first category, by position
}

func historyOf(cats []Category) history {
	h := history{known: map[string]bool{}, first: map[string]string{}}
	for _, c := range cats {
		h.known[c.Key] = true
		if _, ok := h.first[c.Kind]; !ok {
			h.first[c.Kind] = c.Key
		}
	}
	return h
}

// categoryOf is the category a recorded move is in: the one recorded while
// the company still has it; else (a move from before categories, or one
// into a category removed since) its stage's standard category while the
// company keeps it, else its stage's first. Reordering categories never moves
// past numbers. "" for a closed stage.
func (h history) categoryOf(stage string, category *string) string {
	if category != nil && h.known[*category] {
		return *category
	}
	if h.known[stage] { // the standard key of each kind is the stage's own name
		return stage
	}
	return h.first[stage]
}

// moveTo moves an application to stage (its kind's first step), with the
// category it then falls under.
func (m *Module) moveTo(ctx context.Context, tx pgx.Tx, row store.Application, stage string) error {
	category, err := m.service.appCategory(ctx, tx, row.JobOrderID, stage, nil)
	if err != nil {
		return err
	}
	_, err = store.New(tx).MoveApplication(ctx, store.MoveApplicationParams{ID: row.ID, ExpectedVersion: row.Version, Stage: stage, Category: category})
	return err
}
