package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
)

// workAuthFromCV: once a candidate's own CV is read, its nationality and the
// passes it mentions suggest a work authorization (candidates module).
type workAuthFromCV struct {
	documents  *documents.Module
	candidates *candidates.Module
}

func (a workAuthFromCV) ReadDone(ctx context.Context, tx pgx.Tx, _, documentID string, profile *api.ParsedProfile) error {
	if profile == nil {
		return nil
	}
	doc, _, err := a.documents.SourceCV(ctx, tx, documentID)
	if err != nil || doc.Source == "formatted" {
		return err
	}
	nationality := ""
	if v, err := profile.Nationality.Get(); err == nil {
		nationality = v.Value
	}
	text, err := a.documents.LatestCVText(ctx, tx, doc.CandidateID)
	if err != nil {
		return err
	}
	return a.candidates.SuggestWorkAuthFromCV(ctx, tx, doc.CandidateID, nationality, text)
}
