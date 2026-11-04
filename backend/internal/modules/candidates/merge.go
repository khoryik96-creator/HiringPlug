package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// RecordMover moves another module's records about a candidate (CVs, jobs,
// notes) when two duplicates are merged, inside the merge's transaction.
type RecordMover interface {
	MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error
}

// MoveRecordsWith sets the modules whose records follow a merge. Called by
// internal/app once every module exists (they depend on this one).
func (m *Module) MoveRecordsWith(movers ...RecordMover) { m.service.movers = movers }

// FillFrom returns keep with every empty field taken from dup, and the names
// of the fields filled.
func FillFrom(keep, dup Profile) (Profile, []string) {
	var filled []string
	fill := func(name string, k **string, d *string) {
		if *k == nil && d != nil {
			*k = d
			filled = append(filled, name)
		}
	}
	fill("email", &keep.Email, dup.Email)
	fill("phone", &keep.Phone, dup.Phone)
	fill("nationality", &keep.Nationality, dup.Nationality)
	fill("industry", &keep.Industry, dup.Industry)
	fill("specialization", &keep.Specialization, dup.Specialization)
	fill("currentTitle", &keep.CurrentTitle, dup.CurrentTitle)
	fill("currentCompany", &keep.CurrentCompany, dup.CurrentCompany)
	fill("location", &keep.Location, dup.Location)
	fill("source", &keep.Source, dup.Source)
	if keep.WorkAuthorization == nil && dup.WorkAuthorization != nil {
		keep.WorkAuthorization, filled = dup.WorkAuthorization, append(filled, "workAuthorization")
	}
	if keep.NoticePeriod == nil && dup.NoticePeriod != nil {
		keep.NoticePeriod, filled = dup.NoticePeriod, append(filled, "noticePeriod")
	}
	return keep, filled
}

// Merge moves duplicate dupID into keepID (its CVs, jobs, notes, salary if
// keepID has none, and its values for keepID's empty fields) and deletes it,
// all in one transaction. version is keepID's version as loaded.
func (s *Service) Merge(ctx context.Context, p authz.Principal, keepID, dupID string, version int) (View, error) {
	if keepID == dupID {
		var errs apperr.FieldErrors
		errs.Add("duplicateId", "is the same candidate")
		return View{}, errs.Err()
	}
	var out View
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		keepRow, err := q.GetCandidate(ctx, keepID)
		if database.IsNoRows(err) {
			return apperr.NotFound("candidate")
		}
		if err != nil {
			return err
		}
		if int(keepRow.Version) != version {
			return apperr.StaleVersion("candidate").WithDetail("currentVersion", keepRow.Version)
		}
		dupRow, err := q.GetCandidate(ctx, dupID)
		if database.IsNoRows(err) {
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "no such candidate")
			return errs.Err()
		}
		if err != nil {
			return err
		}
		for _, m := range s.movers {
			if err := m.MoveCandidate(ctx, tx, dupID, keepID); err != nil {
				return err
			}
		}
		if _, err := q.GetCompensation(ctx, keepID); database.IsNoRows(err) {
			if _, err := q.MoveCompensation(ctx, store.MoveCompensationParams{FromID: dupID, ToID: keepID}); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		// The duplicate's work history and education, if the kept candidate has none.
		if err := q.MoveWorkIfNone(ctx, store.MoveWorkIfNoneParams{FromID: dupID, ToID: keepID}); err != nil {
			return err
		}
		if err := q.MoveEducationIfNone(ctx, store.MoveEducationIfNoneParams{FromID: dupID, ToID: keepID}); err != nil {
			return err
		}
		before := fromRow(keepRow).Profile
		after, filled := FillFrom(before, fromRow(dupRow).Profile)
		// The duplicate goes first: its email may be moving to the kept candidate.
		if _, err := q.DeleteCandidate(ctx, dupID); err != nil {
			return err
		}
		row := keepRow
		if len(filled) > 0 {
			if row, err = q.UpdateCandidate(ctx, updateParams(keepID, version, after)); err != nil {
				return err
			}
		}
		for _, e := range []audit.Entry{
			{Action: "candidate.merged", EntityType: "candidate", EntityID: keepID,
				Changes: map[string]any{"duplicateId": dupID, "duplicateName": FullName(fromRow(dupRow).Profile), "filled": filled}},
			{Action: "candidate.merged_into", EntityType: "candidate", EntityID: dupID, Changes: map[string]any{"keptId": keepID}},
		} {
			if err := audit.Record(ctx, tx, e); err != nil {
				return err
			}
		}
		out, err = loadView(ctx, tx, s.users, p, fromRow(row))
		return err
	})
	return out, err
}

// MergeCandidate handles POST /v1/candidates/{candidateId}/merge.
func (h *HTTP) MergeCandidate(ctx context.Context, req api.MergeCandidateRequestObject) (api.MergeCandidateResponseObject, error) {
	view, err := h.svc.Merge(ctx, authz.MustFromContext(ctx), req.CandidateId, req.Body.DuplicateId, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.MergeCandidate200JSONResponse(candidateToAPI(view)), nil
}
