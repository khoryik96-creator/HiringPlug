package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// profileFromNewestCV: once a candidate's CV is read, and it is their newest,
// the profile is brought up to date from it (candidates.UpdateFromCV): a new
// CV says where they work now. Reading an older CV again changes nothing.
type profileFromNewestCV struct {
	documents  *documents.Module
	candidates *candidates.Module
}

func (a profileFromNewestCV) ReadDone(ctx context.Context, tx pgx.Tx, _, documentID string, profile *api.ParsedProfile) error {
	if profile == nil {
		return nil
	}
	candidateID, source, _, err := a.documents.CVTextOf(ctx, tx, documentID)
	if err != nil || source == "formatted" {
		return err
	}
	newest, err := a.documents.NewestCV(ctx, tx, candidateID)
	if err != nil || newest != documentID {
		return err
	}
	// In a savepoint: the CV's details are kept even when the profile cannot
	// be updated from them (a value the rules refuse, a save at the same time).
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := a.candidates.UpdateFromCV(ctx, sp, candidateID, documentID, cvFromParsed(*profile)); err != nil {
		logx.From(ctx).WarnContext(ctx, "profile not updated from its newest CV", "candidate_id", candidateID, "document_id", documentID, "error", err)
		return sp.Rollback(ctx)
	}
	return sp.Commit(ctx)
}
