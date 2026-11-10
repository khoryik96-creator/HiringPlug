package documents

// The same CV is never uploaded twice: a file with exactly the content of one
// a candidate already has is refused, an import skips a file already in
// HiringPlug, and the composer of an upload can ask first about CVs with the
// same file name (the SHA-256 of each file is worked out before sending it).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Match is a CV already in HiringPlug like a file about to be uploaded.
type Match struct {
	DocumentID, CandidateID, CandidateName, FileName string
	CreatedAt                                        time.Time
}

// FileRef is a file about to be uploaded: its name and SHA-256.
type FileRef struct{ FileName, SHA256 string }

// MatchResult is what is already there for one file.
type MatchResult struct {
	FileRef
	SameContent *Match  // exactly this file: skip it
	SameName    []Match // the same name, other content: ask
}

// SumOf is a file's SHA-256, as documents keep it.
func SumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Matches finds, for each file, a CV with the same content and CVs with the
// same name: candidateID's only, or any candidate's p may see.
func (s *Service) Matches(ctx context.Context, p authz.Principal, candidateID *string, files []FileRef) ([]MatchResult, error) {
	var out []MatchResult
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		sums, names := make([]string, len(files)), make([]string, len(files))
		for i, f := range files {
			sums[i], names[i] = f.SHA256, strings.ToLower(CleanFileName(f.FileName))
		}
		same, err := q.DocumentsWithContent(ctx, store.DocumentsWithContentParams{Sums: sums, CandidateID: candidateID})
		if err != nil {
			return err
		}
		named, err := q.DocumentsNamed(ctx, store.DocumentsNamedParams{Names: names, CandidateID: candidateID})
		if err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, r := range same {
			ids[r.CandidateID] = true
		}
		for _, r := range named {
			ids[r.CandidateID] = true
		}
		cands := make([]string, 0, len(ids))
		for id := range ids {
			cands = append(cands, id)
		}
		who, err := s.candidates.Names(ctx, tx, cands)
		if err != nil {
			return err
		}
		out = make([]MatchResult, len(files))
		for i, f := range files {
			res := MatchResult{FileRef: f, SameName: []Match{}}
			for _, r := range same { // the oldest with this content
				if r.Sha256 == f.SHA256 {
					res.SameContent = &Match{DocumentID: r.ID, CandidateID: r.CandidateID, CandidateName: who[r.CandidateID], FileName: r.FileName, CreatedAt: r.CreatedAt}
					break
				}
			}
			for _, r := range named {
				if strings.EqualFold(r.FileName, CleanFileName(f.FileName)) && r.Sha256 != f.SHA256 {
					res.SameName = append(res.SameName, Match{DocumentID: r.ID, CandidateID: r.CandidateID, CandidateName: who[r.CandidateID], FileName: r.FileName, CreatedAt: r.CreatedAt})
				}
			}
			out[i] = res
		}
		return nil
	})
	return out, err
}

// sameContent is a CV with exactly data's content: candidateID's, or (nil)
// any candidate's in the tenant.
func (s *Service) sameContent(ctx context.Context, tx pgx.Tx, candidateID *string, data []byte) (*Match, error) {
	rows, err := store.New(tx).DocumentsWithContent(ctx, store.DocumentsWithContentParams{Sums: []string{SumOf(data)}, CandidateID: candidateID})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	r := rows[0]
	who, err := s.candidates.Names(ctx, tx, []string{r.CandidateID})
	if err != nil {
		return nil, err
	}
	return &Match{DocumentID: r.ID, CandidateID: r.CandidateID, CandidateName: who[r.CandidateID], FileName: r.FileName, CreatedAt: r.CreatedAt}, nil
}

// duplicateError refuses a file the candidate already has.
func duplicateError(m Match) error {
	return apperr.New(api.ErrorCodeDOCUMENTDUPLICATE, "This candidate already has this exact file ("+m.FileName+"): it was not uploaded again.").
		WithDetail("existingDocumentId", m.DocumentID)
}
