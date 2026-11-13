package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Attached reports which of some applications carry records of another
// module (interviews, offers, a placement), so merging candidates never
// deletes them.
type Attached func(ctx context.Context, tx pgx.Tx, applicationIDs []string) (map[string]bool, error)

// KeepRecordsWith registers what tells whether an application carries
// records (the interviews, offers and placements modules).
func (m *Module) KeepRecordsWith(checks ...Attached) { m.service.attached = checks }

// Overlap is a job both merged candidates are on: the duplicate's
// application there and the kept candidate's, whether each carries records,
// and whether the duplicate's stage changed last.
type Overlap struct {
	FromID, ToID      string
	FromHas, ToHas    bool
	FromChangedLatest bool
}

// Drop is which of the two applications goes when candidates are merged (a
// candidate is on a job once), or ok false when neither may: both carry
// records. An application with interviews, offers or a placement is never
// the one dropped for a bare one; between two bare ones the one whose stage
// changed last is the truth and stays.
func (o Overlap) Drop() (id string, ok bool) {
	switch {
	case o.FromHas && o.ToHas:
		return "", false
	case o.FromHas:
		return o.ToID, true
	case o.ToHas:
		return o.FromID, true
	case o.FromChangedLatest:
		return o.ToID, true
	}
	return o.FromID, true
}

// MoveCandidate moves this module's records about candidate fromID to toID,
// inside the transaction merging two duplicate candidates. On a job both are
// on, one application stays (see Overlap.Drop); when both carry records the
// merge is refused, so nothing is lost.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	rows, err := q.OverlappingApplications(ctx, store.OverlappingApplicationsParams{FromID: fromID, ToID: toID})
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		ids := make([]string, 0, 2*len(rows))
		for _, r := range rows {
			ids = append(ids, r.FromID, r.ToID)
		}
		has := map[string]bool{}
		for _, check := range m.service.attached {
			found, err := check(ctx, tx, ids)
			if err != nil {
				return err
			}
			for id, yes := range found {
				has[id] = has[id] || yes
			}
		}
		drop := make([]string, 0, len(rows))
		for _, r := range rows {
			id, ok := Overlap{FromID: r.FromID, ToID: r.ToID, FromHas: has[r.FromID], ToHas: has[r.ToID],
				FromChangedLatest: r.FromChangedAt.After(r.ToChangedAt)}.Drop()
			if !ok {
				title := "a job"
				if jobs, err := m.service.jobs.Info(ctx, tx, []string{r.JobOrderID}); err != nil {
					return err
				} else if j, found := jobs[r.JobOrderID]; found {
					title = j.Title
				}
				return apperr.New(api.ErrorCodeAPPLICATIONSOVERLAP,
					"Both are on "+title+", each with interviews, offers or a placement. Remove one of the two from that job first, then merge: nothing is deleted for you.").
					WithDetail("jobId", r.JobOrderID)
			}
			drop = append(drop, id)
		}
		if _, err := q.DeleteApplications(ctx, drop); err != nil {
			return err
		}
	}
	_, err = q.MoveApplicationsToCandidate(ctx, store.MoveApplicationsToCandidateParams{FromID: fromID, ToID: toID})
	return err
}
