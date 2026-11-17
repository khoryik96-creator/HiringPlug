package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
)

// workAuthFromCV: once a candidate's own CV is read, its nationality and the
// passes it mentions (both from that same CV) suggest a work authorization
// (candidates module).
type workAuthFromCV struct {
	documents  *documents.Module
	candidates *candidates.Module
}

func (a workAuthFromCV) ReadDone(ctx context.Context, tx pgx.Tx, _, documentID string, profile *api.ParsedProfile) error {
	if profile == nil {
		return nil
	}
	candidateID, source, text, err := a.documents.CVTextOf(ctx, tx, documentID)
	if err != nil || source == "formatted" {
		return err
	}
	nationality := ""
	if v, err := profile.Nationality.Get(); err == nil {
		nationality = v.Value
	}
	return a.candidates.SuggestWorkAuthFromCV(ctx, tx, candidateID, nationality, text)
}
