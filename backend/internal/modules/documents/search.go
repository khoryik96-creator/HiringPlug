package documents

// CV text for advanced search: which candidates' CVs match a boolean
// search, where (a snippet), and a CV's whole text with the words marked.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/boolsearch"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// CandidatesWithMatchingCV returns up to limit candidates with a CV (read,
// not formatted) matching ts, a text search query, in the caller's
// transaction (row-level security: only candidates the user may see).
func (m *Module) CandidatesWithMatchingCV(ctx context.Context, tx pgx.Tx, ts string, limit int) ([]string, error) {
	return store.New(tx).CandidatesWithMatchingCV(ctx, store.CandidatesWithMatchingCVParams{Ts: ts, RowLimit: int32(limit)}) //nolint:gosec // a small constant
}

// CVSnippets returns, for each candidate given whose CV matches ts, where
// their best matching CV matches: the words found between [[ and ]].
func (m *Module) CVSnippets(ctx context.Context, tx pgx.Tx, ts string, candidateIDs []string) (map[string]string, error) {
	rows, err := store.New(tx).CVSnippets(ctx, store.CVSnippetsParams{Ts: ts, CandidateIds: candidateIDs})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.CandidateID] = strings.Join(strings.Fields(r.Snippet), " ")
	}
	return out, nil
}

// RankCVsByText orders the given candidates by how many words their CVs
// share with a text (a job description), best first, up to limit; those
// sharing none are left out.
func (m *Module) RankCVsByText(ctx context.Context, tx pgx.Tx, text string, candidateIDs []string, limit int) ([]string, error) {
	rows, err := store.New(tx).RankCVsByText(ctx, store.RankCVsByTextParams{JobText: text, CandidateIds: candidateIDs, RowLimit: int32(limit)}) //nolint:gosec // small
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.CandidateID
	}
	return out, err
}

// Highlights splits text marked by the database (each match between \x01
// and \x02) into parts.
func Highlights(marked string) ([]api.DocumentHighlightPart, int) {
	var parts []api.DocumentHighlightPart
	matches := 0
	for marked != "" {
		start := strings.IndexByte(marked, 1)
		if start < 0 {
			parts = append(parts, api.DocumentHighlightPart{Text: marked})
			break
		}
		if start > 0 {
			parts = append(parts, api.DocumentHighlightPart{Text: marked[:start]})
		}
		rest := marked[start+1:]
		end := strings.IndexByte(rest, 2)
		if end < 0 {
			end = len(rest)
		}
		if rest[:end] != "" {
			parts = append(parts, api.DocumentHighlightPart{Text: rest[:end], Match: true})
			matches++
		}
		if end == len(rest) {
			break
		}
		marked = rest[end+1:]
	}
	return parts, matches
}

// HighlightDocument handles GET /v1/documents/{documentId}/highlights.
func (h *HTTP) HighlightDocument(ctx context.Context, req api.HighlightDocumentRequestObject) (api.HighlightDocumentResponseObject, error) {
	q, err := boolsearch.Parse(req.Params.Boolean)
	if err != nil {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "boolean", Message: err.Error()}})
	}
	var marked string
	err = database.InTx(ctx, h.svc.pool, authz.MustFromContext(ctx), func(tx pgx.Tx) error {
		var err error
		marked, err = store.New(tx).HighlightDocumentText(ctx, store.HighlightDocumentTextParams{Ts: q.TSQuery(), ID: req.DocumentId})
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	parts, n := Highlights(marked)
	if parts == nil {
		parts = []api.DocumentHighlightPart{}
	}
	return api.HighlightDocument200JSONResponse{Parts: parts, Matches: n}, nil
}
