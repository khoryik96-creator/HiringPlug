package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ReplaceHistory replaces a candidate's work history and education. The
// candidate's version moves on, so a form opened before this is stale.
func (s *Service) ReplaceHistory(ctx context.Context, p authz.Principal, id string, expectedVersion int, h History) (View, error) {
	h, errs := NormalizeHistory(h)
	if err := errs.Err(); err != nil {
		return View{}, err
	}
	var view View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.TouchCandidate(ctx, store.TouchCandidateParams{ID: id, ExpectedVersion: int32(expectedVersion)}) //nolint:gosec // versions are small
		if database.IsNoRows(err) {
			if exists, err := q.CandidateExists(ctx, id); err != nil || !exists {
				return firstErr(err, apperr.NotFound("candidate"))
			}
			return apperr.StaleVersion("candidate")
		}
		if err != nil {
			return err
		}
		before, err := loadHistory(ctx, q, id)
		if err != nil {
			return err
		}
		if err := saveHistory(ctx, q, id, h); err != nil {
			return err
		}
		// Counts only: the entries are on the candidate's page.
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "candidate.history_replaced", EntityType: "candidate", EntityID: id,
			Changes: map[string]any{
				"experience": map[string]int{"from": len(before.Work), "to": len(h.Work)},
				"education":  map[string]int{"from": len(before.Education), "to": len(h.Education)},
			},
		}); err != nil {
			return err
		}
		view, err = loadView(ctx, tx, s.users, p, fromRow(row))
		return err
	})
	return view, err
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func saveHistory(ctx context.Context, q *store.Queries, id string, h History) error {
	if err := q.DeleteWork(ctx, id); err != nil {
		return err
	}
	if err := q.DeleteEducation(ctx, id); err != nil {
		return err
	}
	for i, j := range h.Work {
		if err := q.InsertWork(ctx, store.InsertWorkParams{
			CandidateID: id, Position: int32(i), Title: j.Title, Company: j.Company, //nolint:gosec // at most 50
			StartDate: j.StartDate, EndDate: j.EndDate, IsCurrent: j.Current, Description: j.Description,
		}); err != nil {
			return err
		}
	}
	for i, e := range h.Education {
		var year *int32
		if e.EndYear != nil {
			y := int32(*e.EndYear) //nolint:gosec // 1950-2100
			year = &y
		}
		if err := q.InsertEducation(ctx, store.InsertEducationParams{
			CandidateID: id, Position: int32(i), Qualification: e.Qualification, Institution: e.Institution, EndYear: year, //nolint:gosec // at most 20
		}); err != nil {
			return err
		}
	}
	return nil
}

func loadHistory(ctx context.Context, q *store.Queries, id string) (History, error) {
	work, err := q.ListWork(ctx, id)
	if err != nil {
		return History{}, err
	}
	edu, err := q.ListEducation(ctx, id)
	if err != nil {
		return History{}, err
	}
	h := History{Work: make([]Job, len(work)), Education: make([]Qualification, len(edu))}
	for i, w := range work {
		h.Work[i] = Job{Title: w.Title, Company: w.Company, StartDate: w.StartDate, EndDate: w.EndDate, Current: w.IsCurrent, Description: w.Description}
	}
	for i, e := range edu {
		var year *int
		if e.EndYear != nil {
			y := int(*e.EndYear)
			year = &y
		}
		h.Education[i] = Qualification{Qualification: e.Qualification, Institution: e.Institution, EndYear: year}
	}
	return h, nil
}

// ReplaceCandidateHistory handles PUT /v1/candidates/{candidateId}/history.
func (h *HTTP) ReplaceCandidateHistory(ctx context.Context, req api.ReplaceCandidateHistoryRequestObject) (api.ReplaceCandidateHistoryResponseObject, error) {
	body := req.Body
	hist := History{Work: make([]Job, len(body.Experience)), Education: make([]Qualification, len(body.Education))}
	for i, w := range body.Experience {
		hist.Work[i] = Job{
			Title: apiconv.ValueOrNil(w.Title), Company: apiconv.ValueOrNil(w.Company), StartDate: apiconv.ValueOrNil(w.StartDate),
			EndDate: apiconv.ValueOrNil(w.EndDate), Current: w.Current, Description: apiconv.ValueOrNil(w.Description),
		}
	}
	for i, e := range body.Education {
		hist.Education[i] = Qualification{
			Qualification: apiconv.ValueOrNil(e.Qualification), Institution: apiconv.ValueOrNil(e.Institution), EndYear: apiconv.ValueOrNil(e.EndYear),
		}
	}
	view, err := h.svc.ReplaceHistory(ctx, authz.MustFromContext(ctx), req.CandidateId, body.Version, hist)
	if err != nil {
		return nil, err
	}
	return api.ReplaceCandidateHistory200JSONResponse(candidateToAPI(view)), nil
}

func historyToAPI(h History) ([]api.WorkEntry, []api.EducationEntry) {
	work := make([]api.WorkEntry, len(h.Work))
	for i, j := range h.Work {
		work[i] = api.WorkEntry{
			Title: apiconv.OrNull(j.Title), Company: apiconv.OrNull(j.Company), StartDate: apiconv.OrNull(j.StartDate),
			EndDate: apiconv.OrNull(j.EndDate), Current: j.Current, Description: apiconv.OrNull(j.Description),
		}
	}
	edu := make([]api.EducationEntry, len(h.Education))
	for i, e := range h.Education {
		edu[i] = api.EducationEntry{Qualification: apiconv.OrNull(e.Qualification), Institution: apiconv.OrNull(e.Institution), EndYear: apiconv.OrNull(e.EndYear)}
	}
	return work, edu
}
