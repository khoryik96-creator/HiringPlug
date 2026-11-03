package pipeline

// What other modules use: application briefs, and the stage moves that
// follow from their records (an interview scheduled, an offer made, a
// placement recorded).

import (
	"context"
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

// ReachInterview moves an application to the interview stage when it is at an
// earlier one (applied, screening, submitted), in the caller's transaction:
// scheduling an interview means the candidate got that far.
func (m *Module) ReachInterview(ctx context.Context, tx pgx.Tx, id string) error {
	return reach(ctx, tx, id, Interview, "interview scheduled")
}

// ReachOffer moves an application to the offer stage when it is at an earlier
// one, in the caller's transaction: making an offer means the candidate got
// that far.
func (m *Module) ReachOffer(ctx context.Context, tx pgx.Tx, id string) error {
	return reach(ctx, tx, id, Offer, "offer made")
}

// reach moves an application forward to stage if it is at an earlier open stage.
func reach(ctx context.Context, tx pgx.Tx, id, stage, via string) error {
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
	if _, err := q.MoveApplication(ctx, store.MoveApplicationParams{ID: id, ExpectedVersion: row.Version, Stage: stage}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "application.moved", EntityType: "application", EntityID: id,
		Changes: map[string]any{"stage": map[string]string{"from": row.Stage, "to": stage}, "via": via},
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
	if _, err := q.MoveApplication(ctx, store.MoveApplicationParams{ID: id, ExpectedVersion: row.Version, Stage: Placed}); err != nil {
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
}

// Facts returns the applications created in [from, to), in the caller's
// transaction.
func (m *Module) Facts(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Fact, error) {
	rows, err := store.New(tx).ApplicationFacts(ctx, store.ApplicationFactsParams{FromTime: from, ToTime: to})
	out := make([]Fact, len(rows))
	for i, r := range rows {
		out[i] = Fact{ID: r.ID, JobID: r.JobOrderID, CandidateID: r.CandidateID, Channel: r.Channel, CreatedAt: r.CreatedAt, Stages: r.Stages}
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
