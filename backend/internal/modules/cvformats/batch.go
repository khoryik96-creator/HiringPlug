package cvformats

// Formatting several candidates' CVs at once, CV Studio's way, with the same
// options: each candidate's newest CV they sent (PDF or Word) goes through
// StartStudio as if started on its own, so every rule, check and audit entry
// is the same. A candidate who cannot be formatted now is skipped with the
// reason; the others go ahead.

import (
	"context"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// MaxBatch is how many candidates one batch formats.
const MaxBatch = 20

// BatchItem is one candidate's outcome: the run started, or why not.
type BatchItem struct {
	CandidateID string
	Run         *StudioCV
	Reason      string
}

// skipReasons are the errors that skip one candidate (in words for the
// recruiter) rather than stop the batch.
var skipReasons = map[api.ErrorCode]string{
	api.ErrorCodeNOTFOUND:        "not found, or not one of the candidates you may see",
	api.ErrorCodeSTUDIOCVRUNNING: "their CV is being formatted already",
	api.ErrorCodeSTUDIOCVSOURCE:  "has no PDF or Word CV they sent",
	api.ErrorCodeDOCUMENTLIMIT:   "already has as many formatted CVs as they can keep: delete one, or format theirs on their page to replace one",
}

// BatchSkipReason says why a candidate is skipped, or ok false when err
// should stop the whole batch (no AI for it, or a fault).
func BatchSkipReason(err error) (reason string, ok bool) {
	e, isApp := apperr.As(err)
	if !isApp {
		return "", false
	}
	reason, ok = skipReasons[e.Code]
	return reason, ok
}

// StartStudioBatch formats each candidate's newest CV they sent, with the
// same options (o.DocumentID and o.Replace are ignored). Candidates are
// taken once each, in the order given.
func (s *Service) StartStudioBatch(ctx context.Context, p authz.Principal, candidateIDs []string, o StudioOptions) ([]BatchItem, error) {
	ids := slices.Clone(candidateIDs)
	seen := map[string]bool{}
	ids = slices.DeleteFunc(ids, func(id string) bool { dup := seen[id]; seen[id] = true; return dup })
	if len(ids) == 0 || len(ids) > MaxBatch {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "candidateIds", Message: "choose 1 to 20 candidates"}})
	}
	if _, err := summaryOptions(o); err != nil { // a direction about pay stops the batch before anything starts
		return nil, err
	}
	o.Replace = nil
	types := slices.Sorted(maps.Keys(formattable))
	out := make([]BatchItem, 0, len(ids))
	for _, id := range ids {
		item := BatchItem{CandidateID: id}
		var doc string
		err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
			if _, err := s.deps.Candidates.ForCV(ctx, tx, p, id); err != nil {
				return err // not found, or not theirs to see
			}
			var err error
			doc, err = s.deps.SourceCVs.NewestCV(ctx, tx, id, types)
			return err
		})
		if err == nil && doc == "" {
			err = apperr.New(api.ErrorCodeSTUDIOCVSOURCE, "no CV")
		}
		if err == nil {
			run := o
			run.DocumentID = doc
			var started StudioCV
			if started, err = s.StartStudio(ctx, p, id, run); err == nil {
				item.Run = &started
			}
		}
		if err != nil {
			reason, skip := BatchSkipReason(err)
			if !skip {
				return out, err
			}
			item.Reason = reason
		}
		out = append(out, item)
	}
	return out, nil
}

// StartStudioCvBatch handles POST /v1/studio-cvs/batch.
func (h *HTTP) StartStudioCvBatch(ctx context.Context, req api.StartStudioCvBatchRequestObject) (api.StartStudioCvBatchResponseObject, error) {
	b := req.Body
	o := StudioOptions{Blind: b.Blind != nil && *b.Blind, NeutralGender: b.NeutralGender != nil && *b.NeutralGender,
		CorrectLanguage: b.CorrectLanguage != nil && *b.CorrectLanguage}
	if b.Alignment != nil {
		o.Alignment = string(*b.Alignment)
	}
	if b.Summary != nil {
		o.Summary = string(*b.Summary)
	}
	if b.SummaryFocus != nil {
		o.SummaryFocus = string(*b.SummaryFocus)
	}
	if d, err := b.SummaryDirection.Get(); err == nil {
		o.SummaryDirection = d
	}
	items, err := h.svc.StartStudioBatch(ctx, authz.MustFromContext(ctx), b.CandidateIds, o)
	if err != nil {
		return nil, err
	}
	out := api.StartStudioCvBatch202JSONResponse{Items: make([]api.StudioCvBatchItem, len(items))}
	for i, it := range items {
		out.Items[i] = api.StudioCvBatchItem{CandidateId: it.CandidateID, Started: it.Run != nil}
		if it.Run != nil {
			run := studioToAPI(*it.Run)
			out.Items[i].StudioCv = &run
		} else {
			reason := it.Reason
			out.Items[i].Reason = &reason
		}
	}
	return out, nil
}
